package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/database"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func timezoneTestApp(t *testing.T) (*App, *BrowserProfile) {
	t.Helper()
	cfg := config.DefaultConfig()
	a := NewApp(t.TempDir())
	a.config = cfg
	a.browserMgr = browser.NewManager(cfg, a.appRoot)
	p := &BrowserProfile{ProfileId: "synthetic", FingerprintArgs: []string{"--lang=fr-FR", "--timezone=Europe/Paris", "--fingerprint-timezone=Asia/Tokyo"}, LaunchArgs: []string{"--timezone", "Asia/Tokyo", "--disable-sync"}}
	a.browserMgr.Profiles = map[string]*browser.Profile{p.ProfileId: p}
	return a, p
}

func TestStartupTimezoneGate(t *testing.T) {
	a, p := timezoneTestApp(t)
	original := append([]string{}, p.FingerprintArgs...)
	calls := 0
	lookup := func(route string) (string, error) {
		calls++
		if route != "socks5://127.0.0.1:12345" {
			t.Fatalf("wrong launch route %q", route)
		}
		return "America/Los_Angeles", nil
	}
	if err := a.updateBrowserStartTimezone(p, "ignored", lookup); err != nil || calls != 0 || !reflect.DeepEqual(original, p.FingerprintArgs) {
		t.Fatal("disabled gate changed behavior", err)
	}
	a.config.Browser.AutoTimezoneOnStart = true
	if err := a.updateBrowserStartTimezone(p, "socks5://127.0.0.1:12345", lookup); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !reflect.DeepEqual(p.FingerprintArgs, []string{"--lang=fr-FR", "--timezone=America/Los_Angeles"}) || !reflect.DeepEqual(p.LaunchArgs, []string{"--disable-sync"}) {
		t.Fatalf("unexpected profile: %+v", p)
	}
	cfg, err := config.Load(filepath.Join(a.appRoot, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Browser.AutoTimezoneOnStart || len(cfg.Browser.Profiles) != 1 || !reflect.DeepEqual(cfg.Browser.Profiles[0].FingerprintArgs, p.FingerprintArgs) {
		t.Fatal("timezone not persisted")
	}
	original = append([]string{}, p.FingerprintArgs...)
	for _, resolve := range []func(string) (string, error){func(string) (string, error) { return "", errors.New("timeout") }, func(string) (string, error) { return "Invalid/Zone", nil }} {
		if err := a.updateBrowserStartTimezone(p, "direct://", resolve); err == nil || !reflect.DeepEqual(original, p.FingerprintArgs) {
			t.Fatal("failed check must block without changing config")
		}
	}
	// Force persistence failure without touching user data.
	if err := os.Remove(filepath.Join(a.appRoot, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(a.appRoot, "config.yaml"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.updateBrowserStartTimezone(p, "direct://", func(string) (string, error) { return "Asia/Tokyo", nil }); err == nil || !reflect.DeepEqual(original, p.FingerprintArgs) {
		t.Fatal("save failure did not roll back")
	}
}

func TestExitTimezoneResponseValidation(t *testing.T) {
	for _, body := range []string{`{"success":true,"ip":"203.0.113.7","timezone":{"id":"America/Los_Angeles"}}`, `{"success":false,"ip":"203.0.113.7","timezone":{"id":"Asia/Tokyo"}}`, `{"success":true,"ip":"bad","timezone":{"id":"Asia/Tokyo"}}`, `{"success":true,"ip":"203.0.113.7","timezone":{"id":"Local"}}`, `{"success":true,"ip":"203.0.113.7","timezone":{"id":"Invalid/Zone"}}`, `{"success":true,"ip":"203.0.113.7"}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			zone, err := lookupExitTimezone(server.Client(), server.URL)
			if body == `{"success":true,"ip":"203.0.113.7","timezone":{"id":"America/Los_Angeles"}}` {
				if err != nil || zone != "America/Los_Angeles" {
					t.Fatal(zone, err)
				}
			} else if err == nil {
				t.Fatal("unverified timezone accepted")
			}
		})
	}
}

func TestTimezoneArgsForEngines(t *testing.T) {
	a, p := timezoneTestApp(t)
	a.config.Browser.AutoTimezoneOnStart = true
	a.config.Browser.Cores = []config.BrowserCore{{CoreId: "cloak", FingerprintEngine: "cloakbrowser"}}
	p.CoreId = "cloak"
	if err := a.updateBrowserStartTimezone(p, "direct://", func(string) (string, error) { return "Asia/Tokyo", nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.FingerprintArgs, []string{"--lang=fr-FR", "--fingerprint-timezone=Asia/Tokyo"}) {
		t.Fatal(p.FingerprintArgs)
	}
	if got := withoutBrowserTimezoneArgs([]string{"--timezone=UTC", "--fingerprint-timezone", "Asia/Tokyo", "--lang=en-US"}); !reflect.DeepEqual(got, []string{"--lang=en-US"}) {
		t.Fatal(got)
	}
}

func TestStartPlanTimezoneBeforeExecution(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprint(direct), func(t *testing.T) {
			a, p := timezoneTestApp(t)
			a.config.Browser.AutoTimezoneOnStart = true
			coreDir := t.TempDir()
			executable := filepath.Join(coreDir, browser.CoreExecutableCandidates()[0])
			if err := os.MkdirAll(filepath.Dir(executable), 0700); err != nil {
				t.Fatal(err)
			}
			// A launch would create a marker; planning must never execute this binary.
			marker := filepath.Join(coreDir, "launched")
			if err := os.WriteFile(executable, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			a.config.Browser.Cores = []config.BrowserCore{{CoreId: "test", CorePath: coreDir, IsDefault: true}}
			p.ProxyConfig = "http://127.0.0.1:12345"
			input := newBrowserStartInput(p.ProfileId, []string{"--timezone=Europe/London", "--disable-sync"}, nil, true, false, direct, "", "")
			if !direct {
				input.TemporaryProxyConfig = "socks5://127.0.0.1:23456"
			}
			expectedRoute := "socks5://127.0.0.1:23456"
			if direct {
				expectedRoute = "direct://"
			}
			calls := 0
			resolver := func(route string) (string, error) {
				calls++
				if route != expectedRoute {
					t.Fatalf("wrong actual route %q", route)
				}
				return "America/Los_Angeles", nil
			}
			plan, err := a.prepareBrowserStartPlanWithTimezoneLookup(input, p, resolver)
			if err != nil {
				t.Fatal(err)
			}
			defer plan.releaseBridgeIfNeeded(a)
			expected := buildBrowserFingerprintExpected(plan.args)
			if calls != 1 || expected.Timezone != "America/Los_Angeles" || expected.Language != "fr-FR" {
				t.Fatalf("wrong launch fingerprint: %+v", expected)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("browser executed during planning")
			}
			_, err = a.prepareBrowserStartPlanWithTimezoneLookup(input, p, func(string) (string, error) { return "", errors.New("synthetic timeout") })
			if err == nil {
				t.Fatal("unverified launch accepted")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("failed gate launched browser")
			}
		})
	}
}

func TestStartupTimezoneSettingsRoundTrip(t *testing.T) {
	a, _ := timezoneTestApp(t)
	if a.GetBrowserSettings().AutoTimezoneOnStart {
		t.Fatal("new switch must default off")
	}
	settings := a.GetBrowserSettings()
	settings.AutoTimezoneOnStart = true
	if err := a.SaveBrowserSettings(settings); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(a.resolveAppPath("config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	a.config = cfg
	if !a.GetBrowserSettings().AutoTimezoneOnStart {
		t.Fatal("saved setting lost after restart")
	}
	settings = a.GetBrowserSettings()
	settings.AutoTimezoneOnStart = false
	if err := a.SaveBrowserSettings(settings); err != nil {
		t.Fatal(err)
	}
	if a.GetBrowserSettings().AutoTimezoneOnStart {
		t.Fatal("cannot disable")
	}
}

func TestExitTimezoneTransportUsesLaunchEndpoint(t *testing.T) {
	proxyRequests := 0
	route := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyRequests++
		if r.URL.Host != "exit-check.invalid" {
			t.Errorf("unexpected destination %s", r.URL.Host)
		}
		fmt.Fprint(w, `{"success":true,"ip":"203.0.113.7","timezone":{"id":"Asia/Tokyo"}}`)
	}))
	defer route.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	client, err := browserExitTimezoneClient(route.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	zone, err := lookupExitTimezone(client, "http://exit-check.invalid/")
	if err != nil || zone != "Asia/Tokyo" || proxyRequests != 1 {
		t.Fatal("did not use launch endpoint", zone, err)
	}
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":true,"ip":"203.0.113.7","timezone":{"id":"Etc/UTC"}}`)
	}))
	defer direct.Close()
	for _, endpoint := range []string{"", "direct://"} {
		client, err := browserExitTimezoneClient(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		defer client.CloseIdleConnections()
		transport := client.Transport.(*http.Transport)
		if transport.Proxy != nil {
			t.Fatal("direct launch inherited system proxy")
		}
		if zone, err := lookupExitTimezone(client, direct.URL); err != nil || zone != "Etc/UTC" {
			t.Fatal(zone, err)
		}
	}
	if _, err := browserExitTimezoneClient("vless://invalid"); err == nil {
		t.Fatal("unresolved kernel route must not fall back to direct")
	}
}

func TestExitTimezoneServiceFailures(t *testing.T) {
	for _, status := range []int{302, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
			defer server.Close()
			if _, err := lookupExitTimezone(server.Client(), server.URL); err == nil {
				t.Fatal("service failure accepted")
			}
		})
	}
}

func TestStartupTimezoneSQLitePersistence(t *testing.T) {
	a, p := timezoneTestApp(t)
	a.config.Browser.AutoTimezoneOnStart = true
	db, err := database.NewDB(filepath.Join(t.TempDir(), "profiles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	a.browserMgr.ProfileDAO = browser.NewSQLiteProfileDAO(db.GetConn())
	if err := a.updateBrowserStartTimezone(p, "direct://", func(string) (string, error) { return "Asia/Tokyo", nil }); err != nil {
		t.Fatal(err)
	}
	stored, err := a.browserMgr.ProfileDAO.GetById(p.ProfileId)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.FingerprintArgs, p.FingerprintArgs) || !reflect.DeepEqual(stored.LaunchArgs, p.LaunchArgs) {
		t.Fatal("database lost timezone")
	}
	original := append([]string{}, p.FingerprintArgs...)
	db.Close()
	if err := a.updateBrowserStartTimezone(p, "direct://", func(string) (string, error) { return "Europe/Paris", nil }); err == nil || !reflect.DeepEqual(original, p.FingerprintArgs) {
		t.Fatal("database failure must block and roll back")
	}
}
