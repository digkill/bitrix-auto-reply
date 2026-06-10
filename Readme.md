# Bitrix Auto Reply

Автоответчик для личных сообщений Bitrix24.

Приложение запускает:

- worker, который опрашивает Bitrix24 REST API;
- мини-админку для правил автоответов;
- HTTP healthcheck.

## Требования

- Go 1.24+
- MySQL 8+
- `goose` для миграций
- входящий webhook Bitrix24 с правом `im` (`Чат и уведомления`)

## Настройка Bitrix24

Создай входящий webhook в Bitrix24:

`Разработчикам` -> `Другое` -> `Входящий вебхук`

Обязательно добавь право:

`Чат и уведомления (im)`

Без этого worker будет получать ошибку:

```text
insufficient_scope: The request requires higher privileges than provided by the webhook token
```

Webhook должен иметь формат:

```text
https://your-company.bitrix24.ru/rest/USER_ID/WEBHOOK_TOKEN/
```

`USER_ID` из этого URL обычно и есть значение `BITRIX_SELF_USER_ID`.

## Локальный запуск

Подними MySQL:

```bash
docker compose up -d mysql
```

Создай `.env`:

```bash
cp .env.example .env
```

Заполни в `.env` реальные значения:

```env
BITRIX_WEBHOOK_BASE=https://your-company.bitrix24.ru/rest/1/your_webhook_token/
BITRIX_SELF_USER_ID=1
DB_DSN=root:secret@tcp(127.0.0.1:3306)/bitrix_auto_reply?parseTime=true&charset=utf8mb4
ADMIN_LOGIN=admin
ADMIN_PASSWORD=change_me
```

Установи `goose`, если он ещё не установлен:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```

Примени миграции:

```bash
goose -dir ./internal/migrations mysql "root:secret@tcp(127.0.0.1:3306)/bitrix_auto_reply?parseTime=true&charset=utf8mb4" up
```

Запусти приложение:

```bash
go run ./cmd/app
```

Админка:

```text
http://localhost:8080/admin
```

Healthcheck:

```text
http://localhost:8080/health
```

## Сборка

```bash
go build -o bitrix-auto-reply ./cmd/app
```

## Установка как systemd daemon

Пример ниже рассчитан на Linux-сервер и установку в `/opt/bitrix-auto-reply`.

Создай пользователя:

```bash
sudo useradd --system --user-group --home-dir /opt/bitrix-auto-reply --shell /usr/sbin/nologin bitrix-auto-reply
```

Создай каталоги:

```bash
sudo mkdir -p /opt/bitrix-auto-reply /etc/bitrix-auto-reply
```

Собери бинарник:

```bash
go build -o bitrix-auto-reply ./cmd/app
```

Скопируй бинарник:

```bash
sudo cp ./bitrix-auto-reply /opt/bitrix-auto-reply/bitrix-auto-reply
sudo chown -R bitrix-auto-reply:bitrix-auto-reply /opt/bitrix-auto-reply
sudo chmod 755 /opt/bitrix-auto-reply/bitrix-auto-reply
```

Скопируй env-файл:

```bash
sudo cp .env.example /etc/bitrix-auto-reply/bitrix-auto-reply.env
sudo chmod 600 /etc/bitrix-auto-reply/bitrix-auto-reply.env
sudo chown root:root /etc/bitrix-auto-reply/bitrix-auto-reply.env
```

Отредактируй реальные значения:

```bash
sudo nano /etc/bitrix-auto-reply/bitrix-auto-reply.env
```

Примени миграции на серверной базе:

```bash
goose -dir ./internal/migrations mysql "USER:PASSWORD@tcp(127.0.0.1:3306)/bitrix_auto_reply?parseTime=true&charset=utf8mb4" up
```

Установи systemd unit:

```bash
sudo cp deploy/systemd/bitrix-auto-reply.service /etc/systemd/system/bitrix-auto-reply.service
sudo systemctl daemon-reload
sudo systemctl enable bitrix-auto-reply
sudo systemctl start bitrix-auto-reply
```

Проверка статуса:

```bash
sudo systemctl status bitrix-auto-reply
```

Логи:

```bash
sudo journalctl -u bitrix-auto-reply -f
```

Перезапуск после изменения env:

```bash
sudo systemctl restart bitrix-auto-reply
```

## Переменные окружения

| Переменная | Описание |
| --- | --- |
| `APP_PORT` | Порт админки и healthcheck. По умолчанию `8080`. |
| `BITRIX_WEBHOOK_BASE` | Базовый URL входящего webhook Bitrix24. |
| `BITRIX_SELF_USER_ID` | ID пользователя, от имени которого создан webhook. |
| `DB_DSN` | MySQL DSN. |
| `POLL_INTERVAL_SECONDS` | Интервал опроса Bitrix24. |
| `DIALOG_COOLDOWN_SECONDS` | Минимальная пауза между автоответами в одном диалоге. |
| `ADMIN_LOGIN` | Логин Basic Auth для админки. |
| `ADMIN_PASSWORD` | Пароль Basic Auth для админки. |

## Методы Bitrix24

Приложение использует REST-методы:

- `im.recent.list`
- `im.dialog.messages.get`
- `im.message.add`

Все они требуют webhook scope `im`.
