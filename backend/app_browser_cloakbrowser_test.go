package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCloakBrowserLaunchPlan(t *testing.T) {
	plan := buildCloakBrowserFingerprintLaunchPlan("profile-cloak", []string{
		"--lang=ja-JP", "--timezone=Asia/Tokyo", "--fingerprint-device-memory=8", "--fingerprint-screen-width=1440", "--fingerprint-gpu-vendor=Intel",
		"--fingerprinting-canvas-image-data-noise", "--disable-spoofing=font", "--fingerprint-noise=false",
	}, "145.0.7632.109")
	for _, arg := range []string{"--fingerprint-locale=ja-JP", "--fingerprint-timezone=Asia/Tokyo", "--fingerprint-device-memory=8", "--fingerprint-screen-width=1440", "--fingerprint-gpu-vendor=Intel", "--fingerprint-noise=false"} {
		assertArgsContain(t, plan.launchArgs, arg)
	}
	for _, key := range []string{"--timezone", "--disable-spoofing", "--fingerprinting-canvas-image-data-noise"} {
		assertArgsDoNotContainKey(t, plan.launchArgs, key)
	}
	if !browserArgsHaveFingerprintSeed(plan.launchArgs) {
		t.Fatal("stable seed missing")
	}
}
func TestCloakBrowserVersionGates(t *testing.T) {
	args := []string{"--fingerprint-portable-cookies", "--fingerprint-windows-font-metrics", "--fingerprint-transparent-proxy-dns=local", "--fingerprint=off"}
	old := buildCloakBrowserFingerprintLaunchPlan("p", args, "145.0.7632.109")
	for _, arg := range args {
		if arg == "--fingerprint=off" {
			if browserArgValue(old.launchArgs, "--fingerprint") == "off" {
				t.Fatal("unsupported off retained")
			}
		} else {
			assertArgsDoNotContainKey(t, old.launchArgs, browserFingerprintArgKey(arg))
		}
	}
	modern := buildCloakBrowserFingerprintLaunchPlanForHost("p", args, "155.0.8059.39.1", "linux")
	for _, arg := range args {
		assertArgsContain(t, modern.launchArgs, arg)
	}
}
func TestCloakBrowserNativeLocaleWins(t *testing.T) {
	plan := buildCloakBrowserFingerprintLaunchPlan("p", []string{"--fingerprint-locale=en-US", "--lang=ja-JP", "--fingerprint-timezone=Europe/London", "--timezone=Asia/Tokyo"}, "146.0.1.1")
	assertArgsContain(t, plan.launchArgs, "--fingerprint-locale=en-US")
	assertArgsContain(t, plan.launchArgs, "--lang=en-US")
	assertArgsContain(t, plan.launchArgs, "--accept-lang=en-US,en")
	assertArgsDoNotContainKey(t, plan.launchArgs, "--timezone")
}

func TestCloakBrowserDNSPlatformAndPatchGate(t *testing.T) {
	flag := []string{"--fingerprint-transparent-proxy-dns=local"}
	for _, tc := range []struct {
		version, host string
		allowed       bool
	}{{"155.0.8059.39.1", "linux", true}, {"155.0.8059.39.0", "linux", false}, {"155.0.8059.39.1", "darwin", false}, {"156.0.0.0", "windows", true}} {
		p := buildCloakBrowserFingerprintLaunchPlanForHost("p", flag, tc.version, tc.host)
		present := browserArgWithKey(p.launchArgs, "--fingerprint-transparent-proxy-dns") != ""
		if present != tc.allowed {
			t.Fatalf("gate %s/%s: %v", tc.version, tc.host, present)
		}
	}
}

func TestCloakBrowserExpectedHardwareUsesEngine(t *testing.T) {
	args := []string{"--fingerprint-device-memory=8", "--fingerprint-locale=en-US"}
	if buildBrowserFingerprintExpected(args).DeviceMemory != "" {
		t.Fatal("legacy engine expectation changed")
	}
	if buildBrowserFingerprintExpectedForEngine(args, "cloakbrowser").DeviceMemory != "8" {
		t.Fatal("native memory expectation lost")
	}
}

