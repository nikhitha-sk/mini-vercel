package api

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/nikhitha-sk/mini-vercel/internal/builder"
	"github.com/nikhitha-sk/mini-vercel/internal/ids"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

type projectView struct {
	store.Project
	ProductionURL        string            `json:"productionUrl"`
	ProductionDeployment *deploymentView   `json:"productionDeployment"`
	LatestDeployment     *deploymentView   `json:"latestDeployment"`
	WebhookURL           string            `json:"webhookUrl,omitempty"`
	WebhookSecret        string            `json:"webhookSecret,omitempty"`
	Aliases              []store.Alias     `json:"aliases,omitempty"`
}

func (s *Server) projectView(ctx context.Context, p store.Project, detail bool) (projectView, error) {
	v := projectView{Project: p, ProductionURL: s.Cfg.PublicURL(p.Slug)}
	deps, err := s.Store.ListDeployments(ctx, p.ID, 1)
	if err != nil {
		return v, err
	}
	if len(deps) > 0 {
		dv := s.deploymentView(deps[0], "")
		v.LatestDeployment = &dv
	}
	if prodID, err := s.Store.ResolveHost(ctx, s.Cfg.Hostname(p.Slug)); err == nil {
		if d, err := s.Store.GetDeployment(ctx, prodID); err == nil {
			dv := s.deploymentView(d, prodID)
			v.ProductionDeployment = &dv
		}
	}
	if detail {
		v.WebhookURL = "/api/webhooks/github/" + p.ID
		v.WebhookSecret = p.WebhookSecret
		v.Aliases, _ = s.Store.ProjectAliases(ctx, p.ID)
	}
	return v, nil
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Store.ListProjects(r.Context(), s.UserID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]projectView, 0, len(ps))
	for _, p := range ps {
		v, err := s.projectView(r.Context(), p, false)
		if err != nil {
			s.internal(w, r, err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	v, err := s.projectView(r.Context(), p, true)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, 200, v)
}

type projectInput struct {
	Name       *string `json:"name"`
	RepoURL    *string `json:"repoUrl"`
	Branch     *string `json:"branch"`
	Framework  *string `json:"framework"`
	InstallCmd *string `json:"installCommand"`
	BuildCmd   *string `json:"buildCommand"`
	OutputDir  *string `json:"outputDirectory"`
	RootDir    *string `json:"rootDirectory"`
	Env        []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"env"`
	Deploy bool `json:"deploy"`
}

var (
	slugRe   = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?$`)
	envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	reserved = map[string]bool{"api": true, "www": true, "app": true, "dashboard": true, "admin": true, "edge": true, "static": true}
)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 40 {
		out = strings.Trim(out[:40], "-")
	}
	return out
}

func str(p *string, def string) string {
	if p == nil {
		return def
	}
	return strings.TrimSpace(*p)
}

func cleanDir(d string) (string, error) {
	d = strings.TrimSpace(strings.ReplaceAll(d, "\\", "/"))
	if d == "" || d == "." || d == "./" {
		return ".", nil
	}
	c := path.Clean("/" + d)
	if strings.Contains(d, "..") {
		return "", fmt.Errorf("directory must stay inside the repository")
	}
	return strings.TrimPrefix(c, "/"), nil
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var in projectInput
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad_request", "Invalid JSON body")
		return
	}
	repo := str(in.RepoURL, "")
	if repo != "" {
		var err error
		if repo, err = builder.ValidateRepoURL(repo); err != nil {
			writeErr(w, 422, "invalid_repo", err.Error())
			return
		}
	}
	name := str(in.Name, "")
	if name == "" && repo != "" {
		name = path.Base(repo)
		name = strings.TrimSuffix(name, ".git")
	}
	if name == "" {
		writeErr(w, 422, "invalid_name", "Project name is required")
		return
	}
	framework := str(in.Framework, "vite")
	preset, ok := builder.PresetByID(framework)
	if !ok {
		writeErr(w, 422, "invalid_framework", "Unknown framework preset")
		return
	}
	out, err := cleanDir(str(in.OutputDir, preset.OutputDir))
	if err != nil {
		writeErr(w, 422, "invalid_output", "Output directory "+err.Error())
		return
	}
	root, err := cleanDir(str(in.RootDir, ""))
	if err != nil {
		writeErr(w, 422, "invalid_root", "Root directory "+err.Error())
		return
	}
	if root == "." {
		root = ""
	}
	for _, e := range in.Env {
		if !envKeyRe.MatchString(e.Key) {
			writeErr(w, 422, "invalid_env", fmt.Sprintf("Invalid environment variable name %q", e.Key))
			return
		}
	}

	slug, err := s.uniqueSlug(r.Context(), slugify(name))
	if err != nil {
		s.internal(w, r, err)
		return
	}
	p, err := s.Store.CreateProject(r.Context(), store.Project{
		ID: ids.Project(), OwnerID: s.UserID, Name: name, Slug: slug, RepoURL: repo,
		Branch: str(in.Branch, "main"), Framework: framework,
		InstallCmd: str(in.InstallCmd, preset.Install), BuildCmd: str(in.BuildCmd, preset.Build),
		OutputDir: out, RootDir: root, WebhookSecret: ids.Secret(),
	})
	if err != nil {
		s.internal(w, r, err)
		return
	}
	if p.Branch == "" {
		p.Branch = "main"
	}
	for _, e := range in.Env {
		if err := s.setEnv(r.Context(), p.ID, e.Key, e.Value); err != nil {
			s.internal(w, r, err)
			return
		}
	}

	resp := map[string]any{}
	if in.Deploy && p.RepoURL != "" {
		d, err := s.enqueueDeployment(r.Context(), p, "git", "manual", "", "")
		if err != nil {
			s.internal(w, r, err)
			return
		}
		dv := s.deploymentView(d, "")
		resp["deployment"] = dv
	}
	v, err := s.projectView(r.Context(), p, true)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	resp["project"] = v
	writeJSON(w, 201, resp)
}

