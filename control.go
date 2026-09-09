package main

import (
	"crypto/tls"
	"log"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/yamux"
	"zoraxy-tunnel/wire"
)

type controlServer struct { tls *tls.Config; store *Store; registry *sessionRegistry }
func newControlServer(tlsCfg *tls.Config, store *Store, registry *sessionRegistry) *controlServer { return &controlServer{tls:tlsCfg, store:store, registry:registry} }

func (c *controlServer) listenAndServe(addr string) error {
	ln, err := tls.Listen("tcp", addr, c.tls); if err != nil { return err }
	log.Printf("[tunnel] control tls on %s", addr)
	for { conn, err := ln.Accept(); if err != nil { return err }; go c.handle(conn) }
}

func (c *controlServer) handle(conn net.Conn) {
	remote := conn.RemoteAddr().String(); defer conn.Close(); _ = conn.SetDeadline(time.Now().Add(10*time.Second))
	sess, err := yamux.Server(conn, yamux.DefaultConfig()); if err != nil { log.Printf("[tunnel] yamux from %s: %v", remote, err); return }
	auth, err := sess.Accept(); if err != nil { sess.Close(); return }; _ = conn.SetDeadline(time.Time{})
	var req wire.AuthReq; if err := wire.ReadJSON(auth, &req); err != nil { auth.Close(); sess.Close(); return }
	tunnelID, ok := c.authorize(req.Token); connectorID := connectorIdentity(req.ConnectorID, req.Hostname, remote)
	resp := wire.AuthResp{OK:ok, TunnelID:tunnelID, ConnectorID:connectorID}; if !ok { resp.Error = "invalid token" }
	_ = wire.WriteJSON(auth, resp); auth.Close(); if !ok { sess.Close(); return }
	now := time.Now().UTC()
	s := &session{yamux:sess, connectorID:connectorID, joined:now, lastActivity:now, remoteAddr:remote, clientVersion:req.Version, clientHostname:req.Hostname, clientOS:req.OS, clientArch:req.Arch}
	if c.registry.register(tunnelID, connectorID, s) { log.Printf("[tunnel] replaced connector %s for %s", connectorID, tunnelID) }
	log.Printf("[tunnel] connector %s connected for %s from %s (%s, %s/%s)", connectorID, tunnelID, remote, req.Version, req.OS, req.Arch)
	<-sess.CloseChan(); c.registry.unregister(tunnelID, connectorID, s); log.Printf("[tunnel] connector %s disconnected for %s", connectorID, tunnelID)
}

func connectorIdentity(explicit, hostname, remote string) string {
	id := strings.TrimSpace(explicit); if id == "" { id = strings.TrimSpace(hostname) }
	if id == "" { if host,_,err := net.SplitHostPort(remote); err == nil { id = host } else { id = remote } }
	id = sanitizeConnectorID(id); if id == "" { id = "connector" }; if len(id) > 128 { id = id[:128] }; return id
}
func sanitizeConnectorID(id string) string {
	var b strings.Builder
	for _, r := range id { if (r>='a'&&r<='z')||(r>='A'&&r<='Z')||(r>='0'&&r<='9')||r=='.'||r=='_'||r=='-' { b.WriteRune(r) } else { b.WriteByte('-') } }
	return strings.Trim(b.String(), "-")
}
func (c *controlServer) authorize(token string) (string,bool) {
	if token == "" { return "",false }; h := hashToken(token)
	for _, t := range c.store.snapshot() { if t.Enabled && t.TokenHash == h { return t.ID,true } }; return "",false
}
