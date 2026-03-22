package tls

import (
	"crypto/tls"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureSelfSignedCert(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	logger := slog.Default()

	// First call should generate certificates.
	if err := EnsureSelfSignedCert(certPath, keyPath, logger); err != nil {
		t.Fatalf("EnsureSelfSignedCert: %v", err)
	}

	// Verify files exist with correct permissions.
	for _, path := range []string{certPath, keyPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("%s has permissions %o, want 0600", path, perm)
		}
	}

	// Second call should be a no-op (files already exist).
	certInfo, _ := os.Stat(certPath)
	modTime := certInfo.ModTime()

	if err := EnsureSelfSignedCert(certPath, keyPath, logger); err != nil {
		t.Fatalf("second EnsureSelfSignedCert: %v", err)
	}

	certInfo2, _ := os.Stat(certPath)
	if certInfo2.ModTime() != modTime {
		t.Error("certificate was regenerated on second call")
	}
}

func TestEnsureSelfSignedCert_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "sub", "deep", "cert.pem")
	keyPath := filepath.Join(dir, "sub", "deep", "key.pem")
	logger := slog.Default()

	if err := EnsureSelfSignedCert(certPath, keyPath, logger); err != nil {
		t.Fatalf("EnsureSelfSignedCert: %v", err)
	}

	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert not created: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("key not created: %v", err)
	}
}

func TestLoadTLSConfig(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	logger := slog.Default()

	if err := EnsureSelfSignedCert(certPath, keyPath, logger); err != nil {
		t.Fatalf("EnsureSelfSignedCert: %v", err)
	}

	tlsCfg, err := LoadTLSConfig(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadTLSConfig: %v", err)
	}

	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want TLS 1.2 (%d)", tlsCfg.MinVersion, tls.VersionTLS12)
	}
	if len(tlsCfg.Certificates) != 1 {
		t.Errorf("got %d certificates, want 1", len(tlsCfg.Certificates))
	}
}

func TestLoadTLSConfig_MissingFiles(t *testing.T) {
	_, err := LoadTLSConfig("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error for missing files")
	}
}

func TestCertFingerprint(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	logger := slog.Default()

	if err := EnsureSelfSignedCert(certPath, keyPath, logger); err != nil {
		t.Fatalf("EnsureSelfSignedCert: %v", err)
	}

	fp, err := CertFingerprint(certPath)
	if err != nil {
		t.Fatalf("CertFingerprint: %v", err)
	}

	// SHA-256 fingerprint should be 64 hex chars + 31 colons = 95 chars.
	if len(fp) != 95 {
		t.Errorf("fingerprint length = %d, want 95 (got %q)", len(fp), fp)
	}

	// Should be stable across calls.
	fp2, _ := CertFingerprint(certPath)
	if fp != fp2 {
		t.Error("fingerprint changed between calls")
	}
}

func TestCertFingerprint_MissingFile(t *testing.T) {
	_, err := CertFingerprint("/nonexistent/cert.pem")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCertExpiry(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	logger := slog.Default()

	if err := EnsureSelfSignedCert(certPath, keyPath, logger); err != nil {
		t.Fatalf("EnsureSelfSignedCert: %v", err)
	}

	expiry, err := CertExpiry(certPath)
	if err != nil {
		t.Fatalf("CertExpiry: %v", err)
	}

	// Should expire approximately 10 years from now.
	expectedExpiry := time.Now().Add(10 * 365 * 24 * time.Hour)
	diff := expiry.Sub(expectedExpiry)
	if diff < -time.Hour || diff > time.Hour {
		t.Errorf("expiry %v is not ~10 years from now (expected ~%v)", expiry, expectedExpiry)
	}
}
