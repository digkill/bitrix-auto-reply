package bitrix

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

/*
Client — маленькая обёртка над REST API Bitrix24.

В .env у нас лежит базовая ссылка вида:

https://b24-xxxxx.bitrix24.ru/rest/USER_ID/WEBHOOK_KEY/

А дальше мы просто добавляем к ней имя метода:

im.recent.list
im.dialog.messages.get
im.message.add

Итоговый URL будет, например:

https://b24-xxxxx.bitrix24.ru/rest/USER_ID/WEBHOOK_KEY/im.message.add
*/
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

/*
APIResponse — стандартная обёртка ответа Bitrix24.

У Bitrix24 почти все REST-методы возвращают JSON вида:

	{
		"result": ...
	}

А если ошибка:

	{
		"error": "...",
		"error_description": "..."
	}
*/
type APIResponse[T any] struct {
	Result           T      `json:"result"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

/*
StringID — ID из Bitrix24, который в разных ответах может приходить
как строкой, так и числом. Внутри приложения держим его строкой,
потому что DIALOG_ID в REST-запросах тоже отправляется строкой.
*/
type StringID string

func (id *StringID) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*id = StringID(text)
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*id = StringID(number.String())
		return nil
	}

	return fmt.Errorf("invalid string id: %s", string(data))
}

func (id StringID) String() string {
	return string(id)
}

/*
RecentListRequest — параметры для im.recent.list.

UNREAD_ONLY=Y — брать только непрочитанные.
SKIP_OPENLINES=Y — не брать открытые линии.
SKIP_CHAT=N — брать и личные диалоги, и групповые чаты.
*/
type RecentListRequest struct {
	UnreadOnly      string `json:"UNREAD_ONLY"`
	SkipOpenLines   string `json:"SKIP_OPENLINES"`
	SkipChat        string `json:"SKIP_CHAT"`
	GetOriginalText string `json:"GET_ORIGINAL_TEXT"`
}

/*
RecentListResult — результат im.recent.list.

Нас интересует список items.
*/
type RecentListResult struct {
	Items []RecentItem `json:"items"`
}

/*
RecentItem — один диалог из списка последних диалогов.
*/
type RecentItem struct {
	ID      StringID      `json:"id"`
	Type    string        `json:"type"`
	Message RecentMessage `json:"message"`
	User    RecentUser    `json:"user"`
}

/*
RecentMessage — последнее сообщение в диалоге.
*/
type RecentMessage struct {
	ID       int64  `json:"id"`
	Text     string `json:"text"`
	AuthorID int64  `json:"author_id"`
	Date     string `json:"date"`
}

/*
RecentUser — пользователь, с которым идёт личный диалог.
*/
type RecentUser struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

/*
DialogMessagesRequest — параметры для im.dialog.messages.get.

DIALOG_ID — ID диалога.
LIMIT — сколько последних сообщений забрать.
GET_ORIGINAL_TEXT=Y — получить оригинальный текст без лишней обработки.
*/
type DialogMessagesRequest struct {
	DialogID        string `json:"DIALOG_ID"`
	Limit           int    `json:"LIMIT"`
	GetOriginalText string `json:"GET_ORIGINAL_TEXT"`
}

/*
DialogMessagesResult — результат истории сообщений.
*/
type DialogMessagesResult struct {
	Messages []DialogMessage `json:"messages"`
}

/*
DialogMessage — одно сообщение из истории.
*/
type DialogMessage struct {
	ID       int64  `json:"id"`
	ChatID   int64  `json:"chat_id"`
	AuthorID int64  `json:"author_id"`
	Text     string `json:"text"`
	Date     string `json:"date"`
}

/*
SendMessageRequest — запрос на отправку обычного текстового сообщения.

SYSTEM=N — обычное сообщение, не системное.
URL_PREVIEW=N — не генерировать превью ссылок.
*/
type SendMessageRequest struct {
	DialogID   string `json:"DIALOG_ID"`
	Message    string `json:"MESSAGE"`
	System     string `json:"SYSTEM"`
	URLPreview string `json:"URL_PREVIEW"`
}

/*
RecentList получает список последних диалогов.

	В нашем случае это главный метод polling-а:
	каждые N секунд мы спрашиваем Bitrix24:
	"Есть ли новые сообщения?"
*/
func (c *Client) RecentList(onlyUnread bool) ([]RecentItem, error) {
	unread := "N"
	if onlyUnread {
		unread = "Y"
	}

	req := RecentListRequest{
		UnreadOnly:      unread,
		SkipOpenLines:   "Y",
		SkipChat:        "N",
		GetOriginalText: "Y",
	}

	var resp APIResponse[RecentListResult]

	if err := c.call("im.recent.list", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("bitrix error: %s - %s", resp.Error, resp.ErrorDescription)
	}

	return resp.Result.Items, nil
}

/*
GetDialogMessages забирает последние сообщения конкретного диалога.

Почему мы не доверяем только last message из im.recent.list:
- иногда нужно обработать несколько новых сообщений;
- можно пропустить сообщение при быстром общении;
- так проще защититься через processed_messages.
*/
func (c *Client) GetDialogMessages(dialogID string, limit int) ([]DialogMessage, error) {
	req := DialogMessagesRequest{
		DialogID:        dialogID,
		Limit:           limit,
		GetOriginalText: "Y",
	}

	var resp APIResponse[DialogMessagesResult]

	if err := c.call("im.dialog.messages.get", req, &resp); err != nil {
		return nil, err
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("bitrix error: %s - %s", resp.Error, resp.ErrorDescription)
	}

	return resp.Result.Messages, nil
}

/*
SendMessage отправляет текст в личный диалог.

Если webhook создан от твоего пользователя,
сообщение уйдёт от твоего имени.
*/
func (c *Client) SendMessage(dialogID string, text string) error {
	req := SendMessageRequest{
		DialogID:   dialogID,
		Message:    text,
		System:     "N",
		URLPreview: "N",
	}

	var resp APIResponse[int64]

	if err := c.call("im.message.add", req, &resp); err != nil {
		return err
	}

	if resp.Error != "" {
		return fmt.Errorf("bitrix error: %s - %s", resp.Error, resp.ErrorDescription)
	}

	return nil
}

/*
SendFileMessage — пока базовая версия отправки файла/картинки как ссылки.

Почему так:
- это стабильно;
- можно отправить картинку, PDF, ссылку на файл из твоего API;
- Bitrix24 сам сделает кликабельную ссылку.

Позже можно заменить на настоящий upload через im.v2.File.upload,
если нужно именно прикреплять файл в Bitrix24.
*/
func (c *Client) SendFileMessage(dialogID string, text string, fileURL string) error {
	message := text

	if message != "" {
		message += "\n\n"
	}

	message += fileURL

	return c.SendMessage(dialogID, message)
}

/*
call — общий метод для всех REST-запросов к Bitrix24.

Он:
1. Кодирует payload в JSON.
2. Делает POST-запрос.
3. Проверяет HTTP-статус.
4. Декодирует JSON-ответ в нужную структуру.
*/
func (c *Client) call(method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("json marshal error: %w", err)
	}

	url := c.baseURL + method

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("request create error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request error: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body error: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitrix method %s bad http status %d: %s", method, resp.StatusCode, string(respBody))
	}

	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("json decode error: %w, body: %s", err, string(respBody))
	}

	return nil
}
