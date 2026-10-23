package browser

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Pinned by upstream js/src/config.ts (BINARY_SIGNING_PUBKEYS).
// https://github.com/CloakHQ/cloakbrowser/blob/main/js/src/config.ts
const cloakBrowserSigningKey = "MKFKwIhUcKWq5xTuNA0Ovg99njcDEcEJvmWYYhApvaU="

var cloakBrowserVersion = regexp.MustCompile(`^chromium-v([0-9]+(?:\.[0-9]+){3,4})$`)
var cloakBrowserAsset = regexp.MustCompile(`^cloakbrowser-(?:windows-x64\.zip|(?:linux|darwin)-(?:x64|arm64)\.tar\.gz)$`)

// Official CloakBrowser downloads must be authenticated before any extraction.
// Other manually supplied publishers retain the existing download contract.
func verifyCloakBrowserArchive(ctx context.Context, client *http.Client, rawURL, archivePath string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if !strings.EqualFold(u.Hostname(), "github.com") || len(parts) < 2 || !strings.EqualFold(parts[0], "CloakHQ") || !strings.EqualFold(parts[1], "cloakbrowser") {
		return nil
	}
	if u.Scheme != "https" || u.Port() != "" || u.User != nil || len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" || !cloakBrowserAsset.MatchString(parts[5]) {
		return fmt.Errorf("CloakBrowser 官方下载地址无效")
	}
	version := cloakBrowserVersion.FindStringSubmatch(parts[4])
	if version == nil {
		return fmt.Errorf("CloakBrowser 发布版本无效")
	}
	base := "https://github.com/CloakHQ/cloakbrowser/releases/download/" + parts[4] + "/"
	verifyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	manifest, err := fetchCloakIntegrityFile(verifyCtx, client, base+"SHA256SUMS", 64*1024)
	if err != nil {
		return err
	}
	signature, err := fetchCloakIntegrityFile(verifyCtx, client, base+"SHA256SUMS.sig", 1024)
	if err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	key, err := base64.StdEncoding.DecodeString(cloakBrowserSigningKey)
	if err != nil {
		return err
	}
	return verifyCloakManifest(file, manifest, signature, version[1], parts[5], ed25519.PublicKey(key))
}

func fetchCloakIntegrityFile(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CloakBrowser 校验清单请求失败: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("CloakBrowser 校验清单过大")
	}
	return data, nil
}

func verifyCloakManifest(archive io.Reader, manifest, signature []byte, version, asset string, key ed25519.PublicKey) error {
	sig, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(signature)))
	if err != nil || len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, manifest, sig) {
		return fmt.Errorf("CloakBrowser 校验清单签名无效")
	}
	declared, expected := "", ""
	for _, line := range strings.Split(string(manifest), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version=") {
			declared = strings.TrimPrefix(line, "version=")
		}
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			if expected != "" {
				return fmt.Errorf("CloakBrowser 校验清单包含重复文件")
			}
			expected = fields[0]
		}
	}
	if declared != version {
		return fmt.Errorf("CloakBrowser 校验清单版本不匹配")
	}
	digest, err := hex.DecodeString(expected)
	if err != nil || len(digest) != sha256.Size {
		return fmt.Errorf("CloakBrowser 校验清单缺少有效的文件哈希")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, archive); err != nil {
		return err
	}
	if !strings.EqualFold(expected, hex.EncodeToString(hash.Sum(nil))) {
		return fmt.Errorf("CloakBrowser 文件 SHA-256 校验失败")
	}
	return nil
}
