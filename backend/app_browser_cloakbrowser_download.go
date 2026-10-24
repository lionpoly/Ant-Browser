package backend

import (
	"ant-chrome/backend/internal/browser"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var cloakBrowserVersionPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){3,4}$`)

type BrowserCloakBrowserVersionRequest struct {
	LicenseKey  string `json:"licenseKey"`
	ProxyConfig string `json:"proxyConfig,omitempty"`
}

type BrowserCloakBrowserVersionResult struct {
	Version     string `json:"version"`
	Platform    string `json:"platform"`
	DownloadURL string `json:"downloadUrl"`
	Message     string `json:"message"`
}

func (a *App) BrowserCloakBrowserVersion(input BrowserCloakBrowserVersionRequest) BrowserCloakBrowserVersionResult {
	if a == nil {
		return BrowserCloakBrowserVersionResult{Message: "应用未初始化"}
	}
	key := strings.TrimSpace(input.LicenseKey)
	if key == "" {
		return BrowserCloakBrowserVersionResult{Message: "请输入 CloakBrowser Key"}
	}
	platform := cloakBrowserPlatformTagForHost()
	proxyConfig := strings.TrimSpace(input.ProxyConfig)
	if proxyConfig == "__system__" {
		proxyConfig = ""
	}
	client, _, err := a.unifiedProxyCoreHTTPClient(20*time.Second, proxyConfig)
	if err != nil {
		return BrowserCloakBrowserVersionResult{Platform: platform, Message: "创建查询代理失败: " + err.Error()}
	}
	req, err := http.NewRequest(http.MethodGet, "https://cloakbrowser.dev/api/download/version", nil)
	if err != nil {
		return BrowserCloakBrowserVersionResult{Platform: platform, Message: "创建版本查询失败: " + err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("X-Platform", platform)
	client = browser.ClientWithCloakRedirectPolicy(client, req.Header)
	resp, err := client.Do(req)
	if err != nil {
		return BrowserCloakBrowserVersionResult{Platform: platform, Message: "CloakBrowser 版本查询失败: " + err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BrowserCloakBrowserVersionResult{Platform: platform, Message: fmt.Sprintf("CloakBrowser 版本查询失败: HTTP %d", resp.StatusCode)}
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return BrowserCloakBrowserVersionResult{Platform: platform, Message: "CloakBrowser 版本响应格式无效"}
	}
	version := strings.TrimSpace(payload.Version)
	if !cloakBrowserVersionPattern.MatchString(version) {
		return BrowserCloakBrowserVersionResult{Platform: platform, Message: "CloakBrowser 没有返回可用版本"}
	}
	return BrowserCloakBrowserVersionResult{
		Version:     version,
		Platform:    platform,
		DownloadURL: "https://cloakbrowser.dev/api/download/" + version,
		Message:     "已获取当前平台最新版本",
	}
}

func cloakBrowserPlatformTagForHost() string {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	return runtime.GOOS + "-" + arch
}
