package backend

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// The public 145/146 builds and newer licensed builds use a distinct switch set.
// Version gates follow CloakHQ/cloakbrowser README, Fingerprint Management.
var cloakVersionedFlags = map[string]int{
	"--fingerprint-windows-font-metrics": 148, "--fingerprint-allow-3p-cookies": 148,
	"--fingerprint-sapi-voices": 150, "--fingerprint-transparent-proxy": 151,
	"--fingerprint-portable-cookies": 151, "--fingerprint-transparent-proxy-dns": 155,
}
var cloakNativeLabels = map[string]string{
	"--fingerprint": "指纹种子", "--fingerprint-brand": "品牌", "--fingerprint-brand-version": "品牌版本",
	"--fingerprint-platform": "平台", "--fingerprint-platform-version": "系统版本",
	"--fingerprint-hardware-concurrency": "CPU 核心数", "--fingerprint-device-memory": "设备内存",
	"--fingerprint-screen-width": "屏幕宽度", "--fingerprint-screen-height": "屏幕高度",
	"--fingerprint-gpu-vendor": "GPU 家族", "--fingerprint-timezone": "时区", "--fingerprint-locale": "语言",
	"--fingerprint-storage-quota": "存储配额", "--fingerprint-taskbar-height": "任务栏高度",
	"--fingerprint-webrtc-ip": "WebRTC IP", "--fingerprint-noise": "噪声",
}

func buildCloakBrowserFingerprintLaunchPlan(profileID string, raw []string, version string) browserFingerprintLaunchPlan {
	return buildCloakBrowserFingerprintLaunchPlanForHost(profileID, raw, version, runtime.GOOS)
}

func buildCloakBrowserFingerprintLaunchPlanForHost(profileID string, raw []string, version, host string) browserFingerprintLaunchPlan {
	args := normalizeNonEmptyStrings(raw)
	plan := browserFingerprintLaunchPlan{launchArgs: []string{}, rows: []BrowserFingerprintCapabilityRow{}, warnings: []string{}}
	major := parseChromeMajor(version)
	if browserArgWithKey(args, "--fingerprint") == "" && profileID != "" {
		plan.launchArgs = append(plan.launchArgs, fmt.Sprintf("--fingerprint=%d", browserFingerprintSeedForProfileID(profileID)))
	}
	for _, arg := range args {
		key := browserFingerprintArgKey(arg)
		value := browserArgValue([]string{arg}, key)
		runtimeArg := arg
		min := cloakVersionedFlags[key]
		if key == "--fingerprint" && isCloakFingerprintOff(value) {
			min = 148
		}
		row := BrowserFingerprintCapabilityRow{Capability: key, Status: "kept", InputArg: arg, RuntimeArg: arg, Action: "保留", Note: "原样保留的扩展参数；尚未确认内核支持，请核对所选版本"}
		if label, ok := cloakNativeLabels[key]; ok {
			row.Capability = label
			row.Note = "CloakBrowser 原生参数；实际支持范围取决于所选二进制版本"
		}
		if !cloakFingerprintSwitchAllowed(key) {
			row.Status = "managed_or_unrelated"
			row.RuntimeArg = ""
			row.Action = "运行时移除"
			row.Note = "指纹配置不能覆盖代理、调试接口或其他启动参数"
			plan.warnings = appendUniqueString(plan.warnings, row.Note)
		} else if key == "--fingerprint-transparent-proxy-dns" && (host != "linux" && host != "windows" || !cloakVersionAtLeast(version, "155.0.8059.39.1")) {
			row.Status = "unsupported_version"
			row.RuntimeArg = ""
			row.Action = "运行时移除"
			row.Note = "透明代理 DNS 需要 CloakBrowser 155.0.8059.39.1+ 且运行于 Linux/Windows"
			plan.warnings = appendUniqueString(plan.warnings, row.Note)
		} else if min > 0 && (major == 0 || major < min) {
			row.Status = "unsupported_version"
			row.RuntimeArg = ""
			row.Action = "运行时移除"
			row.Note = fmt.Sprintf("需要 CloakBrowser Chromium %d+；当前版本 %s", min, version)
			plan.warnings = appendUniqueString(plan.warnings, row.Note)
		} else if key == "--timezone" || key == "--lang" {
			native := "--fingerprint-timezone"
			if key == "--lang" {
				native = "--fingerprint-locale"
			}
			if browserArgWithKey(args, native) != "" {
				row.Status = "overridden"
				row.RuntimeArg = ""
				row.Action = "使用原生参数"
			} else {
				runtimeArg = native + "=" + value
				row.RuntimeArg = runtimeArg
				row.Status = "converted"
				row.Action = "转换"
			}
		} else if key == "--disable-spoofing" || key == "--disable-gpu-fingerprint" || key == "--fingerprint-gpu-renderer" || key == "--fingerprint-canvas-noise" || key == "--fingerprint-audio-noise" || key == "--fingerprint-client-rects-noise" || key == "--fingerprinting-canvas-image-data-noise" || key == "--fingerprinting-client-rects-noise" {
			row.Status = "unsupported"
			row.RuntimeArg = ""
			row.Action = "运行时移除"
			row.Note = "该开关属于 fingerprint-chromium；CloakBrowser 使用统一 --fingerprint-noise 和种子，不能逐项映射"
			plan.warnings = appendUniqueString(plan.warnings, row.Note)
		}
		if row.RuntimeArg != "" {
			// One value per switch; later configuration wins.
			runtimeKey := browserFingerprintArgKey(runtimeArg)
			for i := len(plan.launchArgs) - 1; i >= 0; i-- {
				if browserFingerprintArgKey(plan.launchArgs[i]) == runtimeKey {
					plan.launchArgs = append(plan.launchArgs[:i], plan.launchArgs[i+1:]...)
				}
			}
			plan.launchArgs = append(plan.launchArgs, runtimeArg)
		}
		plan.rows = append(plan.rows, row)
	}
	if browserArgWithKey(plan.launchArgs, "--fingerprint") == "" && profileID != "" {
		plan.launchArgs = append(plan.launchArgs, fmt.Sprintf("--fingerprint=%d", browserFingerprintSeedForProfileID(profileID)))
	}
	if locale := browserArgValue(plan.launchArgs, "--fingerprint-locale"); locale != "" {
		// Chromium's language prefs remain necessary on macOS public binaries.
		plan.launchArgs = append(plan.launchArgs, "--lang="+locale)
		if browserArgValue(plan.launchArgs, "--accept-lang") == "" {
			plan.launchArgs = append(plan.launchArgs, "--accept-lang="+buildAcceptLanguage(locale))
		}
	}
	return plan
}
func isCloakFingerprintOff(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off", "false", "0", "disable", "disabled":
		return true
	}
	return false
}

func cloakVersionAtLeast(version, minimum string) bool {
	a, b := strings.Split(version, "."), strings.Split(minimum, ".")
	for i := range b {
		x := 0
		if i < len(a) {
			var err error
			x, err = strconv.Atoi(a[i])
			if err != nil {
				return false
			}
		}
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return true
}

func cloakFingerprintSwitchAllowed(key string) bool {
	if strings.HasPrefix(key, "--fingerprint") {
		return true
	}
	switch key {
	case "--lang", "--timezone", "--accept-lang", "--window-size", "--disable-non-proxied-udp", "--webrtc-ip-handling-policy", "--enable-blink-features", "--disable-spoofing", "--disable-gpu-fingerprint":
		return true
	}
	return false
}
