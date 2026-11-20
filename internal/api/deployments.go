package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"

	"github.com/nikhitha-sk/mini-vercel/internal/ids"
	"github.com/nikhitha-sk/mini-vercel/internal/logstream"
	"github.com/nikhitha-sk/mini-vercel/internal/queue"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

const maxZipUpload = 100 << 20 // 100 MB

type deploymentView struct {
	store.Deployment
	URL          string `json:"url"`
	IsProduction bool   `json:"isProduction"`
	DurationMs   *int64 `json:"durationMs"`
}

func (s *Server) deploymentView(d store.Deployment, productionID string) deploymentView {
	v := deploymentView{Deployment: d, URL: s.Cfg.PublicURL(ids.DeploymentLabel(d.ID)), IsProduction: productionID != "" && productionID == d.ID}
	if d.StartedAt != nil {
		end := time.Now()
		if d.FinishedAt != nil {
			end = *d.FinishedAt
		}
		ms := end.Sub(*d.StartedAt).Milliseconds()
		v.DurationMs = &ms
	}
	return v
}

func zipKey(deploymentID string) string { return "_sources/" + deploymentID + ".zip" }

// enqueueDeployment snapshots the project's build config into an immutable
// deployment row (QUEUED) and enqueues exactly one asynq job for it.
func (s *Server) enqueueDeployment(ctx context.Context, p store.Project, source, trigger, sha, msg string) (store.Deployment, error) {
	return s.enqueueWithID(ctx, ids.Deployment(), p, source, trigger, sha, msg)
}

func (s *Server) enqueueWithID(ctx context.Context, id string, p store.Project, source, trigger, sha, msg string) (store.Deployment, error) {
	d, err := s.Store.CreateDeployment(ctx, store.Deployment{
		ID: id, ProjectID: p.ID, Source: source, Trigger: trigger, Branch: p.Branch,
		CommitSHA: sha, CommitMessage: msg,
		InstallCmd: p.InstallCmd, BuildCmd: p.BuildCmd, OutputDir: p.OutputDir, RootDir: p.RootDir,
	})
	if err != nil {
		return d, err
	}
	payload := queue.BuildPayload{
		DeploymentID: d.ID,
		ProjectID:    p.ID,
		Source:       queue.Source{Type: source, Repo: p.RepoURL, Branch: p.Branch, Commit: sha},
		Build:        queue.Build{Image: s.Cfg.BuildImage, Install: p.InstallCmd, Command: p.BuildCmd, OutputDir: p.OutputDir, RootDir: p.RootDir},
		Limits: queue.Limits{
			CPUs: s.Cfg.BuildCPUs, MemoryMB: s.Cfg.BuildMemoryMB, PIDs: s.Cfg.BuildPIDs,
			TimeoutSec: int(s.Cfg.BuildTimeout.Seconds()),
		},
	}
	if source == "zip" {
		payload.Source = queue.Source{Type: "zip", ZipKey: zipKey(d.ID)}
	}
	task, err := queue.NewBuildTask(payload)
	if err != nil {
		return d, err
	}
	if _, err := s.Queue.EnqueueContext(ctx, task); err != nil {
		_, _ = s.Store.Finish(context.Background(), d.ID, store.StatusFailed, "Could not enqueue build: "+err.Error(), 0, 0)
		return d, err
	}
	_ = s.Store.TouchProject(ctx, p.ID)
	return d, nil
}

