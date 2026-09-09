package main

import (
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"zoraxy-tunnel/wire"
)

var errTunnelOffline = errors.New("tunnel offline")

type session struct {
	yamux *yamux.Session
	connectorID string
	joined time.Time
	mu sync.RWMutex
	remoteAddr string
	clientVersion string
	clientHostname string
	clientOS string
	clientArch string
	lastActivity time.Time
	requests uint64
	bytesToClient uint64
	bytesFromClient uint64
	activeStreams int
	connectionCount uint64
}

type sessionStats struct {
	ConnectorID string
	Online bool
	Joined time.Time
	LastActivity time.Time
	RemoteAddr string
	ClientVersion string
	ClientHostname string
	ClientOS string
	ClientArch string
	Requests uint64
	BytesToClient uint64
	BytesFromClient uint64
	ActiveStreams int
	ConnectionCount uint64
}

type sessionRegistry struct {
	mu sync.RWMutex
	sessions map[string]map[string]*session
	last map[string]map[string]sessionStats
	connections map[string]map[string]uint64
	active map[string]string
}

func newSessionRegistry() *sessionRegistry { return &sessionRegistry{sessions:make(map[string]map[string]*session),last:make(map[string]map[string]sessionStats),connections:make(map[string]map[string]uint64),active:make(map[string]string)} }

func (r *sessionRegistry) register(tunnelID, connectorID string, s *session) (replaced bool) {
	r.mu.Lock()
	if r.sessions[tunnelID] == nil { r.sessions[tunnelID] = make(map[string]*session) }
	if r.last[tunnelID] == nil { r.last[tunnelID] = make(map[string]sessionStats) }
	if r.connections[tunnelID] == nil { r.connections[tunnelID] = make(map[string]uint64) }
	r.connections[tunnelID][connectorID]++
	s.mu.Lock(); s.connectorID = connectorID; s.connectionCount = r.connections[tunnelID][connectorID]; if s.lastActivity.IsZero() { s.lastActivity = s.joined }; s.mu.Unlock()
	old := r.sessions[tunnelID][connectorID]; if old != nil && old.yamux != s.yamux { replaced = true }
	r.sessions[tunnelID][connectorID] = s; if r.active[tunnelID] == "" { r.active[tunnelID] = connectorID }; r.mu.Unlock()
	if replaced { _ = old.yamux.Close() }
	return replaced
}

func (r *sessionRegistry) unregister(tunnelID, connectorID string, s *session) {
	r.mu.Lock(); defer r.mu.Unlock(); byConnector := r.sessions[tunnelID]; if byConnector == nil { return }
	if cur,ok := byConnector[connectorID]; ok && cur == s { s.touch(); if r.last[tunnelID] == nil { r.last[tunnelID] = make(map[string]sessionStats) }; r.last[tunnelID][connectorID] = s.snapshot(false); delete(byConnector,connectorID); if len(byConnector)==0 { delete(r.sessions,tunnelID) }; if r.active[tunnelID]==connectorID { delete(r.active,tunnelID) } }
}

func (r *sessionRegistry) online(tunnelID string) bool { r.mu.RLock(); defer r.mu.RUnlock(); for _,s := range r.sessions[tunnelID] { if s != nil && !s.yamux.IsClosed() { return true } }; return false }
func (r *sessionRegistry) onlineCount(tunnelID string) int { r.mu.RLock(); defer r.mu.RUnlock(); n:=0; for _,s:=range r.sessions[tunnelID] { if s!=nil && !s.yamux.IsClosed(){n++} }; return n }

func (r *sessionRegistry) pickSession(tunnelID, preferred string, excluded map[string]bool) (*session,string,bool) {
	r.mu.Lock(); defer r.mu.Unlock(); live:=r.sessions[tunnelID]
	valid:=func(id string)(*session,bool){ if id==""||excluded[id]{return nil,false}; s,ok:=live[id]; if !ok||s==nil||s.yamux.IsClosed(){return nil,false}; return s,true }
	if s,ok:=valid(preferred);ok { r.active[tunnelID]=preferred; return s,preferred,true }
	if active:=r.active[tunnelID]; active!="" { if s,ok:=valid(active);ok{return s,active,true}; delete(r.active,tunnelID) }
	ids:=make([]string,0,len(live)); for id,s:=range live { if excluded[id]||s==nil||s.yamux.IsClosed(){continue}; ids=append(ids,id) }; if len(ids)==0{return nil,"",false}
	sort.Slice(ids,func(i,j int)bool{a,b:=live[ids[i]],live[ids[j]]; if a.joined.Equal(b.joined){return ids[i]<ids[j]}; return a.joined.Before(b.joined)})
	chosen:=ids[0]; r.active[tunnelID]=chosen; return live[chosen],chosen,true
}
func (r *sessionRegistry) activeConnectorID(tunnelID, preferred string) string { _,id,ok:=r.pickSession(tunnelID,preferred,map[string]bool{}); if !ok{return ""}; return id }

