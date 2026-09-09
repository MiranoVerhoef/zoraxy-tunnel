package main

import (
	"net/http"
	"strings"
)

func (a *apiServer) handlePreferredConnector(w http.ResponseWriter,r *http.Request){
	if r.Method!=http.MethodPost{http.Error(w,"method not allowed",http.StatusMethodNotAllowed);return}
	var body struct{TunnelID string `json:"tunnel_id"`;ConnectorID string `json:"connector_id"`};if err:=decode(r,&body);err!=nil{http.Error(w,err.Error(),http.StatusBadRequest);return}
	t,ok:=a.store.tunnel(body.TunnelID);if !ok{http.Error(w,"tunnel not found",http.StatusNotFound);return};t.PreferredConnectorID=strings.TrimSpace(body.ConnectorID);if err:=a.store.updateTunnel(t);err!=nil{http.Error(w,err.Error(),http.StatusInternalServerError);return}
	writeJSON(w,map[string]string{"preferred_connector_id":t.PreferredConnectorID,"active_connector_id":a.registry.activeConnectorID(t.ID,t.PreferredConnectorID)})
}