func TestCloakBrowserRejectsManagedNetworkOverrides(t *testing.T) {
	p := buildCloakBrowserFingerprintLaunchPlan("p", []string{"--fingerprint-test-future=keep", "--no-proxy-server", "--proxy-bypass-list=*", "--proxy-server=direct://", "--remote-debugging-address=0.0.0.0", "--host-resolver-rules=MAP * 127.0.0.1"}, "151.0.1.1")
	assertArgsContain(t, p.launchArgs, "--fingerprint-test-future=keep")
	for _, key := range []string{"--no-proxy-server", "--proxy-bypass-list", "--proxy-server", "--remote-debugging-address", "--host-resolver-rules"} {
		assertArgsDoNotContainKey(t, p.launchArgs, key)
	}
}

func TestCloakBrowserActualLaunchContextUsesDialect(t *testing.T) {
	root := t.TempDir()
	coreDir := filepath.Join(root, "custom-core")
	exe := filepath.Join(coreDir, filepath.FromSlash(browser.CoreExecutableCandidates()[0]))
	os.MkdirAll(filepath.Dir(exe), 0755)
	os.WriteFile(exe, []byte("stub"), 0755)
	os.WriteFile(filepath.Join(coreDir, "manifest.json"), []byte(`{"version":"145.0.7632.109"}`), 0644)
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = filepath.Join(root, "profiles")
	cfg.Browser.Cores = []browser.Core{{CoreId: "cloak", CoreName: "custom", CorePath: coreDir, IsDefault: true, FingerprintEngine: "cloakbrowser"}}
	app := NewApp(root)
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, root)
	profile := &browser.Profile{ProfileId: "synthetic", CoreId: "cloak", FingerprintArgs: []string{"--lang=ja-JP", "--timezone=Asia/Tokyo", "--fingerprint-device-memory=8", "--fingerprint-gpu-vendor=Intel", "--fingerprint-screen-width=1440", "--fingerprint-portable-cookies"}}
	profileArgs, extra, fp, _, dir, err := app.prepareBrowserLaunchContext(browserStartInput{ProfileID: profile.ProfileId}, profile, nil)
	if err != nil {
		t.Fatal(err)
	}
	argv := buildBrowserLaunchArgs(dir, 12345, "http://127.0.0.1:12346", fp, profileArgs, extra, nil, false)
	for _, arg := range []string{"--fingerprint-locale=ja-JP", "--fingerprint-timezone=Asia/Tokyo", "--fingerprint-device-memory=8", "--fingerprint-gpu-vendor=Intel", "--fingerprint-screen-width=1440"} {
		assertArgsContain(t, argv, arg)
	}
	assertArgsDoNotContainKey(t, argv, "--fingerprint-portable-cookies")
}

func TestAdsPowerUnreadablePendingDefersStartupNavigation(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix permission enforcement")
	}
	root := t.TempDir()
	coreDir := filepath.Join(root, "core")
	exe := filepath.Join(coreDir, filepath.FromSlash(browser.CoreExecutableCandidates()[0]))
	os.MkdirAll(filepath.Dir(exe), 0755)
	os.WriteFile(exe, []byte("stub"), 0755)
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = filepath.Join(root, "profiles")
	cfg.Browser.Cores = []browser.Core{{CoreId: "core", CoreName: "test", CorePath: coreDir, IsDefault: true, FingerprintEngine: "cloakbrowser"}}
	app := NewApp(root)
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, root)
	profile := &browser.Profile{ProfileId: "pending", CoreId: "core"}
	dir := filepath.Join(cfg.Browser.UserDataRoot, profile.ProfileId, ".ant-import")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "adspower-pending.json"), []byte(`{"cookies":[],"tabs":[]}`), 0600)
	os.Chmod(dir, 0000)
	defer os.Chmod(dir, 0700)
	plan, err := app.prepareBrowserStartPlan(browserStartInput{ProfileID: profile.ProfileId, StartURLs: []string{"https://example.invalid/"}, SkipDefaultStartURLs: true}, profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range plan.args {
		if arg == "https://example.invalid/" {
			t.Fatal("startup navigates before unreadable pending session")
		}
	}
	if len(plan.deferredStartTargets) == 0 {
		t.Fatal("deferred startup targets were lost")
	}
}
