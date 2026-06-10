package admin

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/digkill/bitrix-auto-reply/internal/storage"
)

/*
	Server — мини-админка для управления правилами.

	Что умеет:
	- показать список правил;
	- добавить новое правило;
	- удалить правило.

	Авторизация простая: Basic Auth.
	Логин/пароль берутся из .env:

	ADMIN_LOGIN=admin
	ADMIN_PASSWORD=admin123
*/
type Server struct {
	store         *storage.Storage
	adminLogin    string
	adminPassword string
}

/*
	NewServer создаёт экземпляр админки.
*/
func NewServer(store *storage.Storage, adminLogin string, adminPassword string) *Server {
	return &Server{
		store:         store,
		adminLogin:    adminLogin,
		adminPassword: adminPassword,
	}
}

/*
	RegisterRoutes регистрирует HTTP-роуты.

	GET  /admin        — список правил
	POST /admin/rules  — создать правило
	POST /admin/delete — удалить правило
*/
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin", s.basicAuth(s.handleIndex))
	mux.HandleFunc("/admin/rules", s.basicAuth(s.handleCreateRule))
	mux.HandleFunc("/admin/delete", s.basicAuth(s.handleDeleteRule))
}

/*
	basicAuth — простая защита админки.

	Браузер покажет стандартное окно ввода логина и пароля.
*/
func (s *Server) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		login, password, ok := r.BasicAuth()

		if !ok || login != s.adminLogin || password != s.adminPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="Bitrix Auto Reply Admin"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

