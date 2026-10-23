package backend

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestProxyImportPreferencesRememberAndForget(t *testing.T) {
	root := t.TempDir()
	app := &App{appRoot: root}
	initial, err := app.BrowserProxyGetImportPreferences()
	if err != nil || initial["clashUrl"] != "" {
		t.Fatalf("initial preferences: %v", err)
	}
	for field, value := range map[string]string{"clashUrl": "https://example.test/sub?token=synthetic", "webshareApiKey": "synthetic-key"} {
		if err := app.BrowserProxySaveImportPreference(field, value); err != nil {
			t.Fatal(err)
		}
	}
	restored, err := (&App{appRoot: root}).BrowserProxyGetImportPreferences()
	if err != nil || restored["webshareApiKey"] != "synthetic-key" || restored["clashUrl"] == "" {
		t.Fatal("preferences did not survive new app instance")
	}
	path := filepath.Join(root, "proxy-import.local.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private file permissions: %v", err)
	}
	if err := app.BrowserProxySaveImportPreference("webshareApiKey", ""); err != nil {
		t.Fatal(err)
	}
	restored, _ = app.BrowserProxyGetImportPreferences()
	if restored["webshareApiKey"] != "" || restored["clashUrl"] == "" {
		t.Fatal("forget must preserve other source")
	}
	if err := app.BrowserProxySaveImportPreference("clashUrl", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("empty preferences file should be removed")
	}
}

func TestProxyImportPreferencesRejectInvalidAndCorruptInput(t *testing.T) {
	root := t.TempDir()
	app := &App{appRoot: root}
	if err := app.BrowserProxySaveImportPreference("unknown", "value"); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := app.BrowserProxySaveImportPreference("webshareApiKey", strings.Repeat("x", 17000)); err == nil {
		t.Fatal("oversize value accepted")
	}
	path := filepath.Join(root, "proxy-import.local.json")
	if err := os.WriteFile(path, []byte(`{"webshareApiKey":"synthetic-secret",broken`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := app.BrowserProxyGetImportPreferences()
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal("corruption must fail without exposing file contents")
	}
	if err := app.BrowserProxySaveImportPreference("clashUrl", "https://example.test"); err == nil {
		t.Fatal("must not overwrite unreadable existing preferences")
	}
}

func TestProxyImportPreferencesConcurrentUpdates(t *testing.T) {
	app := &App{appRoot: t.TempDir()}
	var wg sync.WaitGroup
	for field, value := range map[string]string{"clashUrl": "https://example.test", "webshareApiKey": "synthetic-key"} {
		wg.Add(1)
		go func(field, value string) {
			defer wg.Done()
			if err := app.BrowserProxySaveImportPreference(field, value); err != nil {
				t.Error(err)
			}
		}(field, value)
	}
	wg.Wait()
	values, err := app.BrowserProxyGetImportPreferences()
	if err != nil || values["clashUrl"] == "" || values["webshareApiKey"] == "" {
		t.Fatal("concurrent updates lost a source")
	}
}
