package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// apiServer wires the /ui/api/* endpoints to the store, live sessions and the
// Zoraxy back-channel.
type apiServer struct {
	store       *Store
	registry    *sessionRegistry
	certs       *certManager
	zPort       int
	apiKey      string
	ingressPort int
	controlPort int
	uiPort      int
	version     string
}

const maxBody = 1 << 16 // 64 KiB is plenty for every endpoint here

type statusResp struct {
	Fingerprint string `json:"fingerprint"`
	ServerHost  string `json:"server_host"`
	DefaultTag  string `json:"default_tag"`
	IngressPort int    `json:"ingress_port"`
	ControlPort int    `json:"control_port"`
	UIPort      int    `json:"ui_port"`
	TunnelCount int    `json:"tunnel_count"`
	OnlineCount int    `json:"online_count"`
	Version     string `json:"version"`
}

func (a *apiServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	tunnels := a.store.snapshot()
	online := 0
	for _, t := range tunnels {
		if a.registry.online(t.ID) {
			online++
		}
	}
	writeJSON(w, statusResp{
		Fingerprint: a.certs.Fingerprint(),
		ServerHost:  a.store.serverHost(),
		DefaultTag:  a.store.defaultTag(),
		IngressPort: a.ingressPort,
		ControlPort: a.controlPort,
		UIPort:      a.uiPort,
		TunnelCount: len(tunnels),
		OnlineCount: online,
		Version:     a.version,
	})
}

func (a *apiServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		serverHost, defaultTag := a.store.settings()
		writeJSON(w, map[string]string{"server_host": serverHost, "default_tag": defaultTag})
		return
	}
	var body struct {
		ServerHost string `json:"server_host"`
		DefaultTag string `json:"default_tag"`
	}
	if err := decode(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.store.setSettings(body.ServerHost, body.DefaultTag); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{
		"server_host": a.store.serverHost(),
		"default_tag": a.store.defaultTag(),
	})
}

// tunnelView is what we hand to the UI. Token is only present at creation or
// regeneration time — never on plain listings.
type tunnelView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHint string    `json:"token_hint"`
	Online    bool      `json:"online"`
	Enabled   bool      `json:"enabled"`
	Created   time.Time `json:"created"`
	Services  []Service `json:"services"`
}

func toView(t Tunnel, online bool) tunnelView {
	return tunnelView{
		ID: t.ID, Name: t.Name, TokenHint: t.TokenHint,
		Online: online, Enabled: t.Enabled, Created: t.Created, Services: t.Services,
	}
}

func (a *apiServer) handleTunnels(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		tunnels := a.store.snapshot()
		out := make([]tunnelView, 0, len(tunnels))
		for _, t := range tunnels {
			out = append(out, toView(t, a.registry.online(t.ID)))
		}
		writeJSON(w, out)
		return
	}
	var body struct{ Name string `json:"name"` }
	if err := decode(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	plain, hash, hint := newToken()
	t := Tunnel{
		ID: newID("t_"), Name: body.Name, TokenHash: hash,
		TokenHint: hint, Created: time.Now().UTC(), Enabled: true,
	}
	if err := a.store.addTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"tunnel": toView(t, false),
		"token":  plain, // plaintext — shown once, never persisted
	})
}