/*
	handleIndex показывает главную страницу админки.

	На ней:
	- таблица правил;
	- форма создания нового правила;
	- подсказка по action_type.
*/
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rules, err := s.store.GetAllRules()
	if err != nil {
		http.Error(w, "Get rules error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tpl, err := template.New("admin").Parse(adminHTML)
	if err != nil {
		http.Error(w, "Template parse error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		Rules []storage.Rule
	}{
		Rules: rules,
	}

	if err := tpl.Execute(w, data); err != nil {
		log.Printf("template execute error: %v", err)
	}
}

/*
	handleCreateRule создаёт новое правило.

	Форма отправляет:
	- name
	- keywords
	- action_type
	- response_text
	- file_url
	- api_url
	- is_active
*/
func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Parse form error: "+err.Error(), http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	keywordsRaw := strings.TrimSpace(r.FormValue("keywords"))
	actionType := strings.TrimSpace(r.FormValue("action_type"))
	responseText := strings.TrimSpace(r.FormValue("response_text"))
	fileURL := strings.TrimSpace(r.FormValue("file_url"))
	apiURL := strings.TrimSpace(r.FormValue("api_url"))
	isActive := r.FormValue("is_active") == "1"

	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if keywordsRaw == "" {
		http.Error(w, "keywords is required", http.StatusBadRequest)
		return
	}

	if actionType == "" {
		http.Error(w, "action_type is required", http.StatusBadRequest)
		return
	}

	/*
		Валидируем тип действия.

		text — ответить текстом.
		file — отправить текст + ссылку на файл/картинку.
		api  — сходить во внешний API и отправить его ответ.
	*/
	if actionType != "text" && actionType != "file" && actionType != "api" {
		http.Error(w, "action_type must be text, file or api", http.StatusBadRequest)
		return
	}

	if actionType == "text" && responseText == "" {
		http.Error(w, "response_text is required for text action", http.StatusBadRequest)
		return
	}

	if actionType == "file" && fileURL == "" {
		http.Error(w, "file_url is required for file action", http.StatusBadRequest)
		return
	}

	if actionType == "api" && apiURL == "" {
		http.Error(w, "api_url is required for api action", http.StatusBadRequest)
		return
	}

	rule := storage.Rule{
		Name:         name,
		Keywords:     splitKeywords(keywordsRaw),
		ActionType:   actionType,
		ResponseText: responseText,
		FileURL:      fileURL,
		APIURL:       apiURL,
		IsActive:     isActive,
	}

	if err := s.store.CreateRule(rule); err != nil {
		http.Error(w, "Create rule error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

/*
	handleDeleteRule удаляет правило по ID.
*/
func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Parse form error: "+err.Error(), http.StatusBadRequest)
		return
	}

	idRaw := r.FormValue("id")

	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid rule id", http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteRule(id); err != nil {
		http.Error(w, "Delete rule error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

/*
	splitKeywords превращает строку:

	цена, прайс, стоимость

	в массив:

	[]string{"цена", "прайс", "стоимость"}
*/
func splitKeywords(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			result = append(result, value)
		}
	}

	return result
}

/*
	adminHTML — HTML мини-админки прямо внутри Go.

	Так проще стартовать:
	- не нужны отдельные шаблоны;
	- не нужно настраивать embed;
	- один файл сразу работает.

	Позже можно вынести в templates/admin.html.
*/
const adminHTML = `
<!doctype html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<title>Bitrix Auto Reply Admin</title>
	<style>
		body {
			font-family: Arial, sans-serif;
			background: #f5f7fb;
			margin: 0;
			padding: 30px;
			color: #222;
		}

		.container {
			max-width: 1200px;
			margin: 0 auto;
		}

		.card {
			background: white;
			border-radius: 12px;
			padding: 20px;
			margin-bottom: 20px;
			box-shadow: 0 2px 10px rgba(0,0,0,0.06);
		}

		h1, h2 {
			margin-top: 0;
		}

		table {
			width: 100%;
			border-collapse: collapse;
		}

		th, td {
			border-bottom: 1px solid #e5e7eb;
			padding: 10px;
			text-align: left;
			vertical-align: top;
		}

		th {
			background: #f9fafb;
		}

		input, select, textarea {
			width: 100%;
			padding: 10px;
			border: 1px solid #d1d5db;
			border-radius: 8px;
			box-sizing: border-box;
			font-size: 14px;
		}

		textarea {
			min-height: 80px;
		}

		label {
			display: block;
			font-weight: bold;
			margin-bottom: 6px;
		}

		.form-grid {
			display: grid;
			grid-template-columns: 1fr 1fr;
			gap: 16px;
		}

		.form-row {
			margin-bottom: 16px;
		}

		button {
			background: #2563eb;
			color: white;
			border: none;
			padding: 10px 16px;
			border-radius: 8px;
			cursor: pointer;
			font-weight: bold;
		}

		button.danger {
			background: #dc2626;
		}

		.badge {
			display: inline-block;
			padding: 4px 8px;
			border-radius: 999px;
			background: #e0f2fe;
			color: #0369a1;
			font-size: 12px;
			font-weight: bold;
		}

		.badge-off {
			background: #fee2e2;
			color: #991b1b;
		}

		.help {
			background: #fffbeb;
			border: 1px solid #fde68a;
			padding: 12px;
			border-radius: 8px;
			font-size: 14px;
			line-height: 1.5;
		}

		code {
			background: #f3f4f6;
			padding: 2px 5px;
			border-radius: 4px;
		}
	</style>
</head>
<body>
<div class="container">
	<div class="card">
		<h1>Bitrix Auto Reply Admin</h1>
		<p>Мини-админка правил автоответчика Bitrix24.</p>

		<div class="help">
			<b>Типы действий:</b><br>
			<code>text</code> — отправить текст из поля response_text.<br>
			<code>file</code> — отправить текст + ссылку на файл/картинку из file_url.<br>
			<code>api</code> — вызвать внешний API из api_url. API должен вернуть JSON: <code>{"message":"Текст ответа"}</code>.
		</div>
	</div>

	<div class="card">
		<h2>Добавить правило</h2>

		<form method="post" action="/admin/rules">
			<div class="form-grid">
				<div class="form-row">
					<label>Название</label>
					<input name="name" placeholder="Например: Цена" required>
				</div>

				<div class="form-row">
					<label>Тип действия</label>
					<select name="action_type" required>
						<option value="text">text — ответить текстом</option>
						<option value="file">file — отправить файл/картинку ссылкой</option>
						<option value="api">api — получить ответ из внешнего API</option>
					</select>
				</div>
			</div>

			<div class="form-row">
				<label>Ключевые слова через запятую</label>
				<input name="keywords" placeholder="цена, прайс, стоимость, сколько стоит" required>
			</div>

			<div class="form-row">
				<label>Текст ответа</label>
				<textarea name="response_text" placeholder="Привет! Сейчас посмотрю и отвечу."></textarea>
			</div>

			<div class="form-grid">
				<div class="form-row">
					<label>File URL</label>
					<input name="file_url" placeholder="https://site.ru/presentation.pdf">
				</div>

				<div class="form-row">
					<label>API URL</label>
					<input name="api_url" placeholder="https://api.site.ru/bitrix/reply">
				</div>
			</div>

			<div class="form-row">
				<label>
					<input type="checkbox" name="is_active" value="1" checked style="width:auto;">
					Активно
				</label>
			</div>

			<button type="submit">Создать правило</button>
		</form>
	</div>

	<div class="card">
		<h2>Правила</h2>

		<table>
			<thead>
			<tr>
				<th>ID</th>
				<th>Название</th>
				<th>Ключи</th>
				<th>Действие</th>
				<th>Ответ</th>
				<th>File URL</th>
				<th>API URL</th>
				<th>Статус</th>
				<th></th>
			</tr>
			</thead>
			<tbody>
			{{ range .Rules }}
			<tr>
				<td>{{ .ID }}</td>
				<td>{{ .Name }}</td>
				<td>{{ range .Keywords }}<div><code>{{ . }}</code></div>{{ end }}</td>
				<td><b>{{ .ActionType }}</b></td>
				<td>{{ .ResponseText }}</td>
				<td>{{ .FileURL }}</td>
				<td>{{ .APIURL }}</td>
				<td>
					{{ if .IsActive }}
						<span class="badge">active</span>
					{{ else }}
						<span class="badge badge-off">off</span>
					{{ end }}
				</td>
				<td>
					<form method="post" action="/admin/delete" onsubmit="return confirm('Удалить правило?');">
						<input type="hidden" name="id" value="{{ .ID }}">
						<button class="danger" type="submit">Удалить</button>
					</form>
				</td>
			</tr>
			{{ else }}
			<tr>
				<td colspan="9">Правил пока нет.</td>
			</tr>
			{{ end }}
			</tbody>
		</table>
	</div>
</div>
</body>
</html>
`

/*
	Debug helper, если понадобится быстро проверить сервер.
*/
func DebugURL(port string) string {
	return fmt.Sprintf("http://localhost:%s/admin", port)
}