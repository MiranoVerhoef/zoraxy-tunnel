package main

import (
	"strings"
	"sync"
)

var connectorModes = struct {
	sync.RWMutex
	m map[string]map[string]string
}{m: make(map[string]map[string]string)}

func normalizeConnectorUpdateMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "auto") {
		return "auto"
	}
	return "manual"
}

func recordConnectorMode(tunnelID, connectorID, mode string) {
	mode = normalizeConnectorUpdateMode(mode)
	connectorModes.Lock()
	defer connectorModes.Unlock()
	if connectorModes.m[tunnelID] == nil {
		connectorModes.m[tunnelID] = make(map[string]string)
	}
	connectorModes.m[tunnelID][connectorID] = mode
}

func connectorUpdateMode(tunnelID, connectorID string) string {
	connectorModes.RLock()
	defer connectorModes.RUnlock()
	if byConnector := connectorModes.m[tunnelID]; byConnector != nil {
		if mode := byConnector[connectorID]; mode != "" {
			return mode
		}
	}
	return "manual"
}
