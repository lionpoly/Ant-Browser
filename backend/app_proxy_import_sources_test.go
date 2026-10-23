package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProxyImportSourcesMigrateAndManageMultiple(t *testing.T) {
	root := t.TempDir()
	app := &App{appRoot: root}
	if err := os.WriteFile(filepath.Join(root, "proxy-import.local.json"), []byte(`{"clashUrl":"https://old.test/?token=synthetic","webshareApiKey":"old-synthetic-key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := app.BrowserProxyGetImportSources()
	if err != nil || len(state.Sources) != 2 {
		t.Fatalf("legacy migration failed: %v", err)
	}
	clash, err := app.BrowserProxySaveImportSource("", "clash", "订阅二", "https://new.test/?token=synthetic")
	if err != nil {
		t.Fatal(err)
	}
	key, err := app.BrowserProxySaveImportSource("", "webshare", "账号二", "synthetic-key-two")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := app.BrowserProxySaveImportSource("", "webshare", "账号二改名", "synthetic-key-two")
	if err != nil || duplicate.ID != key.ID {
		t.Fatal("same source should be reused")
	}
	if _, err := app.BrowserProxySaveImportSource(clash.ID, "clash", "新名称", "https://updated.test"); err != nil {
		t.Fatal(err)
	}
	state, err = (&App{appRoot: root}).BrowserProxyGetImportSources()
	if err != nil || len(state.Sources) != 4 || state.SelectedWebshareID != key.ID {
		t.Fatal("multiple sources must persist")
	}
	if err := app.BrowserProxyDeleteImportSource(clash.ID); err != nil {
		t.Fatal(err)
	}
	state, _ = app.BrowserProxyGetImportSources()
	if len(state.Sources) != 3 || state.SelectedWebshareID != key.ID {
		t.Fatal("delete erased other sources")
	}
	for _, source := range state.Sources {
		if source.ID == clash.ID {
			t.Fatal("deleted source remains")
		}
	}
}

func TestProxyImportSourcesRejectInvalidEdits(t *testing.T) {
	app := &App{appRoot: t.TempDir()}
	if _, err := app.BrowserProxySaveImportSource("", "unknown", "name", "value"); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if _, err := app.BrowserProxySaveImportSource("", "clash", "name", " "); err == nil {
		t.Fatal("blank source accepted")
	}
	if _, err := app.BrowserProxySaveImportSource("missing", "webshare", "name", "synthetic"); err == nil {
		t.Fatal("stale ID accepted")
	}
	source, err := app.BrowserProxySaveImportSource("", "clash", "name", "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.BrowserProxySaveImportSource(source.ID, "webshare", "name", "synthetic"); err == nil {
		t.Fatal("kind mutation accepted")
	}
}
