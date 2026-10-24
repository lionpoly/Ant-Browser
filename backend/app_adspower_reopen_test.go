package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func adsPowerRetentionTestApp(t *testing.T) (*App, *BrowserProfile) {
	t.Helper()
	cfg := config.DefaultConfig()
	a := NewApp(t.TempDir())
	a.config = cfg
	a.browserMgr = browser.NewManager(cfg, a.appRoot)
	profile := &BrowserProfile{ProfileId: "synthetic-retention", FingerprintArgs: []string{}, LaunchArgs: []string{}}
	a.browserMgr.Profiles = map[string]*browser.Profile{profile.ProfileId: profile}
	return a, profile
}

func TestAdsPowerImportEnablesSessionRetention(t *testing.T) {
	contents, err := buildAdsPowerContents([]map[string]string{{"id": "synthetic-reopen", "cookie": "[]", "tab": "https://example.invalid/start"}})
	if err != nil {
		t.Fatal(err)
	}
	defer contents.Close()
	profile := contents.Profiles[0]
	if !profileRestoreLastSession(&profile, config.DefaultConfig()) {
		t.Fatal("imported profile follows global off: session cookies and imported tabs will be discarded on reopen")
	}
}

// Synthetic localhost only. Never read an actual profile or contact imported sites.
func TestAdsPowerRealBrowserReopenRetainsCurrentSession(t *testing.T) {
	binary := os.Getenv("ANT_TEST_BROWSER_BINARY")
	if binary == "" {
		t.Skip("optional local browser reopen regression")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html><title>synthetic-reopen</title></html>")
	}))
	defer server.Close()
	cookieRows := []map[string]any{
		{"name": "keep", "value": "initial", "domain": "127.0.0.1", "path": "/", "session": true},
		{"name": "rotate", "value": "original", "domain": "127.0.0.1", "path": "/", "session": true},
		{"name": "deleted", "value": "original", "domain": "127.0.0.1", "path": "/", "session": true},
		{"name": "persistent", "value": "control", "domain": "127.0.0.1", "path": "/", "expires": time.Now().Add(24 * time.Hour).Unix()},
	}
	cookieJSON, _ := json.Marshal(cookieRows)
	contents, err := buildAdsPowerContents([]map[string]string{{"id": "synthetic-reopen", "cookie": string(cookieJSON), "tab": server.URL + "/imported"}})
	if err != nil {
		t.Fatal(err)
	}
	defer contents.Close()
	profile := contents.Profiles[0]
	dir := t.TempDir()
	// Extract just this synthetic profile's pending import payload.
	for _, file := range contents.Reader.File {
		if !strings.HasSuffix(file.Name, adsPowerPendingPath) {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		var pending adsPowerPending
		err = json.NewDecoder(reader).Decode(&pending)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := saveAdsPowerPending(filepath.Join(dir, adsPowerPendingPath), pending); err != nil {
			t.Fatal(err)
		}
	}
	launch := func() (int, func()) {
		t.Helper()
		restore := profileRestoreLastSession(&profile, config.DefaultConfig())
		if !restore {
			if err := browser.ClearSessionRestoreData(dir); err != nil {
				t.Fatal(err)
			}
		}
		port, err := nextAvailablePort()
		if err != nil {
			t.Fatal(err)
		}
		flags := []string{"--headless=new", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--disable-component-update", "--disable-sync", "--password-store=basic", "--use-mock-keychain", "--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE localhost"}
		targets := []string{"about:blank"}
		if restore {
			targets = nil
		}
		// Match production: initial import must not restore prior tabs before
		// imported cookies are ready. Reopens use the enabled profile setting.
		if _, err := os.Stat(filepath.Join(dir, adsPowerPendingPath)); err == nil {
			restore = false
			targets = []string{"about:blank"}
		}
		args := buildBrowserLaunchArgs(dir, port, "direct://", nil, flags, nil, targets, restore)
		cmd := exec.Command(binary, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal("local synthetic browser start failed")
		}
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		var once sync.Once
		stop := func() {
			once.Do(func() {
				_ = cdpBrowserCall(port, "Browser.close", nil)
				select {
				case <-exited:
				case <-time.After(10 * time.Second):
					cmd.Process.Kill()
					<-exited
					t.Error("browser did not close cleanly")
				}
			})
		}
		t.Cleanup(stop)
		deadline := time.Now().Add(20 * time.Second)
		for !browserDebugPortReady(port) {
			if time.Now().After(deadline) {
				stop()
				t.Fatalf("local browser debug port unavailable: %s", stderr.String())
			}
			time.Sleep(50 * time.Millisecond)
		}
		return port, stop
	}
	port, stop := launch()
	if err := restoreAdsPowerSession(dir, port); err != nil {
		t.Fatal(err)
	}
	assertCookies := func(port int, rotated string, deleted bool) {
		t.Helper()
		result, err := cdpBrowserCallResult(port, "Storage.getCookies", nil)
		if err != nil {
			t.Fatal(err)
		}
		values := map[string]string{}
		for _, entry := range result["cookies"].([]any) {
			c := entry.(map[string]any)
			values[c["name"].(string)] = c["value"].(string)
		}
		if values["keep"] != "initial" || values["rotate"] != rotated || values["persistent"] != "control" {
			t.Errorf("synthetic current cookie state lost after reopen: %v", values)
		}
		if (values["deleted"] != "") == deleted {
			t.Error("deleted cookie was replayed or disappeared before deletion")
		}
	}
	assertCookies(port, "original", false)
	if err := cdpBrowserCall(port, "Storage.setCookies", map[string]any{"cookies": []map[string]any{{"name": "rotate", "value": "updated-by-site", "domain": "127.0.0.1", "path": "/"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := cdpCall(port, "Network.deleteCookies", map[string]any{"name": "deleted", "domain": "127.0.0.1", "path": "/"}); err != nil {
		t.Fatal(err)
	}
	// Ensure the imported page is registered before the clean shutdown snapshot.
	deadline := time.Now().Add(10 * time.Second)
	hasPage := func(port int) bool {
		r, e := cdpBrowserCallResult(port, "Target.getTargets", nil)
		if e != nil {
			return false
		}
		entries, _ := r["targetInfos"].([]any)
		for _, entry := range entries {
			v := entry.(map[string]any)
			if v["url"] == server.URL+"/imported" {
				return true
			}
		}
		return false
	}
	for !hasPage(port) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !hasPage(port) {
		t.Fatal("first import page absent")
	}
	stop()
	port, stop = launch()
	defer stop()
	if err := restoreAdsPowerSession(dir, port); err != nil {
		t.Fatal(err)
	} // consumed; must not replay original cookies
	assertCookies(port, "updated-by-site", true)
	deadline = time.Now().Add(10 * time.Second)
	for !hasPage(port) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !hasPage(port) {
		t.Error("imported page lost after reopening")
	}
}

func TestAdsPowerExistingImportSessionRetention(t *testing.T) {
	for _, mode := range []string{"", browser.RestoreLastSessionEnabled, browser.RestoreLastSessionDisabled} {
		t.Run("mode_"+mode, func(t *testing.T) {
			a, p := adsPowerRetentionTestApp(t)
			p.RestoreLastSession = mode
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, adsPowerMetadataPath), []byte(`{"id":"synthetic-only"}`), 0600); err != nil {
				t.Fatal(err)
			}
			session := filepath.Join(dir, "Default", "Sessions", "Session_synthetic")
			if err := os.MkdirAll(filepath.Dir(session), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(session, []byte("synthetic session"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := a.ensureAdsPowerSessionRetention(p, dir); err != nil {
				t.Fatal(err)
			}
			expected := mode
			if expected == "" {
				expected = browser.RestoreLastSessionEnabled
			}
			if p.RestoreLastSession != expected {
				t.Fatal("explicit mode overwritten or inherited import not upgraded")
			}
			if _, err := os.Stat(session); err != nil {
				t.Fatal("session removed during compatibility preparation")
			}
			if mode == "" {
				cfg, err := config.Load(filepath.Join(a.appRoot, "config.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				if cfg.Browser.RestoreLastSession || cfg.Browser.Profiles[0].RestoreLastSession != browser.RestoreLastSessionEnabled {
					t.Fatal("upgrade not persisted per profile")
				}
			}
		})
	}
}

func TestAdsPowerSessionRetentionSaveFailureRollsBack(t *testing.T) {
	a, p := adsPowerRetentionTestApp(t)
	dir := t.TempDir()
	if err := a.ensureAdsPowerSessionRetention(p, dir); err != nil || p.RestoreLastSession != "" {
		t.Fatal("ordinary profile changed", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, adsPowerMetadataPath), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(a.appRoot, "config.yaml"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.ensureAdsPowerSessionRetention(p, dir); err == nil || p.RestoreLastSession != "" {
		t.Fatal("failed storage must block and roll back")
	}
}

func TestAdsPowerUnreadableRetentionMetadataBlocksUpgrade(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix permission enforcement")
	}
	a, p := adsPowerRetentionTestApp(t)
	dir := t.TempDir()
	markerDir := filepath.Join(dir, ".ant-import")
	if err := os.Mkdir(markerDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, adsPowerMetadataPath), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(markerDir, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(markerDir, 0700)
	if err := a.ensureAdsPowerSessionRetention(p, dir); err == nil || p.RestoreLastSession != "" {
		t.Fatal("unreadable import metadata must block unsafe session cleanup")
	}
}

func TestAdsPowerAttachedImportRetention(t *testing.T) {
	path := os.Getenv("ANT_ADSPOWER_SAMPLE_FILE")
	if path == "" {
		t.Skip("local private attachment validation only")
	}
	contents, err := openAdsPowerContents(path)
	if err != nil {
		t.Fatal("private attachment failed to parse")
	}
	defer contents.Close()
	if len(contents.Profiles) == 0 {
		t.Fatal("private attachment has no profiles")
	}
	for i := range contents.Profiles {
		if !profileRestoreLastSession(&contents.Profiles[i], config.DefaultConfig()) {
			t.Fatal("private attachment instance does not preserve session")
		}
	}
	t.Logf("Validated %d imported profiles without exposing fields", len(contents.Profiles))
}

func TestAdsPowerStartupUpgradesBeforeSessionCleanup(t *testing.T) {
	for _, saveFails := range []bool{false, true} {
		t.Run(fmt.Sprint(saveFails), func(t *testing.T) {
			a, p := adsPowerRetentionTestApp(t)
			coreDir := t.TempDir()
			exe := filepath.Join(coreDir, browser.CoreExecutableCandidates()[0])
			if err := os.MkdirAll(filepath.Dir(exe), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(exe, []byte("stub"), 0700); err != nil {
				t.Fatal(err)
			}
			a.config.Browser.Cores = []config.BrowserCore{{CoreId: "synthetic-core", CorePath: coreDir, IsDefault: true}}
			dir := a.browserMgr.ResolveUserDataDir(p)
			if err := os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, adsPowerMetadataPath), []byte(`{}`), 0600); err != nil {
				t.Fatal(err)
			}
			session := filepath.Join(dir, "Default", "Sessions", "Session_synthetic")
			if err := os.MkdirAll(filepath.Dir(session), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(session, []byte("synthetic state"), 0600); err != nil {
				t.Fatal(err)
			}
			if saveFails {
				if err := os.Mkdir(filepath.Join(a.appRoot, "config.yaml"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			plan, err := a.prepareBrowserStartPlan(browserStartInput{ProfileID: p.ProfileId, SkipDefaultStartURLs: true}, p)
			if saveFails {
				if err == nil || plan != nil {
					t.Fatal("failed compatibility persistence must block launch")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				defer plan.releaseBridgeIfNeeded(a)
				found := false
				for _, arg := range plan.args {
					if arg == "--restore-last-session" {
						found = true
					}
				}
				if !found {
					t.Fatal("upgraded imported profile did not request native session restore")
				}
			}
			data, err := os.ReadFile(session)
			if err != nil || string(data) != "synthetic state" {
				t.Fatal("startup removed session before preserving imported data")
			}
		})
	}
}

func TestAdsPowerRetentionSQLiteRoundTripAndRollback(t *testing.T) {
	a, p := adsPowerRetentionTestApp(t)
	db, err := database.NewDB(filepath.Join(t.TempDir(), "retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	a.browserMgr.ProfileDAO = browser.NewSQLiteProfileDAO(db.GetConn())
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, adsPowerMetadataPath), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.ensureAdsPowerSessionRetention(p, dir); err != nil {
		t.Fatal(err)
	}
	stored, err := a.browserMgr.ProfileDAO.GetById(p.ProfileId)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RestoreLastSession != browser.RestoreLastSessionEnabled {
		t.Fatal("database did not retain instance override")
	}
	p.RestoreLastSession = browser.RestoreLastSessionFollow
	db.Close()
	if err := a.ensureAdsPowerSessionRetention(p, dir); err == nil || p.RestoreLastSession != browser.RestoreLastSessionFollow {
		t.Fatal("database failure did not block/roll back upgrade")
	}
}
