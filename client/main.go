package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/yamux"

	"zoraxy-tunnel/wire"
)

const (
	clientVersion      = "v1.7.0"
	defaultControlPort = "9443"
	dialTimeout        = 10 * time.Second
	initialBackoff     = time.Second
	maxBackoff         = 30 * time.Second
)

func main() {
	server := flag.String("server", "", "tunnel server hostname or host:port (port 9443 is used when omitted)")
	token := flag.String("token", "", "tunnel token (from the dashboard)")
	fingerprint := flag.String("fingerprint", "", "expected SHA256 cert fingerprint, e.g. AB:CD:EF:...")
	showVersion := flag.Bool("version", false, "print tunnel client version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(clientVersion)
		return
	}
	if *server == "" || *token == "" || *fingerprint == "" {
		fmt.Fprintln(os.Stderr, "usage: tunnel-client --server HOST[:PORT] --token TOKEN --fingerprint FP")
		flag.Usage()
		os.Exit(2)
	}
	normalizedServer, err := normalizeServerAddress(*server)
	if err != nil {
		log.Fatalf("[client] invalid server address: %v", err)
	}
	want := normalizeFingerprint(*fingerprint)

	backoff := initialBackoff
	for {
		connected, err := run(normalizedServer, *token, want)
		if err != nil {
			log.Printf("[client] %v", err)
		}
		if connected {
			backoff = initialBackoff
		}
		log.Printf("[client] reconnecting in %s…", backoff)
		time.Sleep(backoff)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func normalizeServerAddress(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("server address is empty")
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return "", errors.New("invalid URL")
		}
		if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return "", errors.New("server address must not include a path, query or fragment")
		}
		s = u.Host
	}
	if strings.ContainsAny(s, "/?# ") {
		return "", errors.New("server address must be a hostname or IP address")
	}
	if host, port, err := net.SplitHostPort(s); err == nil {
		if host == "" || port == "" {
			return "", errors.New("hostname and port are required")
		}
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 {
			return "", errors.New("port must be between 1 and 65535")
		}
		return net.JoinHostPort(host, port), nil
	}
	plain := strings.Trim(s, "[]")
	if ip := net.ParseIP(plain); ip != nil {
		return net.JoinHostPort(plain, defaultControlPort), nil
	}
	if strings.Count(s, ":") == 0 {
		return net.JoinHostPort(s, defaultControlPort), nil
	}
	if strings.Count(s, ":") == 1 {
		host, port, _ := strings.Cut(s, ":")
		p, err := strconv.Atoi(port)
		if host == "" || err != nil || p < 1 || p > 65535 {
			return "", errors.New("invalid hostname or port")
		}
		return net.JoinHostPort(host, port), nil
	}
	return "", errors.New("invalid server address")
}

func run(server, token, wantFingerprint string) (bool, error) {
	rawConn, err := net.DialTimeout("tcp", server, dialTimeout)
	if err != nil {
		return false, fmt.Errorf("dial: %w", err)
	}
	conn := tls.Client(rawConn, &tls.Config{
		InsecureSkipVerify:    true,
		VerifyPeerCertificate: verifyFingerprint(wantFingerprint),
		MinVersion:            tls.VersionTLS12,
	})
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	if err := conn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return false, fmt.Errorf("tls handshake: %w", err)
	}
	defer conn.Close()
	log.Printf("[client] tls connected to %s", server)

	sess, err := yamux.Client(conn, yamux.DefaultConfig())
	if err != nil {
		return false, fmt.Errorf("yamux: %w", err)
	}
	defer sess.Close()

	auth, err := sess.Open()
	if err != nil {
		return false, err
	}
	hostname, _ := os.Hostname()
	if err := wire.WriteJSON(auth, wire.AuthReq{Token: token, Version: clientVersion, Hostname: hostname, OS: runtime.GOOS, Arch: runtime.GOARCH}); err != nil {
		return false, err
	}
	var resp wire.AuthResp
	if err := wire.ReadJSON(auth, &resp); err != nil {
		return false, err
	}
	auth.Close()
	if !resp.OK {
		return false, errors.New("auth rejected: " + resp.Error)
	}
	log.Printf("[client] authenticated, tunnel=%s version=%s", resp.TunnelID, clientVersion)

	for {
		stream, err := sess.Accept()
		if err != nil {
			return true, fmt.Errorf("session ended: %w", err)
		}
		go handleStream(stream)
	}
}

func verifyFingerprint(want string) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return errors.New("no peer certificate")
		}
		sum := sha256.Sum256(rawCerts[0])
		got := strings.ToUpper(hex.EncodeToString(sum[:]))
		if got != want {
			return fmt.Errorf("fingerprint mismatch (got %s)", formatColon(got))
		}
		return nil
	}
}

func normalizeFingerprint(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, " ", "")
	return strings.ToUpper(s)
}

func formatColon(hexStr string) string {
	var b strings.Builder
	for i := 0; i < len(hexStr); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(hexStr[i : i+2])
	}
	return b.String()
}
