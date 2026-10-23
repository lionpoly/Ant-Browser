package browser

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Explicit engine wins; names/paths keep pre-migration installations usable.
func ResolveFingerprintEngine(core Core) string {
	if core.FingerprintEngine == "cloakbrowser" || core.FingerprintEngine == "fingerprint-chromium" {
		return core.FingerprintEngine
	}
	if strings.Contains(strings.ToLower(core.CoreName+" "+core.CorePath), "cloakbrowser") {
		return "cloakbrowser"
	}
	return "fingerprint-chromium"
}

// Called only after the official archive signature/digest and executable checks.
// The release patch can be more precise than Chromium's four-part app version.
func writeCloakDownloadedVersion(dir, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if !strings.EqualFold(u.Hostname(), "github.com") || len(parts) != 6 || !strings.EqualFold(parts[0], "CloakHQ") || !strings.EqualFold(parts[1], "cloakbrowser") || parts[2] != "releases" || parts[3] != "download" || !cloakBrowserAsset.MatchString(parts[5]) {
		return nil
	}
	match := cloakBrowserVersion.FindStringSubmatch(parts[4])
	if len(match) != 2 {
		return nil
	}
	b, err := json.Marshal(struct {
		Version string `json:"version"`
	}{Version: match[1]})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ".ant-core-version.json"), b, 0600)
}