func (s *Server) productionID(ctx context.Context, p store.Project) string {
	id, _ := s.Store.ResolveHost(ctx, s.Cfg.Hostname(p.Slug))
	return id
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	deps, err := s.Store.ListDeployments(r.Context(), p.ID, limit)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	prod := s.productionID(r.Context(), p)
	out := make([]deploymentView, 0, len(deps))
	for _, d := range deps {
		out = append(out, s.deploymentView(d, prod))
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleRecentDeployments(w http.ResponseWriter, r *http.Request) {
	deps, err := s.Store.ListRecentDeployments(r.Context(), s.UserID, 20)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	type row struct {
		deploymentView
		ProjectName string `json:"projectName"`
		ProjectSlug string `json:"projectSlug"`
	}
	cache := map[string]store.Project{}
	out := make([]row, 0, len(deps))
	for _, d := range deps {
		p, ok := cache[d.ProjectID]
		if !ok {
			p, _ = s.Store.GetProject(r.Context(), d.ProjectID)
			cache[d.ProjectID] = p
		}
		out = append(out, row{deploymentView: s.deploymentView(d, s.productionID(r.Context(), p)), ProjectName: p.Name, ProjectSlug: p.Slug})
	}
	writeJSON(w, 200, out)
}

// handleCreateDeployment accepts either JSON (deploy from the project's Git
// repo) or multipart/form-data with a "file" field (zip upload).
func (s *Server) handleCreateDeployment(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, maxZipUpload+1<<20)
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, 400, "bad_request", "Attach a .zip file in the \"file\" field (max 100 MB)")
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxZipUpload+1))
		if err != nil || len(data) > maxZipUpload {
			writeErr(w, 413, "too_large", "Zip archive must be 100 MB or smaller")
			return
		}
		if len(data) < 4 || string(data[:2]) != "PK" {
			writeErr(w, 422, "invalid_zip", "Uploaded file is not a zip archive")
			return
		}
		id := ids.Deployment()
		if err := s.Storage.Put(r.Context(), zipKey(id), data, "application/zip"); err != nil {
			s.internal(w, r, err)
			return
		}
		d, err := s.enqueueWithID(r.Context(), id, p, "zip", "api", "", "Uploaded archive")
		if err != nil {
			s.internal(w, r, err)
			return
		}
		writeJSON(w, 201, s.deploymentView(d, ""))
		return
	}

	if p.RepoURL == "" {
		writeErr(w, 422, "no_repo", "This project has no Git repository. Upload a zip archive instead")
		return
	}
	var in struct {
		Trigger string `json:"trigger"`
	}
	_ = decode(r, &in)
	trigger := "manual"
	if in.Trigger == "api" {
		trigger = "api"
	}
	d, err := s.enqueueDeployment(r.Context(), p, "git", trigger, "", "")
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, 201, s.deploymentView(d, ""))
}

func (s *Server) loadDeployment(w http.ResponseWriter, r *http.Request) (store.Deployment, store.Project, bool) {
	d, err := s.Store.GetDeployment(r.Context(), chi.URLParam(r, "deploymentID"))
	if err != nil {
		s.internal(w, r, err)
		return d, store.Project{}, false
	}
	p, err := s.Store.GetProject(r.Context(), d.ProjectID)
	if err != nil || p.OwnerID != s.UserID {
		writeErr(w, 404, "not_found", "Deployment not found")
		return d, p, false
	}
	return d, p, true
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	d, p, ok := s.loadDeployment(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{
		"deployment": s.deploymentView(d, s.productionID(r.Context(), p)),
		"project":    map[string]any{"id": p.ID, "name": p.Name, "slug": p.Slug, "repoUrl": p.RepoURL, "productionUrl": s.Cfg.PublicURL(p.Slug)},
	})
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	d, _, ok := s.loadDeployment(w, r)
	if !ok {
		return
	}
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	lines, err := s.Store.ListLogs(r.Context(), d.ID, after)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, 200, lines)
}

// handleEvents streams logs and status changes over Server-Sent Events:
// subscribe to Redis first, replay stored lines from Postgres, then relay
// live events (deduplicated by sequence number) until a terminal state.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	d, p, ok := s.loadDeployment(w, r)
	if !ok {
		return
	}
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)

	ctx := r.Context()
	sub := s.Redis.Subscribe(ctx, logstream.LogChannel(d.ID))
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil { // wait for subscription confirmation
		return
	}
	msgs := sub.Channel()

	send := func(event string, v any) bool {
		b, _ := json.Marshal(v)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	lastSeq := 0
	lines, err := s.Store.ListLogs(ctx, d.ID, 0)
	if err != nil {
		return
	}
	for _, l := range lines {
		if !send("log", l) {
			return
		}
		lastSeq = l.Seq
	}
	// Re-read status after replay so we do not miss a transition.
	if d, err = s.Store.GetDeployment(ctx, d.ID); err != nil {
		return
	}
	prod := s.productionID(ctx, p)
	send("status", s.deploymentView(d, prod))
	if d.Status.Terminal() {
		send("end", map[string]any{"status": d.Status})
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		case m, ok := <-msgs:
			if !ok {
				return
			}
			var ev logstream.Event
			if json.Unmarshal([]byte(m.Payload), &ev) != nil {
				continue
			}
			switch ev.Type {
			case "log":
				if ev.Log == nil || ev.Log.Seq <= lastSeq {
					continue
				}
				lastSeq = ev.Log.Seq
				if !send("log", ev.Log) {
					return
				}
			case "status":
				if ev.Deployment == nil {
					continue
				}
				dv := s.deploymentView(*ev.Deployment, s.productionID(ctx, p))
				if !send("status", dv) {
					return
				}
				if ev.Deployment.Status.Terminal() {
					// Give the worker's final log lines a moment to arrive.
					drain := time.After(750 * time.Millisecond)
				loop:
					for {
						select {
						case m := <-msgs:
							var e2 logstream.Event
							if json.Unmarshal([]byte(m.Payload), &e2) == nil && e2.Type == "log" && e2.Log != nil && e2.Log.Seq > lastSeq {
								lastSeq = e2.Log.Seq
								send("log", e2.Log)
							}
						case <-drain:
							break loop
						}
					}
					send("end", map[string]any{"status": ev.Deployment.Status})
					return
				}
			}
		}
	}
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	d, p, ok := s.loadDeployment(w, r)
	if !ok {
		return
	}
	changed, err := s.Store.Cancel(r.Context(), d.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if !changed {
		writeErr(w, 409, "not_cancelable", "Only queued or building deployments can be canceled")
		return
	}
	s.Redis.Publish(r.Context(), logstream.CancelChannel(d.ID), "cancel")
	d, _ = s.Store.GetDeployment(r.Context(), d.ID)
	logstream.Publish(r.Context(), s.Redis, d.ID, logstream.Event{Type: "status", Status: d.Status, Deployment: &d})
	writeJSON(w, 200, s.deploymentView(d, s.productionID(r.Context(), p)))
}

