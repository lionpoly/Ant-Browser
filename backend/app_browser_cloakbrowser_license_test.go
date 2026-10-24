package backend

import (
	"ant-chrome/backend/internal/browser"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type cloakTestExitCode int

func (e cloakTestExitCode) Error() string { return fmt.Sprintf("exit status %d", e) }
func (e cloakTestExitCode) ExitCode() int { return int(e) }

func TestCloakBrowserLicenseExitMessages(t *testing.T) {
	for _, tc := range []struct {
		code int
		want string
	}{
		{76, "并发"}, {77, "缺失、无效或已过期"}, {78, "许可证服务器"}, {79, "本机配置"},
	} {
		err := newBrowserStartupExitError(browserProcessExitResult{Err: cloakTestExitCode(tc.code)})
		message := describeCloakBrowserLicenseFailure(err)
		if !strings.Contains(message, tc.want) {
			t.Fatalf("code %d: %s", tc.code, message)
		}
	}
	for _, err := range []error{fmt.Errorf("timeout"), newBrowserStartupExitError(browserProcessExitResult{Err: cloakTestExitCode(1)})} {
		if got := describeCloakBrowserLicenseFailure(err); got != "" {
			t.Fatalf("misclassified unrelated failure: %s", got)
		}
	}
}

func TestRejectedCloakDownloadPreservesLicense(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	path := filepath.Join(home, ".cloakbrowser", "license.key")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("cb_test_synthetic_existing"), 0600); err != nil {
		t.Fatal(err)
	}
	app := &App{browserMgr: &browser.Manager{}}
	for _, download := range []func() error{
		func() error {
			return app.BrowserCoreDownload("test", "https://cloakbrowser.dev/api/download/155.0.8059.39.1", "", "cb_test_synthetic_new")
		},
		func() error {
			return app.BrowserCoreRedownload("test", "https://cloakbrowser.dev/api/download/155.0.8059.39.1", "", "cb_test_synthetic_new")
		},
	} {
		if err := download(); err == nil {
			t.Fatal("uninitialized task accepted")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "cb_test_synthetic_existing" {
			t.Fatal("rejected download replaced existing license")
		}
	}
}

func TestBrowserReadyLicenseMessageEngineScope(t *testing.T) {
	err := newBrowserStartupExitError(browserProcessExitResult{Err: cloakTestExitCode(77)})
	for _, engine := range []string{"cloakbrowser", "fingerprint-chromium", ""} {
		got := describeBrowserReadyFailureForEngine(engine, "/test/chromium", 9222, time.Second, err)
		if strings.Contains(got, "保存启动授权") != (engine == "cloakbrowser") {
			t.Fatalf("engine %q: %s", engine, got)
		}
	}
}

func TestBrowserReadyFailureOmitsDevToolsAnnouncementFromReason(t *testing.T) {
	err := newBrowserStartupExitError(browserProcessExitResult{
		Err:        cloakTestExitCode(1),
		StderrTail: "DevTools listening on ws://127.0.0.1:60621/devtools/browser/test",
	})
	message := describeBrowserReadyFailure("/test/Chromium", 60621, time.Second, err)
	if strings.Contains(message, "DevTools listening") {
		t.Fatalf("informational DevTools announcement leaked into failure reason: %s", message)
	}
	if !strings.Contains(message, "exit status 1") {
		t.Fatalf("exit status was lost from failure reason: %s", message)
	}
}
