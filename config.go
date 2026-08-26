package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultServiceTag = "ZoraxyTunnel"

type Service struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Host           string `json:"host"`                      // public hostname the Zoraxy rule + ingress dispatch on
	Path           string `json:"path,omitempty"`            // optional path prefix; empty matches any
	Target         string `json:"target"`                    // client-local upstream, e.g. http://127.0.0.1:3000
	SkipTLSVerify  bool   `json:"skip_tls_verify,omitempty"` // allow self-signed/untrusted HTTPS targets on the client
	Tag            string `json:"tag,omitempty"`             // comma-separated Zoraxy tags applied to the installed route
	InstalledRoute string `json:"installed_route,omitempty"` // rootname when a Zoraxy rule exists, "" otherwise
	Enabled        bool   `json:"enabled"`
}

type Tunnel struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"token_hash"` // sha256 of the plaintext token
	TokenHint string    `json:"token_hint"` // last 4 chars, just so the UI can tell tokens apart
	Created   time.Time `json:"created"`
	Enabled   bool      `json:"enabled"`
	Services  []Service `json:"services"`
}

type configFile struct {
	ServerHost string   `json:"server_host"`           // public addr clients dial, e.g. tunnel.example.com:9443
	DefaultTag string   `json:"default_tag,omitempty"` // default Zoraxy tag for newly registered services
	Tunnels    []Tunnel `json:"tunnels"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	data configFile
}

func newStore(dir string) *Store {
	return &Store{
		path: filepath.Join(dir, "config.json"),
		data: configFile{DefaultTag: defaultServiceTag},
	}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(raw, &s.data)
}

func (s *Store) saveLocked() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0600)
}

func (s *Store) snapshot() []Tunnel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Tunnel, len(s.data.Tunnels))
	copy(out, s.data.Tunnels)
	return out
}

func (s *Store) serverHost() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.ServerHost
}

func (s *Store) defaultTag() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.DefaultTag
}

func (s *Store) settings() (serverHost, defaultTag string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.ServerHost, s.data.DefaultTag
}

func (s *Store) setSettings(host, defaultTag string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.ServerHost = strings.TrimSpace(host)
	s.data.DefaultTag = strings.TrimSpace(defaultTag)
	return s.saveLocked()
}

func (s *Store) setServerHost(host string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.ServerHost = host
	return s.saveLocked()
}

func (s *Store) tunnel(id string) (Tunnel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.data.Tunnels {
		if t.ID == id {
			return t, true
		}
	}
	return Tunnel{}, false
}

// newToken returns a fresh random token plus its sha256 hash. The plaintext
// is shown to the user exactly once; we only persist the hash.
func newToken() (plain, hash, hint string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	plain = "zt_" + hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plain))
	hash = "sha256:" + hex.EncodeToString(sum[:])
	if len(plain) >= 4 {
		hint = "…" + plain[len(plain)-4:]
	}
	return
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newID(prefix string) string {
	b := make([]byte, 8)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func (s *Store) addTunnel(t Tunnel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Tunnels = append(s.data.Tunnels, t)
	return s.saveLocked()
}

func (s *Store) updateTunnel(t Tunnel) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Tunnels {
		if s.data.Tunnels[i].ID == t.ID {
			s.data.Tunnels[i] = t
			return s.saveLocked()
		}
	}
	return fmt.Errorf("tunnel not found")
}

func (s *Store) deleteTunnel(id string) (Tunnel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.data.Tunnels {
		if t.ID == id {
			s.data.Tunnels = append(s.data.Tunnels[:i], s.data.Tunnels[i+1:]...)
			s.saveLocked()
			return t, true
		}
	}
	return Tunnel{}, false
}

func validateTarget(s string) error {
	if s == "" {
		return fmt.Errorf("target is required")
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("target must be http(s)")
	}
	if u.Host == "" {
		return fmt.Errorf("target missing host:port")
	}
	return nil
}