// handlePromote points the production alias at any READY deployment.
// This is both "promote" and "instant rollback": a single UPSERT.
func (s *Server) handlePromote(w http.ResponseWriter, r *http.Request) {
	d, p, ok := s.loadDeployment(w, r)
	if !ok {
		return
	}
	if d.Status != store.StatusReady {
		writeErr(w, 409, "not_ready", "Only READY deployments can be promoted")
		return
	}
	start := time.Now()
	host := s.Cfg.Hostname(p.Slug)
	if err := s.Store.Promote(r.Context(), host, p.ID, d.ID); err != nil {
		s.internal(w, r, err)
		return
	}
	s.invalidateHost(host)
	writeJSON(w, 200, map[string]any{
		"deployment": s.deploymentView(d, d.ID),
		"hostname":   host,
		"tookMs":     float64(time.Since(start).Microseconds()) / 1000,
	})
}

func (s *Server) handleRedeploy(w http.ResponseWriter, r *http.Request) {
	d, p, ok := s.loadDeployment(w, r)
	if !ok {
		return
	}
	var nd store.Deployment
	var err error
	if d.Source == "zip" {
		obj, gerr := s.Storage.Get(r.Context(), zipKey(d.ID))
		if gerr != nil {
			writeErr(w, 410, "source_gone", "The source archive for this deployment is no longer available")
			return
		}
		data, rerr := io.ReadAll(obj.Body)
		obj.Body.Close()
		if rerr != nil {
			s.internal(w, r, rerr)
			return
		}
		id := ids.Deployment()
		if err := s.Storage.Put(r.Context(), zipKey(id), data, "application/zip"); err != nil {
			s.internal(w, r, err)
			return
		}
		nd, err = s.enqueueWithID(r.Context(), id, p, "zip", "redeploy", "", d.CommitMessage)
	} else {
		if p.RepoURL == "" {
			writeErr(w, 422, "no_repo", "This project has no Git repository")
			return
		}
		nd, err = s.enqueueDeployment(r.Context(), p, "git", "redeploy", "", "")
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, 201, s.deploymentView(nd, ""))
}

func (s *Server) invalidateHost(host string) {
	s.Redis.Publish(context.Background(), logstream.EdgeInvalidateChannel, host)
}

// handleGitHubWebhook deploys on push to the project's branch. Configure a
// GitHub webhook (content type application/json) with the project's secret.
func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.GetProject(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		writeErr(w, 404, "not_found", "Unknown project")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		writeErr(w, 400, "bad_request", "Could not read body")
		return
	}
	mac := hmac.New(sha256.New, []byte(p.WebhookSecret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(r.Header.Get("X-Hub-Signature-256"))) {
		writeErr(w, 401, "bad_signature", "Signature mismatch")
		return
	}
	switch r.Header.Get("X-GitHub-Event") {
	case "ping":
		writeJSON(w, 200, map[string]any{"ok": true, "message": "pong"})
		return
	case "push":
	default:
		writeJSON(w, 202, map[string]any{"ignored": "event"})
		return
	}
	var ev struct {
		Ref        string `json:"ref"`
		After      string `json:"after"`
		Deleted    bool   `json:"deleted"`
		HeadCommit *struct {
			Message string `json:"message"`
		} `json:"head_commit"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		writeErr(w, 400, "bad_request", "Invalid JSON")
		return
	}
	if ev.Deleted || ev.Ref != "refs/heads/"+p.Branch {
		writeJSON(w, 202, map[string]any{"ignored": "branch"})
		return
	}
	msg := ""
	if ev.HeadCommit != nil {
		msg = strings.SplitN(ev.HeadCommit.Message, "\n", 2)[0]
	}
	d, err := s.enqueueDeployment(r.Context(), p, "git", "webhook", ev.After, msg)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskIDConflict) {
			writeJSON(w, 202, map[string]any{"ignored": "duplicate"})
			return
		}
		s.internal(w, r, err)
		return
	}
	writeJSON(w, 201, s.deploymentView(d, ""))
}
