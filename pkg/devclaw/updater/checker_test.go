package updater

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCheckerDevBuild(t *testing.T) {
	checker := NewChecker("dev", "http://example.com", time.Hour, nil)
	info, err := checker.CheckNow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Available {
		t.Error("dev build should never report an update")
	}
	if info.CurrentVersion != "dev" {
		t.Errorf("current version = %q, want %q", info.CurrentVersion, "dev")
	}
}

func TestCheckerNoUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v1.0.0\n"))
	}))
	defer srv.Close()

	checker := NewChecker("v1.0.0", srv.URL, time.Hour, nil)
	info, err := checker.CheckNow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Available {
		t.Error("same version should not report update")
	}
	if info.LatestVersion != "v1.0.0" {
		t.Errorf("latest version = %q, want %q", info.LatestVersion, "v1.0.0")
	}
}

func TestCheckerUpdateAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v2.0.0"))
	}))
	defer srv.Close()

	checker := NewChecker("v1.0.0", srv.URL, time.Hour, nil)
	info, err := checker.CheckNow()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.Available {
		t.Error("newer version should report update available")
	}
	if info.LatestVersion != "v2.0.0" {
		t.Errorf("latest version = %q, want %q", info.LatestVersion, "v2.0.0")
	}
}

func TestCheckerHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	checker := NewChecker("v1.0.0", srv.URL, time.Hour, nil)
	_, err := checker.CheckNow()
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func TestCheckerLastCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v1.5.0"))
	}))
	defer srv.Close()

	checker := NewChecker("v1.0.0", srv.URL, time.Hour, nil)

	// Before any check.
	last := checker.LastCheck()
	if last.Available {
		t.Error("should not report available before check")
	}

	// After check.
	checker.CheckNow()
	last = checker.LastCheck()
	if !last.Available {
		t.Error("should report available after check")
	}
	if last.LatestVersion != "v1.5.0" {
		t.Errorf("last check latest = %q, want %q", last.LatestVersion, "v1.5.0")
	}
}