func (s *Server) uniqueSlug(ctx context.Context, base string) (string, error) {
	if base == "" || strings.HasPrefix(base, "dpl-") || reserved[base] {
		base = "site-" + base
		base = strings.Trim(base, "-")
	}
	if !slugRe.MatchString(base) {
		base = "site"
	}
	slug := base
	for i := 0; i < 20; i++ {
		exists, err := s.Store.SlugExists(ctx, slug)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		suffix := ids.Secret()[:4]
		b := base
		if len(b) > 35 {
			b = b[:35]
		}
		slug = b + "-" + suffix
	}
	return "", fmt.Errorf("could not allocate slug")
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	var in projectInput
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad_request", "Invalid JSON body")
		return
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		p.Name = strings.TrimSpace(*in.Name)
	}
	if in.RepoURL != nil {
		repo := strings.TrimSpace(*in.RepoURL)
		if repo != "" {
			var err error
			if repo, err = builder.ValidateRepoURL(repo); err != nil {
				writeErr(w, 422, "invalid_repo", err.Error())
				return
			}
		}
		p.RepoURL = repo
	}
	if in.Branch != nil && strings.TrimSpace(*in.Branch) != "" {
		p.Branch = strings.TrimSpace(*in.Branch)
	}
	if in.Framework != nil {
		if _, ok := builder.PresetByID(*in.Framework); !ok {
			writeErr(w, 422, "invalid_framework", "Unknown framework preset")
			return
		}
		p.Framework = *in.Framework
	}
	if in.InstallCmd != nil {
		p.InstallCmd = strings.TrimSpace(*in.InstallCmd)
	}
	if in.BuildCmd != nil {
		p.BuildCmd = strings.TrimSpace(*in.BuildCmd)
	}
	if in.OutputDir != nil {
		d, err := cleanDir(*in.OutputDir)
		if err != nil {
			writeErr(w, 422, "invalid_output", "Output directory "+err.Error())
			return
		}
		p.OutputDir = d
	}
	if in.RootDir != nil {
		d, err := cleanDir(*in.RootDir)
		if err != nil {
			writeErr(w, 422, "invalid_root", "Root directory "+err.Error())
			return
		}
		if d == "." {
			d = ""
		}
		p.RootDir = d
	}
	p, err := s.Store.UpdateProject(r.Context(), p)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	v, err := s.projectView(r.Context(), p, true)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	aliases, _ := s.Store.ProjectAliases(r.Context(), p.ID)
	if err := s.Store.DeleteProject(r.Context(), p.ID); err != nil {
		s.internal(w, r, err)
		return
	}
	for _, a := range aliases {
		s.invalidateHost(a.Hostname)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loadProject(w http.ResponseWriter, r *http.Request) (store.Project, bool) {
	p, err := s.Store.GetProject(r.Context(), chi.URLParam(r, "projectID"))
	if err != nil {
		s.internal(w, r, err)
		return p, false
	}
	if p.OwnerID != s.UserID {
		writeErr(w, 404, "not_found", "Project not found")
		return p, false
	}
	return p, true
}

// ---------- env vars ----------

type envView struct {
	Key       string    `json:"key"`
	Preview   string    `json:"preview"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *Server) handleListEnv(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	vars, err := s.Store.ListEnvVars(r.Context(), p.ID)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	out := make([]envView, 0, len(vars))
	for _, v := range vars {
		// Values are write-only through the API; only a masked hint is returned.
		out = append(out, envView{Key: v.Key, Preview: "••••••••", UpdatedAt: v.UpdatedAt})
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleSetEnv(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	var in struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "bad_request", "Invalid JSON body")
		return
	}
	if !envKeyRe.MatchString(in.Key) {
		writeErr(w, 422, "invalid_env", "Variable names may contain letters, digits and underscores, and must not start with a digit")
		return
	}
	if len(in.Value) > 32<<10 {
		writeErr(w, 422, "invalid_env", "Value is larger than 32 KB")
		return
	}
	if err := s.setEnv(r.Context(), p.ID, in.Key, in.Value); err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) setEnv(ctx context.Context, projectID, key, value string) error {
	ct, nonce, err := s.Secrets.Seal(value, projectID+"/"+key)
	if err != nil {
		return err
	}
	return s.Store.UpsertEnvVar(ctx, store.EnvVar{ID: ids.EnvVar(), ProjectID: projectID, Key: key, Ciphertext: ct, Nonce: nonce})
}

func (s *Server) handleDeleteEnv(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteEnvVar(r.Context(), p.ID, chi.URLParam(r, "key")); err != nil {
		s.internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, builder.Presets)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.Stats(r.Context(), s.UserID, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		s.internal(w, r, err)
		return
	}
	writeJSON(w, 200, st)
}
