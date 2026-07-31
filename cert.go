package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const certValidity = 99 * 365 * 24 * time.Hour

// certManager owns the plugin's self-signed TLS cert used on the control
// port. Generated once, reused forever (or until the user deletes the file).
type certManager struct {
	certPath string
	keyPath  string
	cert     *tls.Certificate
	fingerprint string
}

func newCertManager(dir string) *certManager {
	return &certManager{
		certPath: filepath.Join(dir, "cert.pem"),
		keyPath:  filepath.Join(dir, "key.pem"),
	}
}

// LoadOrCreate loads the cert or mints a fresh 99-year one on first run.
func (c *certManager) LoadOrCreate() error {
	if err := c.load(); err == nil {
		return nil
	}
	return c.create()
}

func (c *certManager) load() error {
	certPEM, err := os.ReadFile(c.certPath)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(c.keyPath)
	if err != nil {
		return err
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return err
	}
	if len(tlsCert.Certificate) == 0 {
		return fmt.Errorf("no leaf cert")
	}
	c.cert = &tlsCert
	c.fingerprint = sha256Fingerprint(tlsCert.Certificate[0])
	return nil
}

func (c *certManager) create() error {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "zoraxy-tunnel"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(certValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		// no DNS names: client pins via fingerprint, so the cert is intentionally
		// not bound to any domain
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(c.certPath, certPEM, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(c.keyPath, keyPEM, 0600); err != nil {
		return err
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return err
	}
	c.cert = &tlsCert
	c.fingerprint = sha256Fingerprint(der)
	return nil
}

func (c *certManager) TLSConfig() *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{*c.cert}, MinVersion: tls.VersionTLS12}
}

func (c *certManager) Fingerprint() string { return c.fingerprint }

// sha256Fingerprint returns the cert's SHA256 over the DER, formatted the way
// people expect to copy-paste: AB:CD:EF:...
func sha256Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	hexed := hex.EncodeToString(sum[:])
	var parts []string
	for i := 0; i < len(hexed); i += 2 {
		parts = append(parts, strings.ToUpper(hexed[i:i+2]))
	}
	return strings.Join(parts, ":")
}
