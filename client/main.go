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
	"os"
	"strings"
	"time"

	"github.com/hashicorp/yamux"

	"zoraxy-tunnel/wire"
)

const (
	dialTimeout    = 10 * time.Second // bounds both TCP connect and TLS handshake
	initialBackoff = time.Second      // first reconnect delay
	maxBackoff     = 30 * time.Second // ceiling for exponential backoff
)

func main() {
	server := flag.String("server", "", "tunnel server host:port (e.g. tunnel.example.com:9443)")
	token := flag.String("token", "", "tunnel token (from the dashboard)")
	fingerprint := flag.String("fingerprint", "", "expected SHA256 cert fingerprint, e.g. AB:CD:EF:...")
	flag.Parse()

	if *server == "" || *token == "" || *fingerprint == "" {
		fmt.Fprintln(os.Stderr, "usage: tunnel-client --server HOST:PORT --token TOKEN --fingerprint FP")
		flag.Usage()
		os.Exit(2)
	}
	want := normalizeFingerprint(*fingerprint)

	backoff := initialBackoff
	for {
		connected, err := run(*server, *token, want)
		if err != nil {
			log.Printf("[client] %v", err)
		}
		// a real disconnect (we were authenticated) should retry fast; only
		// repeated connect failures deserve a growing delay.
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

// run dials the server, verifies the pinned cert, authenticates and serves
// incoming requests until the connection drops. The connected return tells the
// caller whether we got far enough to reset the reconnect backoff.
func run(server, token, wantFingerprint string) (bool, error) {
	// net.DialTimeout bounds the TCP connect; tls.Dial alone would block ~2 min.
	rawConn, err := net.DialTimeout("tcp", server, dialTimeout)
	if err != nil {
		return false, fmt.Errorf("dial: %w", err)
	}
	conn := tls.Client(rawConn, &tls.Config{
		InsecureSkipVerify:    true, // we pin via fingerprint instead
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
	if err := wire.WriteJSON(auth, wire.AuthReq{Token: token}); err != nil {
		return false, err
	}
	var resp wire.AuthResp
	if err := wire.ReadJSON(auth, &resp); err != nil {
		return false, err
	}
	auth.Close()
	if !resp.OK {
		// wrong token won't fix itself by retrying, but we still back off so a
		// misconfigured client doesn't hammer the server.
		return false, errors.New("auth rejected: " + resp.Error)
	}
	log.Printf("[client] authenticated, tunnel=%s", resp.TunnelID)

	for {
		stream, err := sess.Accept()
		if err != nil {
			return true, fmt.Errorf("session ended: %w", err) // true: we were up, reset backoff
		}
		go handleStream(stream)
	}
}

// verifyFingerprint returns a callback that rejects the peer unless its leaf
// cert's SHA256 matches the pinned value. This is the whole trust anchor —
// there is no CA, the fingerprint IS the identity.
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
