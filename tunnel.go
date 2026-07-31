package main

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/yamux"

	"zoraxy-tunnel/wire"
)

var errTunnelOffline = errors.New("tunnel offline")

// session is one live client connection, keyed by tunnel ID.
type session struct {
	yamux  *yamux.Session
	joined time.Time
}

type sessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*session // tunnelID -> live session
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{sessions: make(map[string]*session)}
}

func (r *sessionRegistry) register(tunnelID string, s *session) (kicked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.sessions[tunnelID]; ok && old.yamux != s.yamux {
		kicked = true
		_ = old.yamux.Close() // one client per tunnel: drop the previous one
	}
	r.sessions[tunnelID] = s
	return
}

func (r *sessionRegistry) unregister(tunnelID string, s *session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.sessions[tunnelID]; ok && cur == s {
		delete(r.sessions, tunnelID)
	}
}

func (r *sessionRegistry) get(tunnelID string) (*session, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[tunnelID]
	return s, ok
}

// online reports whether a client is currently connected for this tunnel.
func (r *sessionRegistry) online(tunnelID string) bool {
	s, ok := r.get(tunnelID)
	return ok && !s.yamux.IsClosed()
}

// forward opens a data stream to the tunnel's client, ships the request head
// (and body for non-websocket), and hands the open stream back so the caller
// can read the response.
func (r *sessionRegistry) forward(tunnelID string, head wire.RequestHead, body io.Reader) (io.ReadWriteCloser, error) {
	s, ok := r.get(tunnelID)
	if !ok || s.yamux.IsClosed() {
		return nil, errTunnelOffline
	}
	stream, err := s.yamux.Open()
	if err != nil {
		return nil, err
	}
	if err := wire.WriteJSON(stream, head); err != nil {
		stream.Close()
		return nil, err
	}
	if !head.IsWebSocket {
		if err := wire.WriteBody(stream, body); err != nil {
			stream.Close()
			return nil, err
		}
	}
	return stream, nil
}
