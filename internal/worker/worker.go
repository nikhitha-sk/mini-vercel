// Package worker consumes build jobs: clone -> install -> build inside a
// sandbox container -> upload output -> mark READY and promote the alias.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/nikhitha-sk/mini-vercel/internal/builder"
	"github.com/nikhitha-sk/mini-vercel/internal/config"
	"github.com/nikhitha-sk/mini-vercel/internal/ids"
	"github.com/nikhitha-sk/mini-vercel/internal/logstream"
	"github.com/nikhitha-sk/mini-vercel/internal/queue"
	"github.com/nikhitha-sk/mini-vercel/internal/secrets"
	"github.com/nikhitha-sk/mini-vercel/internal/storage"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

type Worker struct {
	Cfg     config.Config
	Store   *store.Store
	Redis   *redis.Client
	Storage *storage.Storage
	Sandbox *builder.Sandbox
	Secrets *secrets.Box
}

var errCanceled = errors.New("canceled by user")

func (w *Worker) HandleBuild(ctx context.Context, t *asynq.Task) error {
	var p queue.BuildPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("bad payload: %v: %w", err, asynq.SkipRetry)
	}
	log := slog.With("deployment", p.DeploymentID)

	// At-least-once delivery: a job can run twice. MarkBuilding only succeeds
	// from a non-terminal state, so a READY/FAILED/CANCELED deployment is a no-op.
	d, ok, err := w.Store.MarkBuilding(ctx, p.DeploymentID)
	if err != nil {
		return err
	}
	if !ok {
		log.Info("deployment already finished, skipping")
		return nil
	}
	w.publishStatus(d.ID)

	logger, err := logstream.NewLogger(ctx, d.ID, w.Redis, w.Store)
	if err != nil {
		return err
	}
	defer logger.Close()

	retry, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	if d.Attempt > 1 {
		logger.System(fmt.Sprintf("Retrying build (attempt %d of %d)", d.Attempt, maxRetry+1))
	}

	// Listen for user cancellation on cancel:<id>.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	sub := w.Redis.Subscribe(ctx, logstream.CancelChannel(d.ID))
	defer sub.Close()
	go func() {
		if _, err := sub.ReceiveMessage(ctx); err == nil {
			cancel(errCanceled)
		}
	}()

	start := time.Now()
	files, size, err := w.build(ctx, p, d, logger)
	if context.Cause(ctx) == errCanceled {
		logger.System("Build canceled.")
		w.publishStatus(d.ID)
		return nil
	}
	if err != nil {
		if builder.IsUserError(err) {
			logger.Line("stderr", "Error: "+err.Error())
			w.finish(d.ID, store.StatusFailed, err.Error(), 0, 0)
			return fmt.Errorf("%v: %w", err, asynq.SkipRetry)
		}
		// Infrastructure error: let asynq retry with backoff, unless this was
		// the last attempt.
		if retry >= maxRetry {
			logger.Line("stderr", "Error: "+err.Error())
			w.finish(d.ID, store.StatusFailed, "Infrastructure error after retries: "+err.Error(), 0, 0)
			return fmt.Errorf("%v: %w", err, asynq.SkipRetry)
		}
		logger.Line("stderr", fmt.Sprintf("Infrastructure error: %v (will retry)", err))
		_ = w.Store.SetStatus(context.Background(), d.ID, store.StatusQueued)
		w.publishStatus(d.ID)
		return err
	}

	// READY + promote alias in one step from the user's point of view.
	if !w.finish(d.ID, store.StatusReady, "", files, size) {
		return nil // canceled concurrently
	}
	proj, err := w.Store.GetProject(context.Background(), d.ProjectID)
	if err == nil {
		host := w.Cfg.Hostname(proj.Slug)
		if err := w.Store.Promote(context.Background(), host, proj.ID, d.ID); err != nil {
			log.Error("promote", "err", err)
		} else {
			w.Redis.Publish(context.Background(), logstream.EdgeInvalidateChannel, host)
			logger.System(fmt.Sprintf("Assigned production alias %s", w.Cfg.PublicURL(proj.Slug)))
		}
		_ = w.Store.TouchProject(context.Background(), proj.ID)
	}
	w.Redis.Publish(context.Background(), logstream.EdgeInvalidateChannel, w.Cfg.Hostname(ids.DeploymentLabel(d.ID)))
	logger.System(fmt.Sprintf("Deployment ready in %s at %s", time.Since(start).Round(100*time.Millisecond), w.Cfg.PublicURL(ids.DeploymentLabel(d.ID))))
	w.publishStatus(d.ID)
	log.Info("deployment ready", "files", files, "bytes", size, "took", time.Since(start))
	return nil
}

