package browser

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestCloakManifestVerification(t *testing.T) {
	key, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("synthetic browser archive")
	version, asset := "145.0.7632.109.2", "cloakbrowser-darwin-arm64.tar.gz"
	manifest := []byte(fmt.Sprintf("version=%s\n%x  %s\n", version, sha256.Sum256(data), asset))
	signature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(private, manifest)))
	for _, tc := range []struct {
		name                         string
		manifest, signature, archive []byte
		version, asset               string
		valid                        bool
	}{
		{"valid", manifest, signature, data, version, asset, true},
		{"tampered manifest", append(append([]byte{}, manifest...), 'x'), signature, data, version, asset, false},
		{"tampered archive", manifest, signature, []byte("corrupted"), version, asset, false},
		{"wrong version", manifest, signature, data, "146.0.0.1", asset, false},
		{"wrong asset", manifest, signature, data, version, "cloakbrowser-linux-x64.tar.gz", false},
		{"malformed signature", manifest, []byte("bad signature"), data, version, asset, false},
		{"missing signature", manifest, nil, data, version, asset, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyCloakManifest(bytes.NewReader(tc.archive), tc.manifest, tc.signature, tc.version, tc.asset, key)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
}

type cloakIntegrityTransport func(*http.Request) (*http.Response, error)

func (f cloakIntegrityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCloakOfficialDownloadsFailClosed(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(archive, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: cloakIntegrityTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	for _, raw := range []string{
		"https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v145.0.7632.109.2/cloakbrowser-darwin-arm64.tar.gz",
		"http://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v145.0.7632.109.2/cloakbrowser-darwin-arm64.tar.gz",
		"https://github.com/CloakHQ/cloakbrowser/releases/download/bad/cloakbrowser-darwin-arm64.tar.gz",
	} {
		if err := verifyCloakBrowserArchive(context.Background(), client, raw, archive); err == nil {
			t.Fatalf("unverified official URL accepted: %s", raw)
		}
	}
	// Other user-managed sources retain the existing download contract.
	if err := verifyCloakBrowserArchive(context.Background(), client, "https://example.com/custom.zip", archive); err != nil {
		t.Fatal(err)
	}
}
