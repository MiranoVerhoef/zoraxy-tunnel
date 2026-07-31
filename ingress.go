package main

import (
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

	"zoraxy-tunnel/wire"
)

// ingressServer is the plaintext :9080 listener. Zoraxy routes public host
// rules here; we dispatch by Host header to the matching service + tunnel and
// relay the request through the live client connection.
type ingressServer struct {
	store    *Store
	registry *sessionRegistry
}

func newIngressServer(store *Store, registry *sessionRegistry) *ingressServer {
	return &ingressServer{store: store, registry: registry}
}

func (g *ingressServer) listenAndServe(addr string) error {
	log.Printf("[tunnel] ingress http on %s", addr)
	return http.ListenAndServe(addr, g)
}

func (g *ingressServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := stripPort(r.Host)
	service, tunnelID, ok := g.resolve(host, r.URL.Path)
	if !ok {
		http.Error(w, "no tunnel service for "+host, http.StatusBadGateway)
		return
	}
	if !g.registry.online(tunnelID) {
		http.Error(w, "tunnel offline", http.StatusBadGateway)
		return
	}

	isWS := isWebSocketUpgrade(r)
	head := wire.RequestHead{
		Target:      service.Target,
		Method:      r.Method,
		URL:         r.URL.RequestURI(),
		Host:        r.Host,
		Headers:     flattenHeaders(r.Header),
		IsWebSocket: isWS,
	}

	stream, err := g.registry.forward(tunnelID, head, r.Body)
	if err != nil {
		if err == errTunnelOffline {
			http.Error(w, "tunnel offline", http.StatusBadGateway)
		} else {
			http.Error(w, "tunnel error", http.StatusBadGateway)
		}
		return
	}
	defer stream.Close()

	var resp wire.ResponseHead
	if err := wire.ReadJSON(stream, &resp); err != nil {
		http.Error(w, "tunnel read error", http.StatusBadGateway)
		return
	}

	if isWS {
		g.serveWebSocket(w, r, stream, resp)
		return
	}

	copyHeaders(w.Header(), resp.Headers)
	w.WriteHeader(resp.Status)
	if err := wire.ReadBody(w, stream); err != nil && !isClosedConnErr(err) {
		log.Printf("[tunnel] body copy: %v", err)
	}
}

// serveWebSocket completes the 101 handshake, then pipes the hijacked socket
// against the yamux stream in both directions using the framed transport.
func (g *ingressServer) serveWebSocket(w http.ResponseWriter, r *http.Request, stream io.ReadWriteCloser, resp wire.ResponseHead) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket not supported", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	defer bufrw.Flush()

	// replay the upstream's 101 line + headers to the browser verbatim
	bufrw.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	for k, v := range resp.Headers {
		bufrw.WriteString(k)
		bufrw.WriteString(": ")
		bufrw.WriteString(v)
		bufrw.WriteString("\r\n")
	}
	bufrw.WriteString("\r\n")
	bufrw.Flush()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = wire.PumpRawToFrames(stream, conn) // browser -> client
	}()
	go func() {
		defer wg.Done()
		_ = wire.PumpFramesToRaw(conn, stream) // client -> browser
	}()
	wg.Wait()
}

// resolve finds the service for a host (+ optional path prefix) and the tunnel
// it belongs to. Longest path-prefix wins when several services share a host.
func (g *ingressServer) resolve(host, path string) (Service, string, bool) {
	for _, t := range g.store.snapshot() {
		var best *Service
		for i := range t.Services {
			svc := &t.Services[i]
			if !svc.Enabled || !strings.EqualFold(svc.Host, host) {
				continue
			}
			if svc.Path != "" && !strings.HasPrefix(path, svc.Path) {
				continue
			}
			if best == nil || len(svc.Path) > len(best.Path) {
				best = svc
			}
		}
		if best != nil {
			return *best, t.ID, true
		}
	}
	return Service{}, "", false
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Connection"), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func stripPort(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		return host
	}
	return h
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		// drop pure hop-by-hop framing; keep Connection/Upgrade so the relay can
		// replay a websocket handshake to the upstream
		switch strings.ToLower(k) {
		case "keep-alive", "proxy-authenticate", "proxy-authorization",
			"te", "trailer", "transfer-encoding":
			continue
		}
		out[k] = strings.Join(vs, ", ")
	}
	return out
}

func copyHeaders(dst http.Header, src map[string]string) {
	for k, v := range src {
		switch strings.ToLower(k) {
		case "content-length", "transfer-encoding", "connection":
			continue
		}
		dst.Set(k, v)
	}
}

func isClosedConnErr(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "broken pipe") || strings.Contains(s, "connection reset")
}
