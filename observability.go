package main

import (
	"net/http"
	"time"
)

type clientStatsView struct {
	Online          bool   `json:"online"`
	Version         string `json:"version,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	OS              string `json:"os,omitempty"`
	Arch            string `json:"arch,omitempty"`
	RemoteAddr      string `json:"remote_addr,omitempty"`
	ConnectedAt     string `json:"connected_at,omitempty"`
	LastActivity    string `json:"last_activity,omitempty"`
	Requests        uint64 `json:"requests"`
	ActiveStreams   int    `json:"active_streams"`
	BytesToClient   uint64 `json:"bytes_to_client"`
	BytesFromClient uint64 `json:"bytes_from_client"`
	ConnectionCount uint64 `json:"connection_count"`
	ReconnectCount  uint64 `json:"reconnect_count"`
}

// handleClientStats exposes only operational metadata for configured tunnel
// clients. Tokens and other secrets are never included. Statistics are kept in
// memory and reset when the plugin restarts.
func (a *apiServer) handleClientStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	out := make(map[string]clientStatsView)
	for _, tunnel := range a.store.snapshot() {
		st, ok := a.registry.stats(tunnel.ID)
		if !ok {
			out[tunnel.ID] = clientStatsView{Online: false}
			continue
		}
		view := clientStatsView{
			Online:          st.Online,
			Version:         st.ClientVersion,
			Hostname:        st.ClientHostname,
			OS:              st.ClientOS,
			Arch:            st.ClientArch,
			RemoteAddr:      st.RemoteAddr,
			Requests:        st.Requests,
			ActiveStreams:   st.ActiveStreams,
			BytesToClient:   st.BytesToClient,
			BytesFromClient: st.BytesFromClient,
			ConnectionCount: st.ConnectionCount,
		}
		if st.ConnectionCount > 0 {
			view.ReconnectCount = st.ConnectionCount - 1
		}
		if !st.Joined.IsZero() {
			view.ConnectedAt = st.Joined.UTC().Format(time.RFC3339)
		}
		if !st.LastActivity.IsZero() {
			view.LastActivity = st.LastActivity.UTC().Format(time.RFC3339)
		}
		out[tunnel.ID] = view
	}
	writeJSON(w, out)
}
