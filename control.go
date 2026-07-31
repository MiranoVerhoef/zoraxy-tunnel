package main

import (
	"crypto/tls"
	"log"
	"net"
	"time"

	"github.com/hashicorp/yamux"

	"zoraxy-tunnel/wire"
)

// controlServer is the :9443 TLS endpoint tunnel clients connect to. It pins
// identity via the self-signed cert (client checks fingerprint) and authorizes
// via the tunnel token.
type controlServer struct {
	tls      *tls.Config
	store    *Store
	registry *sessionRegistry
}

func newControlServer(tlsCfg *tls.Config, store *Store, registry *sessionRegistry) *controlServer {
	return &controlServer{tls: tlsCfg, store: store, registry: registry}
}

func (c *controlServer) listenAndServe(addr string) error {
	ln, err := tls.Listen("tcp", addr, c.tls)
	if err != nil {
		return err
	}
	log.Printf("[tunnel] control tls on %s", addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go c.handle(conn)
	}
}

func (c *controlServer) handle(conn net.Conn) {
	remote := conn.RemoteAddr().String()
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	// first stream the client opens carries the auth handshake
	sess, err := yamux.Server(conn, yamux.DefaultConfig())
	if err != nil {
		log.Printf("[tunnel] yamux from %s: %v", remote, err)
		return
	}
	auth, err := sess.Accept()
	if err != nil {
		sess.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{}) // handshake done, clear deadline

	var req wire.AuthReq
	if err := wire.ReadJSON(auth, &req); err != nil {
		auth.Close()
		sess.Close()
		return
	}

	tunnelID, ok := c.authorize(req.Token)
	resp := wire.AuthResp{OK: ok, TunnelID: tunnelID}
	if !ok {
		resp.Error = "invalid token"
	}
	_ = wire.WriteJSON(auth, resp)
	auth.Close()
	if !ok {
		sess.Close()
		return
	}

	s := &session{yamux: sess, joined: time.Now()}
	if c.registry.register(tunnelID, s) {
		log.Printf("[tunnel] replaced previous client for %s", tunnelID)
	}
	log.Printf("[tunnel] client connected for %s from %s", tunnelID, remote)

	// block until the client disappears
	<-sess.CloseChan()
	c.registry.unregister(tunnelID, s)
	log.Printf("[tunnel] client disconnected for %s", tunnelID)
}

// authorize maps a plaintext token back to a tunnel id by hashing it and
// comparing against the stored hashes.
func (c *controlServer) authorize(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	h := hashToken(token)
	for _, t := range c.store.snapshot() {
		if t.Enabled && t.TokenHash == h {
			return t.ID, true
		}
	}
	return "", false
}
