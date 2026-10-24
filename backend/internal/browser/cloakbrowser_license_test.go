package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveCloakBrowserLicense(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".cloakbrowser", "license.key")
	for _, key := range []string{" cb_test_synthetic_first ", "cb_test_synthetic_second"} {
		if err := saveCloakBrowserLicense(path, key); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || strings.TrimSpace(string(data)) != strings.TrimSpace(key) {
			t.Fatal("saved license does not match supplied fixture")
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatalf("license permissions = %o, want 0600", info.Mode().Perm())
		}
	}
	if err := saveCloakBrowserLicense(path, "  "); err == nil {
		t.Fatal("empty key accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) != "cb_test_synthetic_second" {
		t.Fatal("invalid input replaced the existing license")
	}
}

func TestCloakBrowserKeyedDownloadURL(t *testing.T) {
	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"https://cloakbrowser.dev/api/download/155.0.8059.39.1", true},
		{"https://example.com/api/download/155.0.8059.39.1", false},
		{"http://cloakbrowser.dev/api/download/155.0.8059.39.1", false},
		{"https://cloakbrowser.dev:8443/api/download/155.0.8059.39.1", false},
		{"https://user@cloakbrowser.dev/api/download/155.0.8059.39.1", false},
		{"https://cloakbrowser.dev/other", false},
	} {
		if IsCloakBrowserKeyedDownloadURL(tc.url) != tc.valid {
			t.Fatalf("unexpected URL decision: %s", tc.url)
		}
	}
}
