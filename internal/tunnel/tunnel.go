// Package tunnel gives projects a free public HTTPS URL without a domain,
// a public IP or port forwarding, using Cloudflare quick tunnels
// (https://<random>.trycloudflare.com).
//
// For every project with public sharing enabled the manager runs one
// cloudflared container that forwards to the edge router with the Host header
// fixed to <slug>.<root-domain>. The edge therefore serves the project's
// production alias, so promote and rollback apply to the public URL instantly,
// and a tunnel can only ever reach that one project's static files.
//
// The manager is a reconciler: every few seconds (and whenever the API asks
// over Redis) it compares enabled projects with running tunnel containers,
// starts/removes containers, and records the URL cloudflared prints.
package tunnel

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"regexp"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/redis/go-redis/v9"

	"github.com/nikhitha-sk/mini-vercel/internal/config"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

const (
	ReconcileChannel = "tunnels:reconcile"
	roleLabel        = "io.minivercel.role"
	projectLabel     = "io.minivercel.project"
	hostLabel        = "io.minivercel.host"

	StatusStarting = "starting"
	StatusLive     = "live"
	StatusError    = "error"
)

var urlRe = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

type Manager struct {
	Cfg    config.Config
	Store  *store.Store
	Redis  *redis.Client
	docker *client.Client
}

func New(cfg config.Config, st *store.Store, rdb *redis.Client) (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Manager{Cfg: cfg, Store: st, Redis: rdb, docker: cli}, nil
}

// Run reconciles until ctx is canceled.
func (m *Manager) Run(ctx context.Context) {
	sub := m.Redis.Subscribe(ctx, ReconcileChannel)
	defer sub.Close()
	kick := sub.Channel()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		if err := m.reconcile(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("tunnel reconcile", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-kick:
		}
	}
}

func (m *Manager) reconcile(ctx context.Context) error {
	projects, err := m.Store.ListPublicProjects(ctx)
	if err != nil {
		return err
	}
	list, err := m.docker.ContainerList(ctx, container.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("label", roleLabel+"=tunnel"))})
	if err != nil {
		return err
	}
	running := map[string]container.Summary{}
	for _, c := range list {
		running[c.Labels[projectLabel]] = c
	}

	wanted := map[string]bool{}
	for _, p := range projects {
		wanted[p.ID] = true
		host := m.Cfg.Hostname(p.Slug)
		c, ok := running[p.ID]
		if ok && (c.State != "running" || c.Labels[hostLabel] != host) {
			// Crashed (e.g. Cloudflare refused the quick tunnel) or stale config.
			m.remove(c.ID)
			ok = false
		}
		if !ok {
			if err := m.start(ctx, p, host); err != nil {
				slog.Warn("start tunnel", "project", p.Slug, "err", err)
				_ = m.Store.SetPublicURL(ctx, p.ID, "", StatusError)
				continue
			}
			if p.PublicStatus != StatusStarting || p.PublicURL != "" {
				_ = m.Store.SetPublicURL(ctx, p.ID, "", StatusStarting)
			}
			continue
		}
		url := m.currentURL(ctx, c.ID)
		if url != "" && (url != p.PublicURL || p.PublicStatus != StatusLive) {
			slog.Info("public URL ready", "project", p.Slug, "url", url)
			_ = m.Store.SetPublicURL(ctx, p.ID, url, StatusLive)
		}
	}
	for pid, c := range running {
		if !wanted[pid] {
			m.remove(c.ID)
		}
	}
	return nil
}

func (m *Manager) start(ctx context.Context, p store.Project, host string) error {
	img := m.Cfg.TunnelImage
	if _, err := m.docker.ImageInspect(ctx, img); err != nil {
		rc, err := m.docker.ImagePull(ctx, img, image.PullOptions{})
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, rc)
		rc.Close()
	}
	name := "mv-tunnel-" + p.Slug
	_ = m.docker.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	resp, err := m.docker.ContainerCreate(ctx,
		&container.Config{
			Image: img,
			Cmd:   []string{"tunnel", "--no-autoupdate", "--url", m.Cfg.TunnelOrigin, "--http-host-header", host},
			Labels: map[string]string{
				roleLabel: "tunnel", projectLabel: p.ID, hostLabel: host,
			},
		},
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
			ExtraHosts:    []string{"host.docker.internal:host-gateway"},
			CapDrop:       []string{"ALL"},
			SecurityOpt:   []string{"no-new-privileges:true"},
		}, nil, nil, name)
	if err != nil {
		return err
	}
	slog.Info("starting public tunnel", "project", p.Slug, "host", host)
	return m.docker.ContainerStart(ctx, resp.ID, container.StartOptions{})
}

// currentURL returns the most recent trycloudflare URL the container printed
// (after a restart cloudflared gets a new one, so the last match wins).
func (m *Manager) currentURL(ctx context.Context, id string) string {
	rc, err := m.docker.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "400"})
	if err != nil {
		return ""
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, rc); err != nil && buf.Len() == 0 {
		return ""
	}
	all := urlRe.FindAllString(buf.String(), -1)
	if len(all) == 0 {
		return ""
	}
	return all[len(all)-1]
}

func (m *Manager) remove(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := m.docker.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		slog.Warn("remove tunnel", "err", err)
	}
}
