// Package publish pushes READY deployments to a permanent, always-on host so
// sites get real public URLs that work even when this machine is off.
//
// Mini Vercel still does the platform work (sandboxed builds, logs, immutable
// deployments, rollback); Netlify's free tier serves the built files at
// https://<site>.netlify.app over its global CDN. Each Mini Vercel deployment
// becomes one Netlify deploy, so promote / rollback maps to Netlify's
// "restore deploy" and takes effect instantly.
package publish

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nikhitha-sk/mini-vercel/internal/ids"
	"github.com/nikhitha-sk/mini-vercel/internal/secrets"
	"github.com/nikhitha-sk/mini-vercel/internal/storage"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

const (
	apiBase    = "https://api.netlify.com/api/v1"
	settingKey = "netlify_token"

	StatusPublishing = "publishing"
	StatusLive       = "live"
	StatusError      = "error"
)

var ErrNotConnected = errors.New("netlify is not connected")

type Publisher struct {
	Store   *store.Store
	Storage *storage.Storage
	Secrets *secrets.Box
	HTTP    *http.Client
}

func New(st *store.Store, sto *storage.Storage, box *secrets.Box) *Publisher {
	return &Publisher{Store: st, Storage: sto, Secrets: box, HTTP: &http.Client{Timeout: 5 * time.Minute}}
}

// ---------- token management ----------

type Account struct {
	Name  string `json:"full_name"`
	Email string `json:"email"`
	Slug  string `json:"slug"`
}

func (p *Publisher) token(ctx context.Context) (string, error) {
	ct, nonce, err := p.Store.GetSetting(ctx, settingKey)
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrNotConnected
	}
	if err != nil {
		return "", err
	}
	return p.Secrets.Open(ct, nonce, "setting/"+settingKey)
}

// Connect validates a personal access token and stores it encrypted.
func (p *Publisher) Connect(ctx context.Context, token string) (Account, error) {
	token = strings.TrimSpace(token)
	var acct Account
	if err := p.call(ctx, token, http.MethodGet, "/user", nil, "", &acct); err != nil {
		return acct, fmt.Errorf("Netlify rejected this token: %w", err)
	}
	ct, nonce, err := p.Secrets.Seal(token, "setting/"+settingKey)
	if err != nil {
		return acct, err
	}
	return acct, p.Store.SetSetting(ctx, settingKey, ct, nonce)
}

func (p *Publisher) Disconnect(ctx context.Context) error {
	return p.Store.DeleteSetting(ctx, settingKey)
}

// Account returns the connected account, or ErrNotConnected.
func (p *Publisher) Account(ctx context.Context) (Account, error) {
	tok, err := p.token(ctx)
	if err != nil {
		return Account{}, err
	}
	var acct Account
	err = p.call(ctx, tok, http.MethodGet, "/user", nil, "", &acct)
	return acct, err
}

func (p *Publisher) Connected(ctx context.Context) bool {
	_, err := p.token(ctx)
	return err == nil
}

// ---------- publishing ----------

type site struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	SSLURL string `json:"ssl_url"`
	URL    string `json:"url"`
}

type deploy struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	ErrorMessage string `json:"error_message"`
	SSLURL       string `json:"ssl_url"`
	DeploySSLURL string `json:"deploy_ssl_url"`
}

