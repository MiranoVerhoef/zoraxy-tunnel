package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var zoraxyHTTPClient = &http.Client{Timeout: 10 * time.Second}

// slowHTTPClient is for ACME calls that can take a minute or more.
var slowHTTPClient = &http.Client{Timeout: 180 * time.Second}

// zoraxyClient talks to Zoraxy's internal API on localhost. Auth piggybacks
// the api_key the host handed us plus the CSRF token + cookie the browser
// already holds (forwarded from the UI request).
type zoraxyClient struct {
	baseURL              string
	apiKey, csrf, cookie string
}

func newZoraxyClient(port int, apiKey, csrf, cookie string) *zoraxyClient {
	return &zoraxyClient{
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		apiKey:  apiKey, csrf: csrf, cookie: cookie,
	}
}

func (c *zoraxyClient) do(method, path string, params url.Values) ([]byte, int, error) {
	return c.doWith(zoraxyHTTPClient, method, path, params)
}

// doSlow is do() with the long-timeout client, for ACME calls.
func (c *zoraxyClient) doSlow(method, path string, params url.Values) ([]byte, int, error) {
	return c.doWith(slowHTTPClient, method, path, params)
}

func (c *zoraxyClient) doWith(client *http.Client, method, path string, params url.Values) ([]byte, int, error) {
	var body io.Reader
	if params != nil {
		body = strings.NewReader(params.Encode())
	}
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, 0, err
	}
	if params != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", c.apiKey)
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
		req.Header.Set("X-Zoraxy-Csrf", c.csrf)
	}
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

func extractCSRF(r *http.Request) string {
	if t := r.Header.Get("X-CSRF-Token"); t != "" {
		return t
	}
	return r.Header.Get("X-Zoraxy-Csrf")
}

func extractCookie(r *http.Request) string { return r.Header.Get("Cookie") }

// installRoute creates a host proxy rule pointing the public host at this
// plugin's ingress port, so Zoraxy forwards tunneled traffic our way.
func installRoute(port int, apiKey, csrf, cookie, host, ingressTarget string) error {
	z := newZoraxyClient(port, apiKey, csrf, cookie)
	raw, code, err := z.do("POST", "/api/proxy/add", url.Values{
		"type":      {"host"},
		"rootname":  {host},
		"ep":        {ingressTarget},
		"tls":       {"false"},
		"access":    {"default"},
		"enableUtm": {"false"},
	})
	if err != nil {
		return err
	}
	if msg := extractZoraxyError(raw); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	if code >= 400 {
		return fmt.Errorf("zoraxy add route HTTP %d", code)
	}
	return nil
}

func removeRoute(port int, apiKey, csrf, cookie, host string) error {
	z := newZoraxyClient(port, apiKey, csrf, cookie)
	raw, code, err := z.do("POST", "/api/proxy/del", url.Values{"ep": {host}})
	if err != nil {
		return err
	}
	if msg := extractZoraxyError(raw); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	if code >= 400 {
		return fmt.Errorf("zoraxy del route HTTP %d", code)
	}
	return nil
}

// setRouteTags replaces all tags on a host proxy rule. tags is the same
// comma-separated value accepted by Zoraxy's built-in tag editor; an empty
// value clears the tags from the rule.
func setRouteTags(port int, apiKey, csrf, cookie, host, tags string) error {
	z := newZoraxyClient(port, apiKey, csrf, cookie)
	raw, code, err := z.do("POST", "/api/proxy/setTags", url.Values{
		"rootname": {host},
		"tags":     {tags},
	})
	if err != nil {
		return err
	}
	if msg := extractZoraxyError(raw); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	if code >= 400 {
		return fmt.Errorf("zoraxy set tags HTTP %d", code)
	}
	return nil
}

type proxyEndpointInfo struct {
	RootOrMatchingDomain string `json:"RootOrMatchingDomain"`
}

func routeExists(port int, apiKey, csrf, cookie, host string) bool {
	z := newZoraxyClient(port, apiKey, csrf, cookie)
	raw, code, err := z.do("GET", "/api/proxy/list?type=host", nil)
	if err != nil || code >= 400 {
		return false
	}
	var eps []proxyEndpointInfo
	if json.Unmarshal(raw, &eps) != nil {
		return false
	}
	for _, ep := range eps {
		if strings.EqualFold(ep.RootOrMatchingDomain, host) {
			return true
		}
	}
	return false
}

// extractZoraxyError reads Zoraxy's {"error":"..."} reply, which often ships with HTTP 200.
func extractZoraxyError(raw []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil {
		return e.Error
	}
	return ""
}

// acmeGetString returns a Zoraxy endpoint's plain-string reply.
func (c *zoraxyClient) acmeGetString(path string) (string, error) {
	raw, code, err := c.do("GET", path, nil)
	if err != nil {
		return "", err
	}
	if code >= 400 {
		return "", fmt.Errorf("HTTP %d", code)
	}
	if msg := extractZoraxyError(raw); msg != "" {
		return "", fmt.Errorf("%s", msg)
	}
	var s string
	json.Unmarshal(raw, &s)
	return s, nil
}

// issueCertificate mints a cert via Zoraxy's ACME flow; on success it's auto-served (filename match). Wildcards use DNS-01.
func (c *zoraxyClient) issueCertificate(host string) error {
	email, err := c.acmeGetString("/api/acme/autoRenew/email")
	if err != nil {
		return err
	}
	if email == "" {
		return fmt.Errorf("ACME email not set — set it in Zoraxy ACME settings")
	}
	ca, err := c.acmeGetString("/api/acme/autoRenew/ca")
	if err != nil || ca == "" {
		ca = "Let's Encrypt"
	}
	filename := strings.ReplaceAll(host, "*", "_")
	q := url.Values{
		"domains":  {host},
		"filename": {filename},
		"email":    {email},
		"ca":       {ca},
		"dns":      {strconv.FormatBool(strings.HasPrefix(host, "*"))},
	}
	raw, code, err := c.doSlow("GET", "/api/acme/obtainCert?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if msg := extractZoraxyError(raw); msg != "" {
		return fmt.Errorf("%s", msg)
	}
	if code >= 400 || strings.TrimSpace(string(raw)) != "true" {
		return fmt.Errorf("certificate not issued (HTTP %d): %s", code, strings.TrimSpace(string(raw)))
	}
	return nil
}
