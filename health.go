package main

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"zoraxy-tunnel/wire"
)

type serviceHealth struct {
	TunnelID   string    `json:"tunnel_id"`
	ServiceID  string    `json:"service_id"`
	Status     string    `json:"status"`
	HTTPStatus int       `json:"http_status,omitempty"`
	LatencyMS  int64     `json:"latency_ms,omitempty"`
	Connector  string    `json:"connector_id,omitempty"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type healthManager struct {
	store    *Store
	registry *sessionRegistry
	mu       sync.RWMutex
	states   map[string]serviceHealth
	stop     chan struct{}
}

var appHealth *healthManager

func newHealthManager(store *Store, registry *sessionRegistry) *healthManager {
	return &healthManager{store: store, registry: registry, states: make(map[string]serviceHealth), stop: make(chan struct{})}
}

func healthKey(tunnelID, serviceID string) string { return tunnelID + "/" + serviceID }

func (h *healthManager) start() {
	go func() {
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			h.checkAll()
		case <-h.stop:
			return
		}
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.checkAll()
			case <-h.stop:
				return
			}
		}
	}()
}

func (h *healthManager) checkAll() {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, tunnel := range h.store.snapshot() {
		for _, svc := range tunnel.Services {
			tunnel, svc := tunnel, svc
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				h.checkService(tunnel, svc)
			}()
		}
	}
	wg.Wait()
}

func (h *healthManager) checkService(t Tunnel, svc Service) {
	now := time.Now().UTC()
	if !t.Enabled || !svc.Enabled {
		h.set(serviceHealth{TunnelID: t.ID, ServiceID: svc.ID, Status: "disabled", CheckedAt: now}, t, svc)
		return
	}
	if !h.registry.online(t.ID) {
		h.set(serviceHealth{TunnelID: t.ID, ServiceID: svc.ID, Status: "offline", Error: "no connector online", CheckedAt: now}, t, svc)
		return
	}
	status, connector, latency, err := h.registry.probe(t.ID, t.PreferredConnectorID, svc)
	state := serviceHealth{TunnelID: t.ID, ServiceID: svc.ID, HTTPStatus: status, Connector: connector, LatencyMS: latency.Milliseconds(), CheckedAt: now}
	if err != nil {
		state.Status = "unhealthy"
		state.Error = err.Error()
	} else if status >= 100 && status < 500 {
		state.Status = "healthy"
	} else {
		state.Status = "unhealthy"
		state.Error = fmt.Sprintf("upstream returned HTTP %d", status)
	}
	h.set(state, t, svc)
}

func (h *healthManager) set(next serviceHealth, t Tunnel, svc Service) {
	key := healthKey(next.TunnelID, next.ServiceID)
	h.mu.Lock()
	prev, existed := h.states[key]
	h.states[key] = next
	h.mu.Unlock()
	if !existed || prev.Status != next.Status || prev.Connector != next.Connector {
		level := "info"
		if next.Status == "unhealthy" || next.Status == "offline" {
			level = "warning"
		}
		msg := fmt.Sprintf("%s is %s", svc.Name, next.Status)
		if svc.Name == "" {
			msg = fmt.Sprintf("%s is %s", svc.Host, next.Status)
		}
		if next.Status == "healthy" && next.LatencyMS > 0 {
			msg += fmt.Sprintf(" (%d ms)", next.LatencyMS)
		}
		if next.Error != "" {
			msg += ": " + next.Error
		}
		appEvents.add(level, "service.health", t.ID, next.Connector, svc.ID, msg)
	}
}

func (h *healthManager) snapshot() []serviceHealth {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]serviceHealth, 0, len(h.states))
	for _, s := range h.states {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TunnelID != out[j].TunnelID {
			return out[i].TunnelID < out[j].TunnelID
		}
		return out[i].ServiceID < out[j].ServiceID
	})
	return out
}

func (a *apiServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if appHealth == nil {
		writeJSON(w, []serviceHealth{})
		return
	}
	writeJSON(w, appHealth.snapshot())
}

func (r *sessionRegistry) probe(tunnelID, preferred string, svc Service) (int, string, time.Duration, error) {
	start := time.Now()
	s, connectorID, ok := r.pickSession(tunnelID, preferred, map[string]bool{})
	if !ok {
		return 0, "", 0, errTunnelOffline
	}
	stream, err := s.yamux.Open()
	if err != nil {
		return 0, connectorID, 0, err
	}
	defer stream.Close()
	if d, ok := any(stream).(interface{ SetDeadline(time.Time) error }); ok {
		_ = d.SetDeadline(time.Now().Add(8 * time.Second))
	}
	head := wire.RequestHead{
		Target:        svc.Target,
		Method:        http.MethodHead,
		URL:           "/",
		Host:          svc.Host,
		Headers:       map[string]string{"User-Agent": "Zoraxy-Tunnel-Enhanced-Health/1.9"},
		SkipTLSVerify: svc.SkipTLSVerify,
	}
	if err := wire.WriteJSON(stream, head); err != nil {
		return 0, connectorID, time.Since(start), err
	}
	if err := wire.WriteBody(stream, http.NoBody); err != nil {
		return 0, connectorID, time.Since(start), err
	}
	var resp wire.ResponseHead
	if err := wire.ReadJSON(stream, &resp); err != nil {
		return 0, connectorID, time.Since(start), err
	}
	_ = wire.ReadBody(io.Discard, stream)
	return resp.Status, connectorID, time.Since(start), nil
}