// Publish uploads a READY deployment's files to the project's Netlify site
// (creating the site on first use) and makes it the live production deploy.
// It returns the permanent site URL.
func (p *Publisher) Publish(ctx context.Context, proj store.Project, d store.Deployment, logf func(string)) (string, error) {
	tok, err := p.token(ctx)
	if err != nil {
		return "", err
	}
	_ = p.Store.SetProjectLiveStatus(ctx, proj.ID, StatusPublishing)

	siteID, siteURL := proj.LiveSiteID, proj.LiveURL
	if siteID != "" {
		// The site may have been deleted on Netlify; recreate if so.
		var s site
		if err := p.call(ctx, tok, http.MethodGet, "/sites/"+siteID, nil, "", &s); err != nil {
			siteID = ""
		} else {
			siteURL = s.SSLURL
		}
	}
	if siteID == "" {
		s, err := p.createSite(ctx, tok, proj.Slug)
		if err != nil {
			_ = p.Store.SetProjectLiveStatus(ctx, proj.ID, StatusError)
			return "", err
		}
		siteID, siteURL = s.ID, s.SSLURL
		logf("Created Netlify site " + siteURL)
	}

	zipData, files, err := p.zipDeployment(ctx, d.ID)
	if err != nil {
		_ = p.Store.SetProjectLiveStatus(ctx, proj.ID, StatusError)
		return "", err
	}
	logf(fmt.Sprintf("Uploading %d files to Netlify (%d KB)", files, len(zipData)/1024))

	var dep deploy
	if err := p.call(ctx, tok, http.MethodPost, "/sites/"+siteID+"/deploys", bytes.NewReader(zipData), "application/zip", &dep); err != nil {
		_ = p.Store.SetProjectLiveStatus(ctx, proj.ID, StatusError)
		return "", err
	}
	// Wait until Netlify has processed the deploy.
	deadline := time.Now().Add(3 * time.Minute)
	for dep.State != "ready" {
		if dep.State == "error" {
			_ = p.Store.SetProjectLiveStatus(ctx, proj.ID, StatusError)
			return "", fmt.Errorf("netlify deploy failed: %s", dep.ErrorMessage)
		}
		if time.Now().After(deadline) {
			_ = p.Store.SetProjectLiveStatus(ctx, proj.ID, StatusError)
			return "", errors.New("netlify deploy did not become ready in time")
		}
		time.Sleep(2 * time.Second)
		if err := p.call(ctx, tok, http.MethodGet, "/deploys/"+dep.ID, nil, "", &dep); err != nil {
			return "", err
		}
	}
	_ = p.Store.SetDeploymentLive(ctx, d.ID, dep.ID, dep.DeploySSLURL)
	_ = p.Store.SetProjectLive(ctx, proj.ID, siteID, siteURL, StatusLive)
	return siteURL, nil
}

// Restore makes an earlier deployment live again on Netlify (instant rollback).
func (p *Publisher) Restore(ctx context.Context, proj store.Project, d store.Deployment) error {
	if proj.LiveSiteID == "" || d.LiveDeployID == "" {
		return errors.New("this deployment was never published to Netlify")
	}
	tok, err := p.token(ctx)
	if err != nil {
		return err
	}
	var dep deploy
	return p.call(ctx, tok, http.MethodPost, "/sites/"+proj.LiveSiteID+"/deploys/"+d.LiveDeployID+"/restore", nil, "", &dep)
}

// DeleteSite removes the project's Netlify site (when the project is deleted).
func (p *Publisher) DeleteSite(ctx context.Context, siteID string) error {
	if siteID == "" {
		return nil
	}
	tok, err := p.token(ctx)
	if err != nil {
		return err
	}
	return p.call(ctx, tok, http.MethodDelete, "/sites/"+siteID, nil, "", nil)
}

func (p *Publisher) createSite(ctx context.Context, tok, slug string) (site, error) {
	name := slug
	var lastErr error
	for i := 0; i < 5; i++ {
		var s site
		body, _ := json.Marshal(map[string]string{"name": name})
		err := p.call(ctx, tok, http.MethodPost, "/sites", bytes.NewReader(body), "application/json", &s)
		if err == nil {
			return s, nil
		}
		lastErr = err
		// Site names are global on Netlify; add a short suffix if taken.
		suffix := ids.Secret()[:5]
		base := slug
		if len(base) > 50 {
			base = base[:50]
		}
		name = base + "-" + suffix
	}
	return site{}, fmt.Errorf("create netlify site: %w", lastErr)
}

// zipDeployment builds a zip of <deployId>/... from object storage. A SPA
// fallback (_redirects) is added when the site has no 404.html, mirroring
// the local edge router's behaviour.
func (p *Publisher) zipDeployment(ctx context.Context, deploymentID string) ([]byte, int, error) {
	prefix := deploymentID + "/"
	keys, err := p.Storage.List(ctx, prefix)
	if err != nil {
		return nil, 0, err
	}
	if len(keys) == 0 {
		return nil, 0, errors.New("deployment has no files in storage")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	has := map[string]bool{}
	for _, k := range keys {
		rel := strings.TrimPrefix(k, prefix)
		has[rel] = true
		obj, err := p.Storage.Get(ctx, k)
		if err != nil {
			return nil, 0, err
		}
		w, err := zw.Create(rel)
		if err == nil {
			_, err = io.Copy(w, obj.Body)
		}
		obj.Body.Close()
		if err != nil {
			return nil, 0, err
		}
	}
	if has["index.html"] && !has["404.html"] && !has["_redirects"] {
		w, _ := zw.Create("_redirects")
		io.WriteString(w, "/*    /index.html   200\n")
	}
	if err := zw.Close(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), len(keys), nil
}

func (p *Publisher) call(ctx context.Context, tok, method, path string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, apiBase+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("User-Agent", "mini-vercel")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &e)
		msg := e.Message
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("netlify %s %s: %d %s", method, path, resp.StatusCode, msg)
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}
