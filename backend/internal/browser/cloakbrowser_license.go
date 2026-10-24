package browser

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// SaveCloakBrowserLicenseKey uses the upstream binary's native credential file.
// This is shared by CloakBrowser on this machine; it is not profile data.
// Upstream contract: github.com/CloakHQ/CloakBrowser, cloakbrowser/license.py,
// build_launch_env: default_file is read by the binary without env injection.
func SaveCloakBrowserLicenseKey(key string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法确定 CloakBrowser 授权目录: %w", err)
	}
	return saveCloakBrowserLicense(filepath.Join(home, ".cloakbrowser", "license.key"), key)
}

func saveCloakBrowserLicense(path, key string) error {
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, "\r\n\x00") {
		return fmt.Errorf("请输入有效的 CloakBrowser Key（不能包含换行）")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("创建 CloakBrowser 授权目录失败: %w", err)
	}
	file, err := os.CreateTemp(dir, ".license-*")
	if err != nil {
		return fmt.Errorf("创建 CloakBrowser 授权文件失败: %w", err)
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(key)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("写入 CloakBrowser 授权失败: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭 CloakBrowser 授权文件失败: %w", closeErr)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("保存 CloakBrowser 启动授权失败: %w", err)
	}
	return nil
}

func IsCloakBrowserKeyedDownloadURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "cloakbrowser.dev") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return strings.HasPrefix(u.Path, "/api/download/") && cloakBrowserProVersion.MatchString(strings.TrimPrefix(u.Path, "/api/download/"))
}
