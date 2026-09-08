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

	mu              sync.RWMutex
	remoteAddr      string
	clientVersion   string
	clientHostname  string
	clientOS        string
	clientArch      string
	lastActivity    time.Time
	requests        uint64
	bytesToClient   uint64
	bytesFromClient uint64
	activeStreams   int
	connectionCount uint64
}

type sessionStats struct {
	Online          bool
	Joined          time.Time
	LastActivity    time.Time
	RemoteAddr      string
	ClientVersion   string
	ClientHostname  string
	ClientOS        string
	ClientArch      string
	Requests        uint64
	BytesToClient   uint64
	BytesFromClient uint64
	ActiveStreams   int
	ConnectionCount uint64
}

type sessionRegistry struct {
	mu          sync.RWMutex
	sessions    map[string]*session // tunnelID -> live session
	last        map[string]sessionStats
	connections map[string]uint64
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{
		sessions:    make(map[string]*session),
		last:        make(map[string]sessionStats),
		connections: make(map[string]uint64),
	}
}

func (r *sessionRegistry) register(tunnelID string, s *session) (kicked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.connections[tunnelID]++
	s.mu.Lock()
	s.connectionCount = r.connections[tunnelID]
	if s.lastActivity.IsZero() {
		s.lastActivity = s.joined
	}
	s.mu.Unlock()

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
		s.touch()
		r.last[tunnelID] = s.snapshot(false)
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

// stats returns live connection telemetry, or the last known snapshot after a
// disconnect. Telemetry is intentionally in-memory and resets with the plugin.
func (r *sessionRegistry) stats(tunnelID string) (sessionStats, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.sessions[tunnelID]; ok && !s.yamux.IsClosed() {
		return s.snapshot(true), true
	}
	st, ok := r.last[tunnelID]
	return st, ok
}

func (s *session) touch() {
	s.mu.Lock()
	s.lastActivity = time.Now().UTC()
	s.mu.Unlock()
}

func (s *session) streamOpened() {
	s.mu.Lock()
	s.requests++
	s.activeStreams++
	s.lastActivity = time.Now().UTC()
	s.mu.Unlock()
}

func (s *session) streamClosed() {
	s.mu.Lock()
	if s.activeStreams > 0 {
		s.activeStreams--
	}
	s.lastActivity = time.Now().UTC()
	s.mu.Unlock()
}

func (s *session) recordToClient(n int) {
	if n <= 0 {
		return
	}
	s.mu.Lock()
	s.bytesToClient += uint64(n)
	s.lastActivity = time.Now().UTC()
	s.mu.Unlock()
}

func (s *session) recordFromClient(n int) {
	if n <= 0 {
		return
	}
	s.mu.Lock()
	s.bytesFromClient += uint64(n)
	s.lastActivity = time.Now().UTC()
	s.mu.Unlock()
}

func (s *session) snapshot(online bool) sessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sessionStats{
		Online:          online,
		Joined:          s.joined,
		LastActivity:    s.lastActivity,
		RemoteAddr:      s.remoteAddr,
		ClientVersion:   s.clientVersion,
		ClientHostname:  s.clientHostname,
		ClientOS:        s.clientOS,
		ClientArch:      s.clientArch,
		Requests:        s.requests,
		BytesToClient:   s.bytesToClient,
		BytesFromClient: s.bytesFromClient,
		ActiveStreams:   s.activeStreams,
		ConnectionCount: s.connectionCount,
	}
}

type trackedStream struct {
	io.ReadWriteCloser
	session *session
	once    sync.Once
}

func (t *trackedStream) Read(p []byte) (int, error) {
	n, err := t.ReadWriteCloser.Read(p)
	t.session.recordFromClient(n)
	return n, err
}

func (t *trackedStream) Write(p []byte) (int, error) {
	n, err := t.ReadWriteCloser.Write(p)
	t.session.recordToClient(n)
	return n, err
}

func (t *trackedStream) Close() error {
	err := t.ReadWriteCloser.Close()
	t.once.Do(t.session.streamClosed)
	return err
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
	s.streamOpened()
	tracked := &trackedStream{ReadWriteCloser: stream, session: s}
	if err := wire.WriteJSON(tracked, head); err != nil {
		tracked.Close()
		return nil, err
	}
	if !head.IsWebSocket {
		if err := wire.WriteBody(tracked, body); err != nil {
			tracked.Close()
			return nil, err
		}
	}
	return tracked, nil
}
