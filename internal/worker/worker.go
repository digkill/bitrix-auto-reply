package worker

import (
	"log"
	"time"

	"github.com/digkill/bitrix-auto-reply/internal/actions"
	"github.com/digkill/bitrix-auto-reply/internal/bitrix"
	"github.com/digkill/bitrix-auto-reply/internal/matcher"
	"github.com/digkill/bitrix-auto-reply/internal/storage"
)

/*
Worker — главный фоновый процесс.

Он:
1. Каждые N секунд читает список личных диалогов.
2. Берёт последние сообщения.
3. Игнорирует свои сообщения.
4. Проверяет, обрабатывали ли уже сообщение.
5. Ищет правило по ключевым словам.
6. Выполняет действие.
7. Сохраняет результат в MySQL.
*/
type Worker struct {
	bitrixClient          *bitrix.Client
	store                 *storage.Storage
	actionExecutor        *actions.Executor
	selfUserID            int64
	pollIntervalSeconds   int
	dialogCooldownSeconds int
}

func NewWorker(
	bitrixClient *bitrix.Client,
	store *storage.Storage,
	selfUserID int64,
	pollIntervalSeconds int,
	dialogCooldownSeconds int,
) *Worker {
	return &Worker{
		bitrixClient:          bitrixClient,
		store:                 store,
		actionExecutor:        actions.NewExecutor(bitrixClient),
		selfUserID:            selfUserID,
		pollIntervalSeconds:   pollIntervalSeconds,
		dialogCooldownSeconds: dialogCooldownSeconds,
	}
}

/*
Run запускает бесконечный цикл worker-а.
*/
func (w *Worker) Run() {
	log.Println("worker started")

	ticker := time.NewTicker(time.Duration(w.pollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		if err := w.ProcessOnce(); err != nil {
			log.Printf("worker process error: %v", err)
		}

		<-ticker.C
	}
}

/*
ProcessOnce — один проход обработки.

Вынесено отдельно, чтобы потом было удобно:
- тестировать;
- запускать вручную;
- вызывать из debug endpoint.
*/
func (w *Worker) ProcessOnce() error {
	/*
		Берём все recent-диалоги, а не только unread.
		Если чат открыт в Bitrix24, сообщение может стать прочитанным
		до следующего polling-а. Повторы отсекаются через processed_messages.
	*/
	dialogs, err := w.bitrixClient.RecentList(false)
	if err != nil {
		return err
	}

	/*
		Правила подтягиваем из БД на каждом цикле.
		Это значит, что ты можешь добавить правило в админке,
		и worker подхватит его без перезапуска.
	*/
	rules, err := w.store.GetActiveRules()
	if err != nil {
		return err
	}

	for _, dialog := range dialogs {
		dialogID := dialog.ID.String()
		if dialogID == "" {
			continue
		}

		w.processDialog(dialogID, rules)
	}

	return nil
}

/*
processDialog обрабатывает один диалог.
*/
func (w *Worker) processDialog(dialogID string, rules []storage.Rule) {
	/*
		Берём последние 5 сообщений.
		Можно увеличить до 10-20, если сообщения часто летят пачкой.
	*/
	messages, err := w.bitrixClient.GetDialogMessages(dialogID, 5)
	if err != nil {
		log.Printf("get dialog messages error dialog=%s: %v", dialogID, err)
		return
	}

	for _, msg := range messages {
		w.processMessage(dialogID, msg, rules)
	}
}

/*
processMessage обрабатывает одно сообщение.
*/
func (w *Worker) processMessage(dialogID string, msg bitrix.DialogMessage, rules []storage.Rule) {
	if msg.ID == 0 {
		return
	}

	/*
		Критически важно:
		не отвечаем на свои сообщения.

		Иначе будет цикл:
		бот отправил сообщение → увидел своё сообщение → снова ответил.
	*/
	if msg.AuthorID == w.selfUserID {
		return
	}

	processed, err := w.store.IsProcessed(msg.ID)
	if err != nil {
		log.Printf("is processed error message_id=%d: %v", msg.ID, err)
		return
	}

	if processed {
		return
	}

	match := matcher.Match(msg.Text, rules)

	if !match.Found {
		/*
			Если правило не найдено — всё равно сохраняем сообщение.
			Так бот не будет снова и снова проверять старое сообщение.
		*/
		if err := w.store.SaveProcessed(msg.ID, dialogID, msg.AuthorID, msg.Text, "", nil); err != nil {
			log.Printf("save non-matched message error: %v", err)
		}

		return
	}

	canAnswer, err := w.store.CanAnswerDialog(dialogID, w.dialogCooldownSeconds)
	if err != nil {
		log.Printf("cooldown check error dialog=%s: %v", dialogID, err)
		return
	}

	if !canAnswer {
		answerText := "skipped by cooldown"
		ruleID := match.Rule.ID

		if err := w.store.SaveProcessed(msg.ID, dialogID, msg.AuthorID, msg.Text, answerText, &ruleID); err != nil {
			log.Printf("save cooldown message error: %v", err)
		}

		return
	}

	answer, err := w.actionExecutor.Execute(dialogID, msg.Text, match.Rule)
	if err != nil {
		log.Printf("execute action error dialog=%s message=%d rule=%d: %v", dialogID, msg.ID, match.Rule.ID, err)
		return
	}

	ruleID := match.Rule.ID

	if err := w.store.SaveProcessed(msg.ID, dialogID, msg.AuthorID, msg.Text, answer, &ruleID); err != nil {
		log.Printf("save processed message error: %v", err)
	}

	if err := w.store.TouchDialog(dialogID); err != nil {
		log.Printf("touch dialog error: %v", err)
	}

	log.Printf(
		"answered dialog=%s message_id=%d rule_id=%d action=%s",
		dialogID,
		msg.ID,
		match.Rule.ID,
		match.Rule.ActionType,
	)
}
