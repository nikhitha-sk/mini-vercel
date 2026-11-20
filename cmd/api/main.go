// Command api runs the Mini Vercel control-plane API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/nikhitha-sk/mini-vercel/internal/api"
	"github.com/nikhitha-sk/mini-vercel/internal/config"
	"github.com/nikhitha-sk/mini-vercel/internal/secrets"
	"github.com/nikhitha-sk/mini-vercel/internal/storage"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	must(err, "connect postgres")
	defer st.Close()
	must(st.Migrate(ctx), "migrate")
	user, err := st.EnsureLocalUser(ctx, "usr_local")
	must(err, "ensure user")

	box, err := secrets.New(cfg.EncryptionKey)
	must(err, "encryption key")

	sto, err := storage.New(ctx, cfg)
	must(err, "storage")
	must(sto.EnsureBucket(ctx), "ensure bucket")

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPass})
	defer rdb.Close()
	q := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPass})
	defer q.Close()

	srv := &api.Server{Cfg: cfg, Store: st, Redis: rdb, Queue: q, Storage: sto, Secrets: box, UserID: user.ID}
	httpSrv := &http.Server{Addr: cfg.APIAddr, Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		slog.Info("api listening", "addr", cfg.APIAddr, "auth", cfg.APIToken != "")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			must(err, "listen")
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdown)
}

func must(err error, what string) {
	if err != nil {
		slog.Error("fatal: "+what, "err", err)
		os.Exit(1)
	}
}