func (w *Worker) build(ctx context.Context, p queue.BuildPayload, d store.Deployment, logger *logstream.Logger) (int, int64, error) {
	timeout := time.Duration(p.Limits.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = w.Cfg.BuildTimeout
	}
	buildCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 1. Fetch source on the worker (never inside the sandbox).
	tmp, err := os.MkdirTemp("", "mv-src-*")
	if err != nil {
		return 0, 0, err
	}
	defer os.RemoveAll(tmp)

	srcDir := tmp
	switch p.Source.Type {
	case "zip":
		logger.System("Downloading uploaded source archive")
		obj, err := w.Storage.Get(buildCtx, p.Source.ZipKey)
		if err != nil {
			return 0, 0, fmt.Errorf("download source: %w", err)
		}
		data, err := io.ReadAll(obj.Body)
		obj.Body.Close()
		if err != nil {
			return 0, 0, err
		}
		if srcDir, err = builder.ExtractZip(data, tmp); err != nil {
			return 0, 0, err
		}
		logger.System(fmt.Sprintf("Extracted %s archive", humanBytes(int64(len(data)))))
	default:
		logger.System(fmt.Sprintf("Cloning %s (Branch: %s)", strings.TrimPrefix(p.Source.Repo, "https://"), p.Source.Branch))
		cloneStart := time.Now()
		commit, err := builder.Clone(buildCtx, p.Source.Repo, p.Source.Branch, tmp, logger)
		if err != nil {
			return 0, 0, err
		}
		short := commit.SHA
		if len(short) > 7 {
			short = short[:7]
		}
		logger.System(fmt.Sprintf("Cloned in %s. Commit %s: %s", time.Since(cloneStart).Round(time.Millisecond), short, commit.Message))
		_ = w.Store.SetCommit(ctx, d.ID, commit.SHA, commit.Message)
	}

	// 2. Decrypt env vars; plaintext only goes into the sandbox.
	env, err := w.envFor(ctx, d.ProjectID)
	if err != nil {
		return 0, 0, err
	}

	// 3. Start the sandbox.
	logger.System(fmt.Sprintf("Starting build container (%s · %g vCPU · %d MB · %ds limit)", p.Build.Image, p.Limits.CPUs, p.Limits.MemoryMB, int(timeout.Seconds())))
	c, err := w.Sandbox.Create(buildCtx, builder.CreateOptions{
		DeploymentID: d.ID,
		Image:        p.Build.Image,
		Env:          env,
		RootDir:      p.Build.RootDir,
		Limits:       builder.Limits{CPUs: p.Limits.CPUs, MemoryMB: p.Limits.MemoryMB, PIDs: p.Limits.PIDs},
	})
	if err != nil {
		return 0, 0, err
	}
	defer c.Remove() // container never outlives the job
	if err := c.CopySource(buildCtx, srcDir); err != nil {
		return 0, 0, err
	}
	if err := c.Start(buildCtx); err != nil {
		return 0, 0, err
	}
	if len(env) > 0 {
		logger.System(fmt.Sprintf("Injected %d environment variable(s)", len(env)))
	}

	// 4. install -> build
	install, build := p.Build.Install, p.Build.Command
	if install == "" && build != "" {
		install = builder.AutoInstall
	}
	if install != "" {
		label := install
		if install == builder.AutoInstall {
			label = "auto-detected package manager"
		}
		logger.System("Running install command: " + label)
		if err := c.Exec(buildCtx, install, logger); err != nil {
			return 0, 0, w.wrapCtx(buildCtx, err)
		}
	}
	if build != "" {
		logger.System(fmt.Sprintf("Running build command: %q", build))
		if err := c.Exec(buildCtx, build, logger); err != nil {
			return 0, 0, w.wrapCtx(buildCtx, err)
		}
	} else {
		logger.System("No build command, deploying files as-is")
	}

	// 5. Upload output to <deployId>/...
	if err := w.Store.SetStatus(ctx, d.ID, store.StatusUploading); err != nil {
		return 0, 0, err
	}
	w.publishStatus(d.ID)
	logger.System(fmt.Sprintf("Uploading build output from %q", p.Build.OutputDir))
	files, size, err := w.upload(buildCtx, c, d.ID, p.Build.OutputDir)
	if err != nil {
		return 0, 0, w.wrapCtx(buildCtx, err)
	}
	logger.System(fmt.Sprintf("Uploaded %d files (%s)", files, humanBytes(size)))
	return files, size, nil
}

