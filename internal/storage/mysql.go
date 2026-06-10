package storage

import (
	"database/sql"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

/*
	Rule — правило автоответчика.

	keywords храним в БД строкой:
	"цена,прайс,стоимость"

	В Go превращаем это в []string.
*/
type Rule struct {
	ID           int64
	Name         string
	Keywords     []string
	ActionType   string
	ResponseText string
	FileURL      string
	APIURL       string
	IsActive     bool
}

/*
	Storage — слой работы с MySQL.

	Через него worker:
	- получает правила;
	- проверяет, обработано ли сообщение;
	- сохраняет обработанные сообщения;
	- проверяет cooldown по диалогу.
*/
type Storage struct {
	db *sql.DB
}

func NewStorage(dsn string) (*Storage, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &Storage{db: db}, nil
}

/*
	GetActiveRules получает все включённые правила из базы.

	Админка будет менять таблицу rules,
	а worker каждый цикл будет подтягивать актуальные правила.
*/
func (s *Storage) GetActiveRules() ([]Rule, error) {
	rows, err := s.db.Query(`
SELECT 
	id,
	name,
	keywords,
	action_type,
	COALESCE(response_text, ''),
	COALESCE(file_url, ''),
	COALESCE(api_url, ''),
	is_active
FROM rules
WHERE is_active = 1
ORDER BY id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule

	for rows.Next() {
		var rule Rule
		var keywordsRaw string
		var isActive int

		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&keywordsRaw,
			&rule.ActionType,
			&rule.ResponseText,
			&rule.FileURL,
			&rule.APIURL,
			&isActive,
		); err != nil {
			return nil, err
		}

		rule.Keywords = splitKeywords(keywordsRaw)
		rule.IsActive = isActive == 1

		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

/*
	GetAllRules нужен для мини-админки.
	Там показываем и активные, и выключенные правила.
*/
func (s *Storage) GetAllRules() ([]Rule, error) {
	rows, err := s.db.Query(`
SELECT 
	id,
	name,
	keywords,
	action_type,
	COALESCE(response_text, ''),
	COALESCE(file_url, ''),
	COALESCE(api_url, ''),
	is_active
FROM rules
ORDER BY id DESC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []Rule

	for rows.Next() {
		var rule Rule
		var keywordsRaw string
		var isActive int

		if err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&keywordsRaw,
			&rule.ActionType,
			&rule.ResponseText,
			&rule.FileURL,
			&rule.APIURL,
			&isActive,
		); err != nil {
			return nil, err
		}

		rule.Keywords = splitKeywords(keywordsRaw)
		rule.IsActive = isActive == 1

		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

/*
	CreateRule создаёт новое правило из админки.
*/
func (s *Storage) CreateRule(rule Rule) error {
	_, err := s.db.Exec(`
INSERT INTO rules
(name, keywords, action_type, response_text, file_url, api_url, is_active)
VALUES (?, ?, ?, ?, ?, ?, ?)
`,
		rule.Name,
		strings.Join(rule.Keywords, ","),
		rule.ActionType,
		rule.ResponseText,
		rule.FileURL,
		rule.APIURL,
		boolToInt(rule.IsActive),
	)

	return err
}

/*
	DeleteRule удаляет правило из админки.
*/
func (s *Storage) DeleteRule(id int64) error {
	_, err := s.db.Exec(`DELETE FROM rules WHERE id = ?`, id)
	return err
}

/*
	IsProcessed проверяет, обрабатывали ли мы уже сообщение.

	Это защита от дублей.
	Без неё бот будет отвечать на одно и то же сообщение бесконечно.
*/
func (s *Storage) IsProcessed(messageID int64) (bool, error) {
	var exists int

	err := s.db.QueryRow(`
SELECT 1 
FROM processed_messages 
WHERE bitrix_message_id = ? 
LIMIT 1
`, messageID).Scan(&exists)

	if err == sql.ErrNoRows {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

/*
	SaveProcessed сохраняет сообщение как обработанное.

	answerText может быть пустым:
	например, если ключевые слова не найдены.
	Так мы всё равно не будем постоянно проверять одно и то же старое сообщение.
*/
func (s *Storage) SaveProcessed(
	messageID int64,
	dialogID string,
	authorID int64,
	messageText string,
	answerText string,
	ruleID *int64,
) error {
	_, err := s.db.Exec(`
INSERT IGNORE INTO processed_messages
(bitrix_message_id, dialog_id, author_id, message_text, answer_text, rule_id)
VALUES (?, ?, ?, ?, ?, ?)
`,
		messageID,
		dialogID,
		authorID,
		messageText,
		answerText,
		ruleID,
	)

	return err
}

/*
	CanAnswerDialog проверяет cooldown.

	Например:
	если человек написал 5 сообщений подряд:
	"цена"
	"срочно"
	"прайс"
	"алло"
	"ответь"

	Без cooldown бот может отправить 3-5 автоответов подряд.
	С cooldown мы ограничиваем: один ответ раз в N секунд.
*/
func (s *Storage) CanAnswerDialog(dialogID string, cooldownSeconds int) (bool, error) {
	var lastAnswerAt sql.NullTime

	err := s.db.QueryRow(`
SELECT last_answer_at 
FROM dialog_locks 
WHERE dialog_id = ? 
LIMIT 1
`, dialogID).Scan(&lastAnswerAt)

	if err == sql.ErrNoRows {
		return true, nil
	}

	if err != nil {
		return false, err
	}

	if !lastAnswerAt.Valid {
		return true, nil
	}

	return time.Since(lastAnswerAt.Time) >= time.Duration(cooldownSeconds)*time.Second, nil
}

/*
	TouchDialog обновляет время последнего автоответа по диалогу.
*/
func (s *Storage) TouchDialog(dialogID string) error {
	_, err := s.db.Exec(`
INSERT INTO dialog_locks (dialog_id, last_answer_at)
VALUES (?, NOW())
ON DUPLICATE KEY UPDATE last_answer_at = NOW()
`, dialogID)

	return err
}

func splitKeywords(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		keyword := strings.TrimSpace(part)
		if keyword != "" {
			result = append(result, keyword)
		}
	}

	return result
}

func boolToInt(v bool) int {
	if v {
		return 1
	}

	return 0
}