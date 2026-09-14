package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type setupStateFile struct {
	Completed bool `json:"completed"`
	Dismissed bool `json:"dismissed"`
}

type setupStateStore struct {
	mu    sync.RWMutex
	path  string
	state setupStateFile
}

var appSetup *setupStateStore

func newSetupStateStore(dir string) *setupStateStore {
	return &setupStateStore{path: filepath.Join(dir, "setup-state.json")}
}

func (s *setupStateStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(raw, &s.state)
}

func (s *setupStateStore) snapshot() setupStateFile {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *setupStateStore) set(action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch action {
	case "complete":
		s.state.Completed = true
		s.state.Dismissed = false
	case "dismiss":
		s.state.Dismissed = true
	case "reset":
		s.state.Completed = false
		s.state.Dismissed = false
	}
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0600)
}

func (a *apiServer) handleSetupState(w http.ResponseWriter, r *http.Request) {
	if appSetup == nil {
		writeJSON(w, setupStateFile{})
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, appSetup.snapshot())
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := decode(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Action != "complete" && body.Action != "dismiss" && body.Action != "reset" {
		http.Error(w, "unknown setup action", http.StatusBadRequest)
		return
	}
	if err := appSetup.set(body.Action); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, appSetup.snapshot())
}
