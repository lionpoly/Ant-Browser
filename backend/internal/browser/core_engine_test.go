package browser

import (
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"os"
	"path/filepath"
	"testing"
)

func TestCoreFingerprintEngineRoundTrip(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
	dao := NewSQLiteCoreDAO(db.GetConn())
	core := Core{CoreId: "test", CoreName: "custom", CorePath: "/browser", FingerprintEngine: "cloakbrowser"}
	if err = dao.Upsert(core); err != nil {
		t.Fatal(err)
	}
	list, err := dao.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v %v", list, err)
	}
	if ResolveFingerprintEngine(list[0]) != "cloakbrowser" {
		t.Fatal("engine lost")
	}
	if err = db.Migrate(); err != nil {
		t.Fatal(err)
	}
}
func TestResolveFingerprintEngine(t *testing.T) {
	for _, tc := range []struct {
		core Core
		want string
	}{
		{Core{CoreName: "cloakbrowser-145"}, "cloakbrowser"},
		{Core{CorePath: "/chrome/cloakbrowser/Chromium.app"}, "cloakbrowser"},
		{Core{CoreName: "cloakbrowser", FingerprintEngine: "fingerprint-chromium"}, "fingerprint-chromium"},
		{Core{CoreName: "Chromium"}, "fingerprint-chromium"},
	} {
		if got := ResolveFingerprintEngine(tc.core); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
}

func TestCloakDownloadedFullVersionMetadata(t *testing.T) {
	root := t.TempDir()
	url := "https://github.com/CloakHQ/cloakbrowser/releases/download/chromium-v155.0.8059.39.1/cloakbrowser-linux-x64.tar.gz"
	if err := writeCloakDownloadedVersion(root, url); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	m := NewManager(cfg, root)
	if got := m.GetChromeVersion(root); got != "155.0.8059.39.1" {
		t.Fatalf("full release patch missing: %q", got)
	}
	other := t.TempDir()
	if err := writeCloakDownloadedVersion(other, "https://example.invalid/download.zip"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other, ".ant-core-version.json")); !os.IsNotExist(err) {
		t.Fatal("unrecognized source wrote version metadata")
	}
}
