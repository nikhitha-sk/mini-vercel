// Command worker consumes build jobs from the asynq queue and runs each one
// in an isolated Docker container.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/nikhitha-sk/mini-vercel/internal/builder"
	"github.com/nikhitha-sk/mini-vercel/internal/config"
	"github.com/nikhitha-sk/mini-vercel/internal/queue"
	"github.com/nikhitha-sk/mini-vercel/internal/secrets"
	"github.com/nikhitha-sk/mini-vercel/internal/storage"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
	"github.com/nikhitha-sk/mini-vercel/internal/tunnel"
	"github.com/nikhitha-sk/mini-vercel/internal/worker"
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

	box, err := secrets.New(cfg.EncryptionKey)
	must(err, "encryption key")
	sto, err := storage.New(ctx, cfg)
	must(err, "storage")
	must(sto.EnsureBucket(ctx), "ensure bucket")

	sb, err := builder.NewSandbox(cfg.BuildImage)
	must(err, "docker client")
	must(sb.Ping(ctx), "docker daemon unreachable")
	go func() {
		if err := sb.EnsureImage(ctx, cfg.BuildImage); err != nil {
			slog.Warn("pre-pull build image", "err", err)
		}
	}()
	// Janitor: remove orphaned build containers left by crashed workers.
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			sb.Janitor(ctx, cfg.BuildTimeout+10*time.Minute)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPass})
	defer rdb.Close()

	// Free public URLs (Cloudflare quick tunnels) for projects that opt in.
	if tm, err := tunnel.New(cfg, st, rdb); err != nil {
		slog.Warn("public URLs disabled", "err", err)
	} else {
		go tm.Run(ctx)
	}

	w := &worker.Worker{Cfg: cfg, Store: st, Redis: rdb, Storage: sto, Sandbox: sb, Secrets: box}
	srv := asynq.NewServer(asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPass}, asynq.Config{
		Concurrency:     cfg.WorkerConcurrent,
		Queues:          map[string]int{queue.QueueBuild: 1},
		RetryDelayFunc:  queue.RetryDelay,
		ErrorHandler:    asynq.ErrorHandlerFunc(w.ErrorHandler),
		ShutdownTimeout: 30 * time.Second,
		Logger:          asynqLogger{},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypeBuild, w.HandleBuild)

	slog.Info("worker started", "concurrency", cfg.WorkerConcurrent, "image", cfg.BuildImage)
	if err := srv.Start(mux); err != nil {
		must(err, "start worker")
	}
	<-ctx.Done()
	srv.Shutdown()
}

func must(err error, what string) {
	if err != nil {
		slog.Error("fatal: "+what, "err", err)
		os.Exit(1)
	}
}

type asynqLogger struct{}

func (asynqLogger) Debug(args ...any) {}
func (asynqLogger) Info(args ...any)  { slog.Info("asynq", "msg", args) }
func (asynqLogger) Warn(args ...any)  { slog.Warn("asynq", "msg", args) }
func (asynqLogger) Error(args ...any) { slog.Error("asynq", "msg", args) }
func (asynqLogger) Fatal(args ...any) { slog.Error("asynq fatal", "msg", args); os.Exit(1) }
