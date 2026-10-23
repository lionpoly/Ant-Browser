package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/proxy"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	_ "time/tzdata"
)

// Reuse the launch endpoint, including the acquired bridge. Never resolve the
// original subscription/node again or fall back to a different connector stack.
func resolveBrowserExitTimezone(effectiveProxy string) (string, error) {
	client, err := browserExitTimezoneClient(effectiveProxy)
	if err != nil {
		return "", err
	}
	defer client.CloseIdleConnections()
	return lookupExitTimezone(client, "https://ipwho.is/")
}

func browserExitTimezoneClient(effectiveProxy string) (*http.Client, error) {
	client, err := proxy.BuildProxyHTTPClient(effectiveProxy, "", nil, nil, nil, nil, config.BrowserConnectorXray, 12*time.Second)
	if err != nil {
		return nil, fmt.Errorf("无法建立出口检测连接")
	}
	// An empty/direct route must bypass HTTP_PROXY just like --no-proxy-server.
	if effectiveProxy == "" || effectiveProxy == "direct://" {
		client.Transport = &http.Transport{Proxy: nil}
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client, nil
}

func lookupExitTimezone(client *http.Client, endpoint string) (string, error) {
	response, err := client.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("出口 IP 定位请求失败或超时")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("出口 IP 定位服务暂不可用（HTTP %d）", response.StatusCode)
	}
	var result struct {
		Success  bool   `json:"success"`
		IP       string `json:"ip"`
		Timezone struct {
			ID string `json:"id"`
		} `json:"timezone"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&result); err != nil || !result.Success || net.ParseIP(result.IP) == nil {
		return "", fmt.Errorf("出口 IP 定位服务未返回有效位置")
	}
	if err := validateBrowserTimezone(result.Timezone.ID); err != nil {
		return "", err
	}
	return result.Timezone.ID, nil
}

func validateBrowserTimezone(zone string) error {
	if zone == "" || zone == "Local" || strings.TrimSpace(zone) != zone {
		return fmt.Errorf("出口 IP 定位服务未返回有效 IANA 时区")
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return fmt.Errorf("出口 IP 定位服务未返回有效 IANA 时区")
	}
	return nil
}

// Called with browserMgr.Mutex held, before any browser or extension helper starts.
func (a *App) updateBrowserStartTimezone(profile *BrowserProfile, effectiveProxy string, lookup func(string) (string, error)) error {
	if a.config == nil || !a.config.Browser.AutoTimezoneOnStart {
		return nil
	}
	zone, err := lookup(effectiveProxy)
	if err == nil {
		err = validateBrowserTimezone(zone)
	}
	if err != nil {
		return fmt.Errorf("实例启动已阻止：启动前 IP 时区检查失败：%w。请检查网络后重试，或在基础配置中关闭此检查。", err)
	}
	key := "--timezone"
	if core, ok := a.resolveBrowserCoreForFingerprintReport(profile.CoreId); ok && browser.ResolveFingerprintEngine(core) == "cloakbrowser" {
		key = "--fingerprint-timezone"
	}
	previousFingerprint, previousLaunch, previousUpdated := profile.FingerprintArgs, profile.LaunchArgs, profile.UpdatedAt
	profile.FingerprintArgs = append(withoutBrowserTimezoneArgs(profile.FingerprintArgs), key+"="+zone)
	profile.LaunchArgs = withoutBrowserTimezoneArgs(profile.LaunchArgs)
	profile.UpdatedAt = time.Now().Format(time.RFC3339)
	previousProfiles := a.config.Browser.Profiles
	var saveErr error
	if a.browserMgr.ProfileDAO != nil {
		saveErr = a.browserMgr.ProfileDAO.Upsert(profile)
	} else {
		saveErr = a.browserMgr.SaveProfiles()
	}
	if saveErr != nil {
		a.config.Browser.Profiles = previousProfiles
		profile.FingerprintArgs, profile.LaunchArgs, profile.UpdatedAt = previousFingerprint, previousLaunch, previousUpdated
		return fmt.Errorf("实例启动已阻止：自动时区配置保存失败，请检查存储权限后重试")
	}
	return nil
}

func withoutBrowserTimezoneArgs(args []string) []string {
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		key := strings.SplitN(arg, "=", 2)[0]
		if key == "--timezone" || key == "--fingerprint-timezone" {
			if arg == key && i+1 < len(args) && !strings.HasPrefix(strings.TrimSpace(args[i+1]), "--") {
				i++
			}
			continue
		}
		result = append(result, args[i])
	}
	return result
}
