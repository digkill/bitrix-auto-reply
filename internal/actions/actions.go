package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/digkill/bitrix-auto-reply/internal/bitrix"
	"github.com/digkill/bitrix-auto-reply/internal/storage"
)

/*
	Executor выполняет действие правила.

	Поддерживаем action_type:

	1. text
	   Просто отправить response_text.

	2. file
	   Отправить response_text + ссылку на файл/картинку.

	3. api
	   Сходить во внешний API и отправить его ответ в Bitrix24.
*/
type Executor struct {
	bitrixClient *bitrix.Client
	httpClient   *http.Client
}

func NewExecutor(bitrixClient *bitrix.Client) *Executor {
	return &Executor{
		bitrixClient: bitrixClient,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

/*
	Execute — главный метод выполнения действия.
*/
func (e *Executor) Execute(dialogID string, incomingText string, rule storage.Rule) (string, error) {
	switch rule.ActionType {
	case "text":
		return e.executeText(dialogID, rule)

	case "file":
		return e.executeFile(dialogID, rule)

	case "api":
		return e.executeAPI(dialogID, incomingText, rule)

	default:
		return "", fmt.Errorf("unknown action_type: %s", rule.ActionType)
	}
}

/*
	executeText отправляет обычный текст.
*/
func (e *Executor) executeText(dialogID string, rule storage.Rule) (string, error) {
	answer := rule.ResponseText

	if answer == "" {
		answer = "Привет! Увидел сообщение, скоро отвечу."
	}

	if err := e.bitrixClient.SendMessage(dialogID, answer); err != nil {
		return "", err
	}

	return answer, nil
}

/*
	executeFile отправляет файл/картинку как ссылку.

	Пример:
	response_text = "Вот презентация:"
	file_url = "https://site.ru/file.pdf"

	В Bitrix24 уйдёт:

	Вот презентация:

	https://site.ru/file.pdf
*/
func (e *Executor) executeFile(dialogID string, rule storage.Rule) (string, error) {
	if rule.FileURL == "" {
		return "", fmt.Errorf("file_url is empty")
	}

	text := rule.ResponseText
	if text == "" {
		text = "Отправляю файл:"
	}

	if err := e.bitrixClient.SendFileMessage(dialogID, text, rule.FileURL); err != nil {
		return "", err
	}

	return text + "\n" + rule.FileURL, nil
}

/*
	executeAPI ходит во внешний API.

	Сценарий:
	- клиент пишет: "остаток по заказу 123"
	- правило вызывает твой API
	- API возвращает JSON с ответом
	- бот отправляет этот ответ в Bitrix24

	Ожидаемый формат ответа от внешнего API:

	{
		"message": "Ваш заказ готов"
	}

	Если message нет — отправим весь JSON как строку.
*/
func (e *Executor) executeAPI(dialogID string, incomingText string, rule storage.Rule) (string, error) {
	if rule.APIURL == "" {
		return "", fmt.Errorf("api_url is empty")
	}

	req, err := http.NewRequest(http.MethodGet, rule.APIURL, nil)
	if err != nil {
		return "", err
	}

	/*
		Передаём исходный текст в заголовке.
		Так внешний API может понять, что написал пользователь.

		Позже можно заменить на POST JSON.
	*/
	req.Header.Set("X-Incoming-Text", incomingText)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("api bad status %d: %s", resp.StatusCode, string(body))
	}

	answer := extractMessageFromAPI(body)

	if answer == "" {
		answer = string(body)
	}

	if err := e.bitrixClient.SendMessage(dialogID, answer); err != nil {
		return "", err
	}

	return answer, nil
}

/*
	extractMessageFromAPI пытается достать поле message из JSON.
*/
func extractMessageFromAPI(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}

	return payload.Message
}