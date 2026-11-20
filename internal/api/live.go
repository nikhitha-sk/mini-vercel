package api

import (
	"errors"
	"net/http"

	"github.com/nikhitha-sk/mini-vercel/internal/publish"
	"github.com/nikhitha-sk/mini-vercel/internal/queue"
	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

// GET /api/integrations/netlify
func (s *Server) handleNetlifyStatus(w http.ResponseWriter, r *http.Request) {
	acct, err := s.Publish.Account(r.Context())
	if errors.Is(err, publish.ErrNotConnected) {
		writeJSON(w, 200, map[string]any{"connected": false})
		return
	}
	if err != nil {
		writeJSON(w, 200, map[string]any{"connected": true, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"connected": true, "name": acct.Name, "email": acct.Email})
}

// PUT /api/integrations/netlify {token}
func (s *Server) handleNetlifyConnect(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := decode(r, &in); err != nil || in.Token == "" {
		writeErr(w, 400, "bad_request", "Paste your Netlify personal access token")
		return
	}
	acct, err := s.Publish.Connect(r.Context(), in.Token)
	if err != nil {
		writeErr(w, 422, "invalid_token", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"connected": true, "name": acct.Name, "email": acct.Email})
}

// DELETE /api/integrations/netlify
func (s *Server) handleNetlifyDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := s.Publish.Disconnect(r.Context()); err != nil {
		s.internal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/projects/{id}/publish: publish the current production deployment.
func (s *Server) handlePublishProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.loadProject(w, r)
	if !ok {
		return
	}
	if !s.Publish.Connected(r.Context()) {
		writeErr(w, 409, "not_connected", "Connect Netlify on the Platform page first")
		return
	}
	prodID := s.productionID(r.Context(), p)
	if prodID == "" {
		writeErr(w, 409, "no_production", "Deploy the project first")
		return
	}
	if err := s.enqueuePublish(r, prodID); err != nil {
		s.internal(w, r, err)
		return
	}
	_ = s.Store.SetProjectLiveStatus(r.Context(), p.ID, publish.StatusPublishing)
	writeJSON(w, 202, map[string]any{"deploymentId": prodID, "status": publish.StatusPublishing})
}

func (s *Server) enqueuePublish(r *http.Request, deploymentID string) error {
	task, err := queue.NewPublishTask(deploymentID)
	if err != nil {
		return err
	}
	_, err = s.Queue.EnqueueContext(r.Context(), task)
	return err
}

// syncLiveOnPromote mirrors a promote/rollback to the live host: restore the
// matching Netlify deploy instantly, or publish it if it was never uploaded.
func (s *Server) syncLiveOnPromote(r *http.Request, p store.Project, d store.Deployment) string {
	if !s.Publish.Connected(r.Context()) || p.LiveSiteID == "" {
		return ""
	}
	if d.LiveDeployID != "" {
		if err := s.Publish.Restore(r.Context(), p, d); err == nil {
			return "restored"
		}
	}
	if err := s.enqueuePublish(r, d.ID); err == nil {
		_ = s.Store.SetProjectLiveStatus(r.Context(), p.ID, publish.StatusPublishing)
		return "publishing"
	}
	return ""
}
