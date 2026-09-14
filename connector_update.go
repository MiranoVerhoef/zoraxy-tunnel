package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"

	"zoraxy-tunnel/wire"
)

const connectorUpdaterTarget = "http://127.0.0.1:3009"

func (r *sessionRegistry) requestOnConnector(tunnelID, connectorID string, head wire.RequestHead, body []byte) (wire.ResponseHead, []byte, error) {
	r.mu.RLock()
	byConnector := r.sessions[tunnelID]
	var s *session
	if byConnector != nil {
		s = byConnector[connectorID]
	}
	r.mu.RUnlock()
	if s == nil || s.yamux == nil || s.yamux.IsClosed() {
		return wire.ResponseHead{}, nil, errTunnelOffline
	}
	stream, err := s.yamux.Open()
	if err != nil {
		return wire.ResponseHead{}, nil, err
	}
	s.streamOpened()
	tracked := &trackedStream{ReadWriteCloser: stream, session: s}
	defer tracked.Close()
	if err := wire.WriteJSON(tracked, head); err != nil {
		return wire.ResponseHead{}, nil, err
	}
	if err := wire.WriteBody(tracked, bytes.NewReader(body)); err != nil {
		return wire.ResponseHead{}, nil, err
	}
	var resp wire.ResponseHead
	if err := wire.ReadJSON(tracked, &resp); err != nil {
		return wire.ResponseHead{}, nil, err
	}
	var buf bytes.Buffer
	if err := wire.ReadBody(&buf, tracked); err != nil {
		return wire.ResponseHead{}, nil, err
	}
	return resp, buf.Bytes(), nil
}

func (a *apiServer) handleConnectorUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		TunnelID    string `json:"tunnel_id"`
		ConnectorID string `json:"connector_id"`
	}
	if err := decode(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.TunnelID == "" || body.ConnectorID == "" {
		http.Error(w, "tunnel id and connector id are required", http.StatusBadRequest)
		return
	}
	if connectorUpdateMode(body.TunnelID, body.ConnectorID) != "auto" {
		http.Error(w, "this connector is not configured for automatic Docker updates", http.StatusConflict)
		return
	}
	if _, ok := a.store.tunnel(body.TunnelID); !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}
	user, password, ok := connectorUpdaterCredentials(body.TunnelID, body.ConnectorID)
	if !ok {
		http.Error(w, "updater authentication is not available; redeploy this connector using the v1.12 automatic Compose configuration", http.StatusConflict)
		return
	}
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))

	paths := []string{"/api/containers/watch", "/api/v1/containers/watch"}
	var lastErr error
	for _, path := range paths {
		head := wire.RequestHead{
			Target: connectorUpdaterTarget,
			Method: http.MethodPost,
			URL:    path,
			Host:   "127.0.0.1:3009",
			Headers: map[string]string{
				"Accept":        "application/json",
				"Authorization": auth,
			},
		}
		resp, raw, err := a.registry.requestOnConnector(body.TunnelID, body.ConnectorID, head, nil)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.Status == http.StatusNotFound {
			lastErr = errors.New("updater API endpoint not found")
			continue
		}
		if resp.Status == http.StatusUnauthorized || resp.Status == http.StatusForbidden {
			http.Error(w, "updater authentication failed; redeploy this connector with the current automatic Compose configuration", http.StatusBadGateway)
			return
		}
		if resp.Status < 200 || resp.Status >= 300 {
			msg := string(raw)
			if msg == "" {
				msg = http.StatusText(resp.Status)
			}
			http.Error(w, fmt.Sprintf("updater returned %d: %s", resp.Status, msg), http.StatusBadGateway)
			return
		}
		appEvents.add("info", "connector.update_requested", body.TunnelID, body.ConnectorID, "", "Manual Docker update check requested")
		writeJSON(w, map[string]any{"ok": true, "message": "Update check requested"})
		return
	}
	if lastErr == nil {
		lastErr = errors.New("updater unavailable")
	}
	http.Error(w, lastErr.Error(), http.StatusBadGateway)
}
