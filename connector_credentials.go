package main

import (
	"net/http"
	"strings"
	"time"
)

type connectorCredentialView struct {
	ID          string    `json:"id"`
	ConnectorID string    `json:"connector_id"`
	TokenHint   string    `json:"token_hint"`
	Created     time.Time `json:"created"`
	Enabled     bool      `json:"enabled"`
}

func connectorCredentialViews(in []ConnectorCredential) []connectorCredentialView {
	out := make([]connectorCredentialView, 0, len(in))
	for _, c := range in {
		out = append(out, connectorCredentialView{
			ID: c.ID, ConnectorID: c.ConnectorID, TokenHint: c.TokenHint,
			Created: c.Created, Enabled: c.Enabled,
		})
	}
	return out
}

func (a *apiServer) handleAddConnector(w http.ResponseWriter, r *http.Request) {
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
	connectorID := strings.TrimSpace(body.ConnectorID)
	if connectorID == "" {
		http.Error(w, "connector id required", http.StatusBadRequest)
		return
	}
	if connectorID != sanitizeConnectorID(connectorID) || len(connectorID) > 128 {
		http.Error(w, "connector id may only contain letters, numbers, dot, underscore and dash", http.StatusBadRequest)
		return
	}
	t, ok := a.store.tunnel(body.TunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}
	for _, cred := range t.ConnectorCredentials {
		if strings.EqualFold(cred.ConnectorID, connectorID) && cred.Enabled {
			http.Error(w, "this connector id already has an active credential", http.StatusConflict)
			return
		}
	}
	plain, hash, hint := newToken()
	cred := ConnectorCredential{
		ID: newID("cc_"), ConnectorID: connectorID, TokenHash: hash,
		TokenHint: hint, Created: time.Now().UTC(), Enabled: true,
	}
	t.ConnectorCredentials = append(t.ConnectorCredentials, cred)
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	appEvents.add("info", "connector.credential_created", t.ID, connectorID, "", "Additional connector credential created")
	writeJSON(w, map[string]any{
		"tunnel_id": t.ID,
		"tunnel_name": t.Name,
		"connector_id": connectorID,
		"credential": connectorCredentialView{ID: cred.ID, ConnectorID: cred.ConnectorID, TokenHint: cred.TokenHint, Created: cred.Created, Enabled: true},
		"token": plain,
	})
}
