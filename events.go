package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type activityEvent struct {
	ID          string    `json:"id"`
	Time        time.Time `json:"time"`
	Level       string    `json:"level"`
	Type        string    `json:"type"`
	TunnelID    string    `json:"tunnel_id,omitempty"`
	ConnectorID string    `json:"connector_id,omitempty"`
	ServiceID   string    `json:"service_id,omitempty"`
	Message     string    `json:"message"`
}

type eventLog struct {
	mu     sync.RWMutex
	path   string
	limit  int
	seq    uint64
	events []activityEvent
}

var appEvents = newEventLog(500)

func newEventLog(limit int) *eventLog {
	if limit < 1 {
		limit = 500
	}
	return &eventLog{limit: limit}
}

func (l *eventLog) setPath(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.path = path
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var events []activityEvent
	if json.Unmarshal(raw, &events) == nil {
		if len(events) > l.limit {
			events = events[len(events)-l.limit:]
		}
		l.events = events
	}
}

func (l *eventLog) add(level, eventType, tunnelID, connectorID, serviceID, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	e := activityEvent{
		ID:          fmt.Sprintf("%d-%d", time.Now().UnixNano(), l.seq),
		Time:        time.Now().UTC(),
		Level:       level,
		Type:        eventType,
		TunnelID:    tunnelID,
		ConnectorID: connectorID,
		ServiceID:   serviceID,
		Message:     message,
	}
	l.events = append(l.events, e)
	if len(l.events) > l.limit {
		l.events = append([]activityEvent(nil), l.events[len(l.events)-l.limit:]...)
	}
	l.saveLocked()
}

func (l *eventLog) saveLocked() {
	if l.path == "" {
		return
	}
	raw, err := json.MarshalIndent(l.events, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(l.path, raw, 0600)
}

func (l *eventLog) list(limit int, tunnelID, eventType string) []activityEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := make([]activityEvent, 0, limit)
	for i := len(l.events) - 1; i >= 0 && len(out) < limit; i-- {
		e := l.events[i]
		if tunnelID != "" && e.TunnelID != tunnelID {
			continue
		}
		if eventType != "" && !strings.HasPrefix(e.Type, eventType) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (a *apiServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	writeJSON(w, appEvents.list(limit, r.URL.Query().Get("tunnel_id"), r.URL.Query().Get("type")))
}
