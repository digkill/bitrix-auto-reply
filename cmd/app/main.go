package main

import (
	"log"
	"net/http"

	"github.com/digkill/bitrix-auto-reply/internal/admin"
	"github.com/digkill/bitrix-auto-reply/internal/bitrix"
	"github.com/digkill/bitrix-auto-reply/internal/config"
	"github.com/digkill/bitrix-auto-reply/internal/storage"
	"github.com/digkill/bitrix-auto-reply/internal/worker"
)

/*
	main — точка входа приложения.

	Что запускается:
	1. Загружаем .env.
	2. Подключаемся к MySQL.
	3. Создаём Bitrix24 REST-клиент.
	4. Запускаем worker в отдельной goroutine.
	5. Запускаем HTTP-админку.
*/
func main() {
	cfg := config.Load()

	store, err := storage.NewStorage(cfg.DBDSN)
	if err != nil {
		log.Fatalf("storage init error: %v", err)
	}

	bitrixClient := bitrix.NewClient(cfg.BitrixWebhookBase)

	autoReplyWorker := worker.NewWorker(
		bitrixClient,
		store,
		cfg.BitrixSelfUserID,
		cfg.PollIntervalSeconds,
		cfg.DialogCooldownSeconds,
	)

	/*
		Worker запускаем в отдельной goroutine,
		чтобы он работал параллельно с HTTP-админкой.

		Если запустить просто autoReplyWorker.Run(),
		код ниже уже никогда не выполнится.
	*/
	go autoReplyWorker.Run()

	mux := http.NewServeMux()

	adminServer := admin.NewServer(
		store,
		cfg.AdminLogin,
		cfg.AdminPassword,
	)

	adminServer.RegisterRoutes(mux)

	/*
		Простой healthcheck.

		Можно использовать для Docker, Nginx, Supervisor:
		curl http://localhost:8080/health
	*/
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	addr := ":" + cfg.AppPort

	log.Printf("admin started: http://localhost%s/admin", addr)
	log.Printf("healthcheck: http://localhost%s/health", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("http server error: %v", err)
	}
}