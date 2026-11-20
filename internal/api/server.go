// Package api is the control-plane HTTP API: projects, deployments, env
// vars, rollback, live logs (SSE) and GitHub webhooks. It never builds
// anything itself; it records state and enqueues jobs.
package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/nikhitha-sk/mini-vercel/internal/config"
	"github.com/nikhitha-sk/mini-vercel/internal/publish"
	"github.com/nikhitha-sk/mini-vercel/internal/secrets"
	"github.com/nikhitha-sk/mini-vercel/internal/storage"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

type Server struct {
	Cfg     config.Config
	Store   *store.Store
	Redis   *redis.Client
	Queue   *asynq.Client
	Storage *storage.Storage
	Secrets *secrets.Box
	Publish *publish.Publisher
	UserID  string // single local user until GitHub OAuth lands
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer)
	r.Use(requestLogger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.Cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 200, map[string]any{"ok": true, "time": time.Now().UTC()})
		})
		r.Get("/config", s.handleConfig)
		r.Post("/webhooks/github/{projectID}", s.handleGitHubWebhook) // HMAC-authenticated

		r.Group(func(r chi.Router) {
			r.Use(s.auth)
			r.Get("/presets", s.handlePresets)
			r.Get("/stats", s.handleStats)
			r.Get("/deployments", s.handleRecentDeployments)

			r.Get("/projects", s.handleListProjects)
			r.Post("/projects", s.handleCreateProject)
			r.Get("/projects/{projectID}", s.handleGetProject)
			r.Patch("/projects/{projectID}", s.handleUpdateProject)
			r.Delete("/projects/{projectID}", s.handleDeleteProject)
			r.Post("/projects/{projectID}/public", s.handleSetPublic)
			r.Post("/projects/{projectID}/publish", s.handlePublishProject)
			r.Get("/integrations/netlify", s.handleNetlifyStatus)
			r.Put("/integrations/netlify", s.handleNetlifyConnect)
			r.Delete("/integrations/netlify", s.handleNetlifyDisconnect)

			r.Get("/projects/{projectID}/env", s.handleListEnv)
			r.Put("/projects/{projectID}/env", s.handleSetEnv)
			r.Delete("/projects/{projectID}/env/{key}", s.handleDeleteEnv)

			r.Get("/projects/{projectID}/deployments", s.handleListDeployments)
			r.Post("/projects/{projectID}/deployments", s.handleCreateDeployment)

			r.Get("/deployments/{deploymentID}", s.handleGetDeployment)
			r.Get("/deployments/{deploymentID}/logs", s.handleGetLogs)
			r.Get("/deployments/{deploymentID}/events", s.handleEvents)
			r.Post("/deployments/{deploymentID}/cancel", s.handleCancel)
			r.Post("/deployments/{deploymentID}/promote", s.handlePromote)
			r.Post("/deployments/{deploymentID}/redeploy", s.handleRedeploy)
		})
	})

	if s.Cfg.DashboardDir != "" {
		r.NotFound(staticSite(s.Cfg.DashboardDir))
	}
	return r
}

// auth enforces MV_API_TOKEN when it is configured. EventSource cannot send
// headers, so ?token= is accepted as well.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Cfg.APIToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.Cfg.APIToken)) != 1 {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "A valid API token is required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"rootDomain":   s.Cfg.RootDomain,
		"publicScheme": s.Cfg.PublicScheme,
		"publicPort":   s.Cfg.PublicPort,
		"authRequired": s.Cfg.APIToken != "",
		"limits": map[string]any{
			"cpus":       s.Cfg.BuildCPUs,
			"memoryMB":   s.Cfg.BuildMemoryMB,
			"pids":       s.Cfg.BuildPIDs,
			"timeoutSec": int(s.Cfg.BuildTimeout.Seconds()),
			"image":      s.Cfg.BuildImage,
		},
	})
}

// staticSite serves a Next.js static export: /project -> project.html,
// /docs/ -> docs/index.html, unknown -> 404.html.
func staticSite(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		candidates := []string{p, p + ".html", path.Join(p, "index.html")}
		for _, c := range candidates {
			f := filepath.Join(dir, filepath.FromSlash(c))
			if st, err := os.Stat(f); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "/_next/static/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				http.ServeFile(w, r, f)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		http.ServeFile(w, r, filepath.Join(dir, "404.html"))
	}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "Resource not found")
		return
	}
	slog.Error("request failed", "path", r.URL.Path, "err", err)
	writeErr(w, http.StatusInternalServerError, "internal", "Something went wrong")
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if strings.HasSuffix(r.URL.Path, "/events") {
			return
		}
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).Round(time.Microsecond))
	})
}