func (a *apiServer) handleTunnelAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string `json:"id"`
		Action string `json:"action"` // delete | regenerate | toggle
	}
	if err := decode(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch body.Action {
	case "delete":
		a.deleteTunnel(w, r, body.ID)
	case "regenerate":
		a.regenerateToken(w, r, body.ID)
	case "toggle":
		a.toggleTunnel(w, r, body.ID)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func (a *apiServer) deleteTunnel(w http.ResponseWriter, r *http.Request, id string) {
	t, ok := a.store.deleteTunnel(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	a.cleanupRoutes(r, t.Services)
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *apiServer) regenerateToken(w http.ResponseWriter, r *http.Request, id string) {
	t, ok := a.store.tunnel(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	plain, hash, hint := newToken()
	t.TokenHash = hash
	t.TokenHint = hint
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"tunnel": toView(t, a.registry.online(id)),
		"token":  plain,
	})
}

func (a *apiServer) toggleTunnel(w http.ResponseWriter, r *http.Request, id string) {
	t, ok := a.store.tunnel(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	t.Enabled = !t.Enabled
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, toView(t, a.registry.online(id)))
}

type serviceActionBody struct {
	TunnelID      string  `json:"tunnel_id"`
	ServiceID     string  `json:"service_id"`
	Action        string  `json:"action"`
	Name          string  `json:"name"`
	Host          string  `json:"host"`
	Path          string  `json:"path"`
	Target        string  `json:"target"`
	SkipTLSVerify bool    `json:"skip_tls_verify"`
	Tag           *string `json:"tag"`
	IssueCert     bool    `json:"issue_cert"`
}

func (a *apiServer) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	var body serviceActionBody
	if err := decode(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch body.Action {
	case "add":
		a.addService(w, r, &body)
	case "edit":
		a.editService(w, r, &body)
	case "delete":
		a.deleteService(w, r, body.TunnelID, body.ServiceID)
	case "install":
		a.installService(w, r, &body)
	case "uninstall":
		a.uninstallService(w, r, body.TunnelID, body.ServiceID)
	case "toggle":
		a.toggleService(w, r, body.TunnelID, body.ServiceID)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func (a *apiServer) addService(w http.ResponseWriter, r *http.Request, b *serviceActionBody) {
	if err := validateTarget(b.Target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if b.Host == "" {
		http.Error(w, "host required", http.StatusBadRequest)
		return
	}
	t, ok := a.store.tunnel(b.TunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}
	tag := a.store.defaultTag()
	if b.Tag != nil {
		tag = strings.TrimSpace(*b.Tag)
	}
	t.Services = append(t.Services, Service{
		ID: newID("s_"), Name: b.Name, Host: b.Host, Path: b.Path,
		Target: b.Target, SkipTLSVerify: b.SkipTLSVerify, Tag: tag, Enabled: true,
	})
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, toView(t, a.registry.online(t.ID)))
}

func (a *apiServer) editService(w http.ResponseWriter, r *http.Request, b *serviceActionBody) {
	if err := validateTarget(b.Target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if b.Host == "" {
		http.Error(w, "host required", http.StatusBadRequest)
		return
	}
	t, ok := a.store.tunnel(b.TunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}
	old, idx := findService(t, b.ServiceID)
	if idx < 0 {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}

	updated := old
	updated.Name = b.Name
	updated.Host = b.Host
	updated.Path = b.Path
	updated.Target = b.Target
	updated.SkipTLSVerify = b.SkipTLSVerify
	if b.Tag != nil {
		updated.Tag = strings.TrimSpace(*b.Tag)
	}

	if old.InstalledRoute != "" {
		if a.apiKey == "" {
			http.Error(w, "plugin has no Zoraxy API key", http.StatusForbidden)
			return
		}
		csrf, cookie := extractCSRF(r), extractCookie(r)
		if !strings.EqualFold(old.InstalledRoute, updated.Host) {
			target := fmt.Sprintf("127.0.0.1:%d", a.ingressPort)
			if err := installRoute(a.zPort, a.apiKey, csrf, cookie, updated.Host, target); err != nil {
				http.Error(w, "could not move installed route: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if err := setRouteTags(a.zPort, a.apiKey, csrf, cookie, updated.Host, updated.Tag); err != nil {
				_ = removeRoute(a.zPort, a.apiKey, csrf, cookie, updated.Host)
				http.Error(w, "could not tag new route: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if err := removeRoute(a.zPort, a.apiKey, csrf, cookie, old.InstalledRoute); err != nil {
				_ = removeRoute(a.zPort, a.apiKey, csrf, cookie, updated.Host)
				http.Error(w, "could not remove previous route: "+err.Error(), http.StatusInternalServerError)
				return
			}
			updated.InstalledRoute = updated.Host
		} else if err := setRouteTags(a.zPort, a.apiKey, csrf, cookie, old.InstalledRoute, updated.Tag); err != nil {
			http.Error(w, "could not update route tags: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	t.Services[idx] = updated
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, toView(t, a.registry.online(t.ID)))
}

func (a *apiServer) deleteService(w http.ResponseWriter, r *http.Request, tunnelID, serviceID string) {
	t, ok := a.store.tunnel(tunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}
	for i, svc := range t.Services {
		if svc.ID == serviceID {
			if svc.InstalledRoute != "" {
				a.tryRemoveRoute(r, svc.InstalledRoute)
			}
			t.Services = append(t.Services[:i], t.Services[i+1:]...)
			break
		}
	}
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, toView(t, a.registry.online(t.ID)))
}

func (a *apiServer) installService(w http.ResponseWriter, r *http.Request, b *serviceActionBody) {
	t, ok := a.store.tunnel(b.TunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}
	svc, idx := findService(t, b.ServiceID)
	if idx < 0 {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if a.apiKey == "" {
		http.Error(w, "plugin has no Zoraxy API key", http.StatusForbidden)
		return
	}
	csrf, cookie := extractCSRF(r), extractCookie(r)
	target := fmt.Sprintf("127.0.0.1:%d", a.ingressPort)
	if err := installRoute(a.zPort, a.apiKey, csrf, cookie, svc.Host, target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t.Services[idx].InstalledRoute = svc.Host
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if svc.Tag != "" {
		if err := setRouteTags(a.zPort, a.apiKey, csrf, cookie, svc.Host, svc.Tag); err != nil {
			http.Error(w, "route installed, but tag assignment failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	// Issue after the route is saved, so a cert failure still leaves the route installed.
	if b.IssueCert {
		z := newZoraxyClient(a.zPort, a.apiKey, csrf, cookie)
		if err := z.issueCertificate(svc.Host); err != nil {
			http.Error(w, "route installed, but certificate failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, toView(t, a.registry.online(t.ID)))
}

func (a *apiServer) uninstallService(w http.ResponseWriter, r *http.Request, tunnelID, serviceID string) {
	t, ok := a.store.tunnel(tunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}
	svc, idx := findService(t, serviceID)
	if idx < 0 {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if svc.InstalledRoute != "" {
		a.tryRemoveRoute(r, svc.InstalledRoute)
	}
	t.Services[idx].InstalledRoute = ""
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, toView(t, a.registry.online(t.ID)))
}

func (a *apiServer) toggleService(w http.ResponseWriter, r *http.Request, tunnelID, serviceID string) {
	t, ok := a.store.tunnel(tunnelID)
	if !ok {
		http.Error(w, "tunnel not found", http.StatusNotFound)
		return
	}
	_, idx := findService(t, serviceID)
	if idx < 0 {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	t.Services[idx].Enabled = !t.Services[idx].Enabled
	if err := a.store.updateTunnel(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, toView(t, a.registry.online(t.ID)))
}

// cleanupRoutes removes every installed Zoraxy rule a tunnel owned. Best
// effort: a failed delete is logged but doesn't abort the tunnel deletion.
func (a *apiServer) cleanupRoutes(r *http.Request, services []Service) {
	seen := map[string]bool{}
	for _, svc := range services {
		if svc.InstalledRoute == "" || seen[svc.InstalledRoute] {
			continue
		}
		seen[svc.InstalledRoute] = true
		a.tryRemoveRoute(r, svc.InstalledRoute)
	}
}

func (a *apiServer) tryRemoveRoute(r *http.Request, host string) {
	if a.apiKey == "" {
		return
	}
	if err := removeRoute(a.zPort, a.apiKey, extractCSRF(r), extractCookie(r), host); err != nil {
		log.Printf("[tunnel] remove route %s: %v", host, err)
	}
}

func findService(t Tunnel, id string) (Service, int) {
	for i, s := range t.Services {
		if s.ID == id {
			return s, i
		}
	}
	return Service{}, -1
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(v)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
