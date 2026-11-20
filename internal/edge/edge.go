// Package edge is the data-plane router. It maps the Host header to an
// immutable deployment and serves files from object storage.
//
//	dpl-<id>.<root>   -> that exact deployment (preview URL, never changes)
//	<slug>.<root>     -> alias lookup -> current production deployment
//
// Host lookups are cached in an LRU with a short TTL and invalidated over
// Redis pub/sub when an alias is promoted, so a rollback takes effect
// immediately without hitting Postgres on every request.
package edge

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/redis/go-redis/v9"

	"github.com/nikhitha-sk/mini-vercel/internal/config"
	"github.com/nikhitha-sk/mini-vercel/internal/ids"
	"github.com/nikhitha-sk/mini-vercel/internal/logstream"
	"github.com/nikhitha-sk/mini-vercel/internal/storage"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

type Router struct {
	Cfg     config.Config
	Store   *store.Store
	Storage *storage.Storage
	Redis   *redis.Client

	cache    *expirable.LRU[string, string] // hostname -> deploymentID ("" = not found)
	hits     atomic.Int64
	misses   atomic.Int64
	requests atomic.Int64
}

func New(cfg config.Config, st *store.Store, sto *storage.Storage, rdb *redis.Client) *Router {
	return &Router{
		Cfg: cfg, Store: st, Storage: sto, Redis: rdb,
		cache: expirable.NewLRU[string, string](10_000, nil, 30*time.Second),
	}
}

// ListenInvalidations drops cached hosts when the API/worker promotes.
func (rt *Router) ListenInvalidations(ctx context.Context) {
	for {
		sub := rt.Redis.Subscribe(ctx, logstream.EdgeInvalidateChannel)
		ch := sub.Channel()
		for m := range ch {
			rt.cache.Remove(m.Payload)
			slog.Debug("cache invalidated", "host", m.Payload)
		}
		sub.Close()
		if ctx.Err() != nil {
			return
		}
		time.Sleep(time.Second)
	}
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.requests.Add(1)
	w.Header().Set("Server", "mini-vercel")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	host := strings.ToLower(r.Host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == rt.Cfg.RootDomain && r.URL.Path == "/_edge/health" {
		fmt.Fprintf(w, `{"ok":true,"requests":%d,"cacheHits":%d,"cacheMisses":%d}`, rt.requests.Load(), rt.hits.Load(), rt.misses.Load())
		return
	}

	depID, err := rt.resolve(r.Context(), host)
	if err != nil {
		slog.Error("resolve host", "host", host, "err", err)
		rt.errorPage(w, http.StatusBadGateway, "Something went wrong", "The edge router could not reach the routing table.")
		return
	}
	if depID == "" {
		rt.errorPage(w, http.StatusNotFound, "Deployment not found", "There is no live deployment at "+host+".")
		return
	}
	w.Header().Set("X-MiniVercel-Deployment", depID)
	rt.serveFile(w, r, depID)
}

func (rt *Router) resolve(ctx context.Context, host string) (string, error) {
	if id, ok := rt.cache.Get(host); ok {
		rt.hits.Add(1)
		return id, nil
	}
	rt.misses.Add(1)
	suffix := "." + rt.Cfg.RootDomain
	if !strings.HasSuffix(host, suffix) {
		return "", nil
	}
	label := strings.TrimSuffix(host, suffix)
	var id string
	if depID, ok := ids.DeploymentFromLabel(label); ok {
		st, err := rt.Store.DeploymentStatus(ctx, depID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
		if st == store.StatusReady {
			id = depID
		}
	} else {
		var err error
		id, err = rt.Store.ResolveHost(ctx, host)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return "", err
		}
	}
	rt.cache.Add(host, id)
	return id, nil
}

var hashedAsset = regexp.MustCompile(`[.\-_][A-Za-z0-9_-]{8,}\.(?:js|mjs|css|woff2?|png|jpe?g|gif|svg|webp|avif|ico|wasm)$`)

func (rt *Router) serveFile(w http.ResponseWriter, r *http.Request, depID string) {
	p := path.Clean("/" + r.URL.Path)
	rel := strings.TrimPrefix(p, "/")

	// Candidate keys, in order: exact file, directory index, clean URLs
	// (/about -> about.html), and finally SPA fallback to /index.html.
	var candidates []string
	if rel == "" {
		candidates = []string{"index.html"}
	} else {
		candidates = []string{rel, rel + "/index.html", rel + ".html"}
	}
	isAsset := path.Ext(rel) != "" && !strings.HasSuffix(rel, ".html")
	if !isAsset {
		candidates = append(candidates, "index.html") // SPA fallback
	}

	ctx := r.Context()
	for i, c := range candidates {
		obj, err := rt.Storage.Get(ctx, depID+"/"+c)
		if errors.Is(err, storage.ErrNotFound) {
			continue
		}
		if err != nil {
			slog.Error("storage get", "key", depID+"/"+c, "err", err)
			rt.errorPage(w, http.StatusBadGateway, "Storage unavailable", "The deployment files could not be fetched. Try again shortly.")
			return
		}
		defer obj.Body.Close()
		status := http.StatusOK
		if !isAsset && i == len(candidates)-1 && c == "index.html" && rel != "" {
			w.Header().Set("X-MiniVercel-Fallback", "spa")
		}
		h := w.Header()
		h.Set("Content-Type", obj.ContentType)
		if obj.ETag != "" {
			h.Set("ETag", obj.ETag)
		}
		if hashedAsset.MatchString(c) || strings.HasPrefix(c, "_next/static/") || strings.HasPrefix(c, "assets/") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "public, max-age=0, must-revalidate")
		}
		h.Set("X-Content-Type-Options", "nosniff")
		if inm := r.Header.Get("If-None-Match"); inm != "" && obj.ETag != "" && inm == obj.ETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		h.Set("Content-Length", strconv.FormatInt(obj.ContentLength, 10))
		w.WriteHeader(status)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = io.Copy(w, obj.Body)
		return
	}
	// Asset not found: honour a custom 404.html if the site ships one.
	if obj, err := rt.Storage.Get(ctx, depID+"/404.html"); err == nil {
		defer obj.Body.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.Copy(w, obj.Body)
		return
	}
	rt.errorPage(w, http.StatusNotFound, "Page not found", "The file "+p+" does not exist in this deployment.")
}

var errorTmpl = template.Must(template.New("err").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Code}}: {{.Title}}</title>
<style>
:root{color-scheme:light dark;--bg:#fff;--fg:#0a0a0a;--muted:#666;--border:#eaeaea}
@media (prefers-color-scheme:dark){:root{--bg:#000;--fg:#ededed;--muted:#a1a1a1;--border:#2e2e2e}}
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:var(--bg);color:var(--fg);
font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;padding:16px}
.box{display:flex;align-items:center;gap:24px}.code{font-size:24px;font-weight:600;padding-right:24px;border-right:1px solid var(--border)}
h1{font-size:15px;font-weight:500;margin:0 0 4px}p{margin:0;color:var(--muted);font-size:13px}
footer{position:fixed;bottom:20px;font-size:12px;color:var(--muted);display:flex;gap:6px;align-items:center}
</style></head><body><div class="box"><div class="code">{{.Code}}</div><div><h1>{{.Title}}</h1><p>{{.Message}}</p></div></div>
<footer><svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><path d="M12 2L22 20H2z"/></svg> Mini Vercel Edge</footer></body></html>`))

func (rt *Router) errorPage(w http.ResponseWriter, code int, title, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = errorTmpl.Execute(w, map[string]any{"Code": code, "Title": title, "Message": msg})
}
