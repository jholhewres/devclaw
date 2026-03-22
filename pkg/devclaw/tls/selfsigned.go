// Package tls provides self-signed TLS certificate generation and loading
// for DevClaw's HTTPS support. Uses Go's crypto/x509 stdlib (no OpenSSL dependency).
package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// EnsureSelfSignedCert checks if cert+key exist at the given paths.
// If not, generates new self-signed certificates using ECDSA P-256.
// Certificate validity: 10 years. File permissions: 0600.
func EnsureSelfSignedCert(certPath, keyPath string, logger *slog.Logger) error {
	// Check if both files already exist.
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	if certErr == nil && keyErr == nil {
		logger.Info("TLS certificates already exist", "cert", certPath, "key", keyPath)
		return nil
	}

	// If only one file exists, remove the orphan to avoid cert/key mismatch.
	if (certErr == nil) != (keyErr == nil) {
		logger.Warn("TLS certificate files are inconsistent, regenerating both",
			"cert_exists", certErr == nil,
			"key_exists", keyErr == nil,
		)
		os.Remove(certPath)
		os.Remove(keyPath)
	}

	logger.Info("generating self-signed TLS certificate", "cert", certPath, "key", keyPath)

	// Ensure parent directories exist.
	if err := os.MkdirAll(filepath.Dir(certPath), 0700); err != nil {
		return fmt.Errorf("creating cert directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return fmt.Errorf("creating key directory: %w", err)
	}

	// Generate ECDSA P-256 private key.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generating ECDSA key: %w", err)
	}

	// Create certificate template.
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generating serial number: %w", err)
	}

	now := time.Now()
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "devclaw",
			Organization: []string{"DevClaw Self-Signed"},
		},
		NotBefore:             now,
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour), // ~10 years
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost", "devclaw"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	// Self-sign the certificate.
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("creating certificate: %w", err)
	}

	// Write certificate PEM.
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certPath, certPEM, 0600); err != nil {
		return fmt.Errorf("writing certificate: %w", err)
	}

	// Write private key PEM.
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("marshaling private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("writing private key: %w", err)
	}

	logger.Info("self-signed TLS certificate generated",
		"cn", "devclaw",
		"validity", "10 years",
		"algorithm", "ECDSA P-256",
	)

	return nil
}

// LoadTLSConfig reads the certificate and key files and returns a *tls.Config
// ready for use with http.Server. MinVersion is set to TLS 1.2.
func LoadTLSConfig(certPath, keyPath string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("loading TLS key pair: %w", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// CertFingerprint returns the SHA-256 fingerprint of the certificate at certPath
// as a lowercase hex string with colon separators (e.g. "ab:cd:ef:...").
func CertFingerprint(certPath string) (string, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return "", fmt.Errorf("reading certificate: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return "", fmt.Errorf("no PEM block found in %s", certPath)
	}

	hash := sha256.Sum256(block.Bytes)
	hexStr := hex.EncodeToString(hash[:])

	// Format with colons every 2 chars.
	var formatted []byte
	for i, c := range hexStr {
		if i > 0 && i%2 == 0 {
			formatted = append(formatted, ':')
		}
		formatted = append(formatted, byte(c))
	}
	return string(formatted), nil
}

// CertExpiry returns the NotAfter time of the certificate at certPath.
func CertExpiry(certPath string) (time.Time, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("reading certificate: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return time.Time{}, fmt.Errorf("no PEM block found in %s", certPath)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing certificate: %w", err)
	}

	return cert.NotAfter, nil
}