// upload streams output files from the container to object storage with a
// small pool of concurrent PUTs. Keys are deterministic, so a retried job
// simply overwrites the same objects.
func (w *Worker) upload(ctx context.Context, c *builder.Container, deploymentID, outputDir string) (int, int64, error) {
	type item struct {
		key  string
		data []byte
	}
	ch := make(chan item, 16)
	var (
		wg       sync.WaitGroup
		firstErr atomic.Value
		count    atomic.Int64
		size     atomic.Int64
	)
	upCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				if err := w.Storage.Put(upCtx, it.key, it.data, ""); err != nil {
					firstErr.CompareAndSwap(nil, err)
					cancel()
					continue
				}
				count.Add(1)
				size.Add(int64(len(it.data)))
			}
		}()
	}
	walkErr := c.WalkOutput(ctx, outputDir, func(rel string, n int64, r io.Reader) error {
		var buf bytes.Buffer
		buf.Grow(int(n))
		if _, err := io.Copy(&buf, r); err != nil {
			return err
		}
		select {
		case ch <- item{key: deploymentID + "/" + rel, data: buf.Bytes()}:
			return nil
		case <-upCtx.Done():
			return upCtx.Err()
		}
	})
	close(ch)
	wg.Wait()
	if walkErr != nil {
		if e, ok := firstErr.Load().(error); ok && e != nil {
			return 0, 0, fmt.Errorf("upload: %w", e)
		}
		return 0, 0, walkErr
	}
	if e, ok := firstErr.Load().(error); ok && e != nil {
		return 0, 0, fmt.Errorf("upload: %w", e)
	}
	return int(count.Load()), size.Load(), nil
}

func (w *Worker) envFor(ctx context.Context, projectID string) ([]string, error) {
	vars, err := w.Store.ListEnvVars(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(vars))
	for _, v := range vars {
		val, err := w.Secrets.Open(v.Ciphertext, v.Nonce, projectID+"/"+v.Key)
		if err != nil {
			return nil, &builder.UserError{Msg: fmt.Sprintf("Could not decrypt environment variable %s", v.Key)}
		}
		out = append(out, v.Key+"="+val)
	}
	return out, nil
}

// wrapCtx turns a build-context deadline into a user-facing timeout error.
func (w *Worker) wrapCtx(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && !builder.IsUserError(err) {
		return &builder.UserError{Msg: "Build exceeded the time limit and was stopped"}
	}
	return err
}

func (w *Worker) finish(id string, st store.Status, msg string, files int, size int64) bool {
	ok, err := w.Store.Finish(context.Background(), id, st, msg, files, size)
	if err != nil {
		slog.Error("finish deployment", "deployment", id, "err", err)
	}
	w.publishStatus(id)
	return ok
}

func (w *Worker) publishStatus(id string) {
	d, err := w.Store.GetDeployment(context.Background(), id)
	if err != nil {
		return
	}
	logstream.Publish(context.Background(), w.Redis, id, logstream.Event{Type: "status", Status: d.Status, Error: d.Error, Deployment: &d})
}

// ErrorHandler marks a deployment FAILED if asynq gives up on it for a reason
// the handler did not catch itself (panic, task timeout).
func (w *Worker) ErrorHandler(ctx context.Context, t *asynq.Task, err error) {
	retry, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	if retry < maxRetry && !errors.Is(err, asynq.SkipRetry) {
		return
	}
	var p queue.BuildPayload
	if json.Unmarshal(t.Payload(), &p) != nil {
		return
	}
	if ok, _ := w.Store.Finish(context.Background(), p.DeploymentID, store.StatusFailed, err.Error(), 0, 0); ok {
		w.publishStatus(p.DeploymentID)
	}
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
