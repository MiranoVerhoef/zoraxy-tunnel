package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	zp "zoraxy-tunnel/zoraxy_plugin"
)

//go:embed web/* icon.png
var webFS embed.FS

// static ports: the control port clients dial and the ingress port Zoraxy
// routes to must survive restarts, so they are constants (cf. anubis adapter).
const (
	controlPort = 9443
	ingressPort = 9080
)

const (
	verMajor = 1
	verMinor = 1
	verPatch = 1
)

var pluginVersion = fmt.Sprintf("v%d.%d.%d", verMajor, verMinor, verPatch)

var pluginSpec = &zp.IntroSpect{
	ID:          "com.sniffingsugar.zoraxy-tunnel",
	Name:        "Zoraxy Tunnel",
	Author:      "sniffingsugar",
	Description: "Cloudflare-Tunnel-style reverse tunnel server for Zoraxy.",
	URL:         "https://github.com/sniffingsugar/zoraxy-tunnel",
	Type:        zp.PluginType_Utilities,
	VersionMajor: verMajor, VersionMinor: verMinor, VersionPatch: verPatch,
	UIPath: "/ui",
	PermittedAPIEndpoints: []zp.PermittedAPIEndpoint{
		{Method: "GET", Endpoint: "/api/proxy/list", Reason: "Check installed routes"},
		{Method: "POST", Endpoint: "/api/proxy/add", Reason: "Install a service route"},
		{Method: "POST", Endpoint: "/api/proxy/del", Reason: "Remove a service route"},
		{Method: "POST", Endpoint: "/api/proxy/setTags", Reason: "Assign tags to installed tunnel routes"},
		{Method: "GET", Endpoint: "/api/acme/autoRenew/email", Reason: "Read the ACME email configured in Zoraxy"},
		{Method: "GET", Endpoint: "/api/acme/autoRenew/ca", Reason: "Read the user's preferred ACME CA"},
		{Method: "GET", Endpoint: "/api/acme/obtainCert", Reason: "Issue an SSL certificate for an installed route"},
	},
}

func main() {
	config, err := zp.ServeAndRecvSpec(pluginSpec)
	if err != nil {
		log.Println("[tunnel] dev mode (no -configure flag)")
		config = &zp.ConfigureSpec{Port: 9699}
	}

	zPort := config.ZoraxyPort
	if zPort == 0 {
		zPort = 8000
	}
	uiPort := config.Port

	pluginDir := workingDir()
	log.Printf("[tunnel] data dir: %s", pluginDir)
	if icon, err := webFS.ReadFile("icon.png"); err == nil {
		os.WriteFile(filepath.Join(pluginDir, "icon.png"), icon, 0644)
	}

	certs := newCertManager(pluginDir)
	if err := certs.LoadOrCreate(); err != nil {
		log.Fatalf("[tunnel] cert: %v", err)
	}
	log.Printf("[tunnel] cert fingerprint: %s", certs.Fingerprint())

	store := newStore(pluginDir)
	if err := store.Load(); err != nil {
		log.Printf("[tunnel] config load: %v", err)
	}
	registry := newSessionRegistry()

	api := &apiServer{
		store: store, registry: registry, certs: certs,
		zPort: zPort, apiKey: config.APIKey,
		ingressPort: ingressPort, controlPort: controlPort, uiPort: uiPort,
		version: pluginVersion,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ui/api/status", api.handleStatus)
	mux.HandleFunc("/ui/api/settings", api.handleSettings)
	mux.HandleFunc("/ui/api/tunnels", api.handleTunnels)
	mux.HandleFunc("/ui/api/tunnels/action", api.handleTunnelAction)
	mux.HandleFunc("/ui/api/services/action", api.handleServiceAction)

	ui := zp.NewPluginEmbedUIRouter(pluginSpec.ID, &webFS, "web", "/ui")
	ui.AttachHandlerToMux(mux)
	ui.RegisterTerminateHandler(func() { log.Println("[tunnel] bye") }, mux)

	// control plane: TLS server tunnel clients connect to
	control := newControlServer(certs.TLSConfig(), store, registry)
	go func() {
		if err := control.listenAndServe(fmt.Sprintf("0.0.0.0:%d", controlPort)); err != nil {
			log.Printf("[tunnel] control: %v", err)
		}
	}()

	// data plane: public HTTP Zoraxy routes into
	ingress := newIngressServer(store, registry)
	go func() {
		if err := ingress.listenAndServe(fmt.Sprintf("127.0.0.1:%d", ingressPort)); err != nil {
			log.Printf("[tunnel] ingress: %v", err)
		}
	}()

	log.Printf("[tunnel] ui :%d  ingress :%d  control :%d", uiPort, ingressPort, controlPort)
	log.Fatalf("[tunnel] %v", http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", uiPort), mux))
}

func exePath() string {
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			return resolved
		}
		return exe
	}
	return "."
}

// workingDir returns the directory the plugin should store its state in.
func workingDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return filepath.Dir(exePath())
}
