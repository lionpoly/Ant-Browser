package backend

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func parseImportedCookies(content string) ([]CookieInfo, error) {
	if len(content) > 2*1024*1024 || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("请提供不超过 2 MB 的 Cookie JSON 或 Netscape 文本")
	}
	content = strings.TrimPrefix(content, "\ufeff")
	var cookies []CookieInfo
	if strings.HasPrefix(strings.TrimSpace(content), "[") || strings.HasPrefix(strings.TrimSpace(content), "{") {
		type record struct {
			CookieInfo
			ExpirationDate *float64 `json:"expirationDate"`
			Session        bool     `json:"session"`
		}
		var records []record
		if strings.HasPrefix(strings.TrimSpace(content), "{") {
			var wrapper struct {
				Cookies []record `json:"cookies"`
			}
			if err := json.Unmarshal([]byte(content), &wrapper); err != nil {
				return nil, fmt.Errorf("Cookie JSON 格式无效")
			}
			records = wrapper.Cookies
		} else if err := json.Unmarshal([]byte(content), &records); err != nil {
			return nil, fmt.Errorf("Cookie JSON 格式无效")
		}
		for _, r := range records {
			cookie := r.CookieInfo
			if r.ExpirationDate != nil {
				cookie.Expires = *r.ExpirationDate
			}
			if r.Session {
				cookie.Expires = -1
			}
			cookies = append(cookies, cookie)
		}
	} else {
		for i, line := range strings.Split(content, "\n") {
			line = strings.TrimSuffix(line, "\r")
			httpOnly := strings.HasPrefix(line, "#HttpOnly_")
			if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") && !httpOnly {
				continue
			}
			line = strings.TrimPrefix(line, "#HttpOnly_")
			fields := strings.SplitN(line, "\t", 7)
			if len(fields) != 7 {
				return nil, fmt.Errorf("Netscape 第 %d 行格式无效，需要 7 列", i+1)
			}
			expires, err := strconv.ParseFloat(fields[4], 64)
			if err != nil {
				return nil, fmt.Errorf("Netscape 第 %d 行过期时间无效", i+1)
			}
			if fields[3] != "TRUE" && fields[3] != "FALSE" {
				return nil, fmt.Errorf("Netscape 第 %d 行 Secure 值无效", i+1)
			}
			cookies = append(cookies, CookieInfo{Name: fields[5], Value: fields[6], Domain: fields[0], Path: fields[2], Expires: expires, HttpOnly: httpOnly, Secure: fields[3] == "TRUE"})
		}
	}
	if len(cookies) == 0 || len(cookies) > 2000 {
		return nil, fmt.Errorf("每次应导入 1 至 2000 条 Cookie")
	}
	for i := range cookies {
		c := &cookies[i]
		if c.Name == "" || strings.ContainsAny(c.Name, " \t\r\n;=,") || strings.IndexFunc(c.Name, unicode.IsControl) >= 0 || c.Domain == "" || strings.ContainsAny(c.Domain, "/\\ \t\r\n") || strings.ContainsAny(c.Value, "\r\n") {
			return nil, fmt.Errorf("第 %d 条 Cookie 的名称、域名或值无效", i+1)
		}
		if c.Path == "" {
			c.Path = "/"
		}
		if !strings.HasPrefix(c.Path, "/") || strings.ContainsAny(c.Path, "\r\n") {
			return nil, fmt.Errorf("第 %d 条 Cookie 路径无效", i+1)
		}
		if c.Expires != c.Expires || c.Expires > 253402300799 || c.Expires < -1 {
			return nil, fmt.Errorf("第 %d 条 Cookie 过期时间无效", i+1)
		}
		switch strings.ToLower(c.SameSite) {
		case "", "unspecified":
			c.SameSite = ""
		case "none", "no_restriction":
			c.SameSite = "None"
		case "lax":
			c.SameSite = "Lax"
		case "strict":
			c.SameSite = "Strict"
		default:
			return nil, fmt.Errorf("第 %d 条 Cookie 的 SameSite 无效", i+1)
		}
		if c.SameSite == "None" && !c.Secure {
			return nil, fmt.Errorf("第 %d 条 Cookie 使用 SameSite=None 时必须开启 Secure", i+1)
		}
	}
	return cookies, nil
}

// BrowserImportCookies only handles user-supplied content; values are never logged.
func (a *App) BrowserImportCookies(profileID, content string) (int, error) {
	cookies, err := parseImportedCookies(content)
	if err != nil {
		return 0, err
	}
	debugPort, err := a.getDebugPort(profileID)
	if err != nil {
		return 0, err
	}
	params := make([]map[string]any, 0, len(cookies))
	for _, c := range cookies {
		p := map[string]any{"name": c.Name, "value": c.Value, "domain": c.Domain, "path": c.Path, "httpOnly": c.HttpOnly, "secure": c.Secure}
		if c.Expires > 0 {
			p["expires"] = c.Expires
		}
		if c.SameSite != "" {
			p["sameSite"] = c.SameSite
		}
		params = append(params, p)
	}
	if _, err := cdpCall(debugPort, "Network.setCookies", map[string]any{"cookies": params}); err != nil {
		return 0, fmt.Errorf("Cookie 导入失败，请检查格式及实例调试连接")
	}
	return len(cookies), nil
}
