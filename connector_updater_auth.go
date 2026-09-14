package main

import (
	"strings"
	"sync"
)

type connectorUpdaterCredential struct {
	User     string
	Password string
}

var connectorUpdaterCredentialsState = struct {
	sync.RWMutex
	m map[string]map[string]connectorUpdaterCredential
}{m: make(map[string]map[string]connectorUpdaterCredential)}

// recordConnectorUpdaterCredentials keeps WUD credentials only in memory.
// They arrive over the already TLS-protected connector control channel and are
// never exposed through telemetry, logs, config.json, or the dashboard API.
func recordConnectorUpdaterCredentials(tunnelID, connectorID, user, password string) {
	user = strings.TrimSpace(user)
	password = strings.TrimSpace(password)
	connectorUpdaterCredentialsState.Lock()
	defer connectorUpdaterCredentialsState.Unlock()

	if user == "" || password == "" {
		if byConnector := connectorUpdaterCredentialsState.m[tunnelID]; byConnector != nil {
			delete(byConnector, connectorID)
			if len(byConnector) == 0 {
				delete(connectorUpdaterCredentialsState.m, tunnelID)
			}
		}
		return
	}
	if connectorUpdaterCredentialsState.m[tunnelID] == nil {
		connectorUpdaterCredentialsState.m[tunnelID] = make(map[string]connectorUpdaterCredential)
	}
	connectorUpdaterCredentialsState.m[tunnelID][connectorID] = connectorUpdaterCredential{User: user, Password: password}
}

func connectorUpdaterCredentials(tunnelID, connectorID string) (string, string, bool) {
	connectorUpdaterCredentialsState.RLock()
	defer connectorUpdaterCredentialsState.RUnlock()
	byConnector := connectorUpdaterCredentialsState.m[tunnelID]
	if byConnector == nil {
		return "", "", false
	}
	cred, ok := byConnector[connectorID]
	if !ok || cred.User == "" || cred.Password == "" {
		return "", "", false
	}
	return cred.User, cred.Password, true
}

func connectorUpdaterControlAvailable(tunnelID, connectorID string) bool {
	_, _, ok := connectorUpdaterCredentials(tunnelID, connectorID)
	return ok
}