func (r *sessionRegistry) statsAll(tunnelID string) []sessionStats {
	r.mu.RLock(); defer r.mu.RUnlock(); merged:=make(map[string]sessionStats)
	for id,st:=range r.last[tunnelID]{merged[id]=st}; for id,s:=range r.sessions[tunnelID]{if s!=nil&&!s.yamux.IsClosed(){merged[id]=s.snapshot(true)}}
	out:=make([]sessionStats,0,len(merged)); for _,st:=range merged{out=append(out,st)}
	sort.Slice(out,func(i,j int)bool{if out[i].Online!=out[j].Online{return out[i].Online}; return out[i].ConnectorID<out[j].ConnectorID}); return out
}

func (s *session) touch(){s.mu.Lock();s.lastActivity=time.Now().UTC();s.mu.Unlock()}
func (s *session) streamOpened(){s.mu.Lock();s.requests++;s.activeStreams++;s.lastActivity=time.Now().UTC();s.mu.Unlock()}
func (s *session) streamClosed(){s.mu.Lock();if s.activeStreams>0{s.activeStreams--};s.lastActivity=time.Now().UTC();s.mu.Unlock()}
func (s *session) recordToClient(n int){if n<=0{return};s.mu.Lock();s.bytesToClient+=uint64(n);s.lastActivity=time.Now().UTC();s.mu.Unlock()}
func (s *session) recordFromClient(n int){if n<=0{return};s.mu.Lock();s.bytesFromClient+=uint64(n);s.lastActivity=time.Now().UTC();s.mu.Unlock()}
func (s *session) snapshot(online bool) sessionStats { s.mu.RLock(); defer s.mu.RUnlock(); return sessionStats{ConnectorID:s.connectorID,Online:online,Joined:s.joined,LastActivity:s.lastActivity,RemoteAddr:s.remoteAddr,ClientVersion:s.clientVersion,ClientHostname:s.clientHostname,ClientOS:s.clientOS,ClientArch:s.clientArch,Requests:s.requests,BytesToClient:s.bytesToClient,BytesFromClient:s.bytesFromClient,ActiveStreams:s.activeStreams,ConnectionCount:s.connectionCount} }

type trackedStream struct{io.ReadWriteCloser;session *session;once sync.Once}
func (t *trackedStream) Read(p []byte)(int,error){n,err:=t.ReadWriteCloser.Read(p);t.session.recordFromClient(n);return n,err}
func (t *trackedStream) Write(p []byte)(int,error){n,err:=t.ReadWriteCloser.Write(p);t.session.recordToClient(n);return n,err}
func (t *trackedStream) Close() error {err:=t.ReadWriteCloser.Close();t.once.Do(t.session.streamClosed);return err}

func (r *sessionRegistry) forward(tunnelID, preferred string, head wire.RequestHead, body io.Reader) (io.ReadWriteCloser,error) {
	excluded:=make(map[string]bool)
	for { s,connectorID,ok:=r.pickSession(tunnelID,preferred,excluded); if !ok{return nil,errTunnelOffline}; stream,err:=s.yamux.Open(); if err!=nil{excluded[connectorID]=true;_ = s.yamux.Close();continue}; s.streamOpened(); tracked:=&trackedStream{ReadWriteCloser:stream,session:s}; if err:=wire.WriteJSON(tracked,head);err!=nil{tracked.Close();excluded[connectorID]=true;_ = s.yamux.Close();continue}; if !head.IsWebSocket{if err:=wire.WriteBody(tracked,body);err!=nil{tracked.Close();return nil,err}}; return tracked,nil }
}
