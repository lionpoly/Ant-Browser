package backend

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ant-chrome/backend/internal/browser"
)

const adsPowerMaxBytes = 64 << 20
const adsPowerMaxRows = 10000
const adsPowerMetadataPath = ".ant-import/adspower.json"
const adsPowerPendingPath = ".ant-import/adspower-pending.json"

// Source fields are retained locally, including fields without an Ant equivalent.
type adsPowerPending struct {
	Cookies        []map[string]any `json:"cookies"`
	Tabs           []string         `json:"tabs"`
	SkippedCookies int              `json:"skippedCookies,omitempty"`
}

func readAdsPowerFile(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("无法读取 AdsPower 文件")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, adsPowerMaxBytes+1))
	if err != nil || len(b) > adsPowerMaxBytes {
		return nil, fmt.Errorf("AdsPower 文件过大或读取失败（上限 64 MB）")
	}
	return b, nil
}
func parseAdsPowerTXT(data []byte) ([]map[string]string, error) {
	if len(data) > adsPowerMaxBytes {
		return nil, fmt.Errorf("AdsPower TXT 超出大小限制")
	}
	rows := []map[string]string{}
	row := map[string]string{}
	flush := func() {
		if len(row) > 0 {
			rows = append(rows, row)
			row = map[string]string{}
		}
	}
	s := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, []byte{239, 187, 191})))
	s.Buffer(make([]byte, 4096), adsPowerMaxBytes)
	for s.Scan() {
		line := strings.TrimSuffix(s.Text(), "\r")
		if strings.Trim(line, "* \t") == "" {
			if strings.Contains(line, "*") {
				flush()
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("AdsPower TXT 行格式无效")
		}
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 128 || len(row) >= 128 {
			return nil, fmt.Errorf("AdsPower TXT 字段名为空")
		}
		if _, exists := row[key]; exists {
			return nil, fmt.Errorf("AdsPower TXT 存在重复字段")
		}
		row[key] = value
		if len(rows) >= adsPowerMaxRows {
			return nil, fmt.Errorf("AdsPower 实例数量超出限制")
		}
	}
	if s.Err() != nil {
		return nil, fmt.Errorf("AdsPower TXT 读取失败")
	}
	flush()
	return validateAdsPowerRows(rows)
}
func validateAdsPowerRows(rows []map[string]string) ([]map[string]string, error) {
	if len(rows) == 0 || len(rows) > adsPowerMaxRows {
		return nil, fmt.Errorf("AdsPower 数据为空或实例数量超出限制")
	}
	seen := map[string]bool{}
	for i, r := range rows {
		id := r["id"]
		if id == "" {
			id = r["acc_id"]
		}
		if id == "" {
			return nil, fmt.Errorf("AdsPower 第 %d 条记录缺少 id/acc_id", i+1)
		}
		if seen[id] {
			return nil, fmt.Errorf("AdsPower 存在重复实例 ID")
		}
		seen[id] = true
	}
	return rows, nil
}

// XLSX is read as bounded OOXML; formulas and external relationships are never evaluated.
func parseAdsPowerXLSX(p string) ([]map[string]string, error) {
	b, err := readAdsPowerFile(p)
	if err != nil {
		return nil, err
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("AdsPower XLSX 格式无效")
	}
	files := map[string]*zip.File{}
	var total uint64
	for _, f := range z.File {
		if f.UncompressedSize64 > adsPowerMaxBytes {
			return nil, fmt.Errorf("AdsPower XLSX 内容过大")
		}
		total += f.UncompressedSize64
		if total > adsPowerMaxBytes || len(z.File) > 1000 {
			return nil, fmt.Errorf("AdsPower XLSX 解压大小超出限制")
		}
		if files[f.Name] != nil {
			return nil, fmt.Errorf("XLSX 存在重复文件")
		}
		files[f.Name] = f
	}
	readXML := func(name string, out any) error {
		f := files[name]
		if f == nil {
			return fmt.Errorf("XLSX 缺少必要内容")
		}
		r, e := f.Open()
		if e != nil {
			return fmt.Errorf("XLSX 内容无法读取")
		}
		defer r.Close()
		if e = xml.NewDecoder(io.LimitReader(r, adsPowerMaxBytes+1)).Decode(out); e != nil {
			return fmt.Errorf("XLSX XML 无效")
		}
		return nil
	}
	type rich struct {
		Text string `xml:"t"`
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	}
	richText := func(r rich) string {
		s := r.Text
		for _, v := range r.Runs {
			s += v.Text
		}
		return s
	}
	var shared struct {
		Items []rich `xml:"si"`
	}
	if files["xl/sharedStrings.xml"] != nil {
		if err = readXML("xl/sharedStrings.xml", &shared); err != nil {
			return nil, err
		}
	}
	var workbook struct {
		Sheets []struct {
			ID string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	if err = readXML("xl/workbook.xml", &workbook); err != nil {
		return nil, err
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err = readXML("xl/_rels/workbook.xml.rels", &rels); err != nil {
		return nil, err
	}
	if len(workbook.Sheets) != 1 {
		return nil, fmt.Errorf("AdsPower XLSX 必须包含一个数据工作表")
	}
	sheet := ""
	for _, r := range rels.Items {
		if r.ID == workbook.Sheets[0].ID && r.Mode != "External" {
			sheet = path.Clean(path.Join("xl", r.Target))
			if strings.HasPrefix(r.Target, "/") {
				sheet = strings.TrimPrefix(r.Target, "/")
			}
		}
	}
	if !strings.HasPrefix(sheet, "xl/worksheets/") {
		return nil, fmt.Errorf("XLSX 工作表路径无效")
	}
	var ws struct {
		Rows []struct {
			Cells []struct {
				Ref     string  `xml:"r,attr"`
				Type    string  `xml:"t,attr"`
				Value   string  `xml:"v"`
				Inline  rich    `xml:"is"`
				Formula *string `xml:"f"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err = validateAdsPowerSheetShape(files[sheet]); err != nil {
		return nil, err
	}
	if err = readXML(sheet, &ws); err != nil {
		return nil, err
	}
	if len(ws.Rows) < 2 || len(ws.Rows) > adsPowerMaxRows+1 {
		return nil, fmt.Errorf("XLSX 记录数无效")
	}
	headers := map[int]string{}
	rows := []map[string]string{}
	for ri, row := range ws.Rows {
		values := map[int]string{}
		for _, c := range row.Cells {
			if c.Formula != nil {
				return nil, fmt.Errorf("AdsPower XLSX 不接受公式单元格")
			}
			col := 0
			for _, v := range c.Ref {
				if v < 'A' || v > 'Z' {
					break
				}
				col = col*26 + int(v-'A'+1)
			}
			if col == 0 || col > 128 {
				return nil, fmt.Errorf("XLSX 单元格位置无效")
			}
			if _, ok := values[col]; ok {
				return nil, fmt.Errorf("XLSX 单元格重复")
			}
			v := c.Value
			switch c.Type {
			case "s":
				n, e := strconv.Atoi(v)
				if e != nil || n < 0 || n >= len(shared.Items) {
					return nil, fmt.Errorf("XLSX 共享字符串无效")
				}
				v = richText(shared.Items[n])
			case "inlineStr":
				v = richText(c.Inline)
			case "e":
				return nil, fmt.Errorf("XLSX 包含错误单元格")
			}
			values[col] = v
		}
		if ri == 0 {
			seen := map[string]bool{}
			for col, v := range values {
				v = strings.TrimSpace(v)
				if v != "" {
					if seen[v] {
						return nil, fmt.Errorf("XLSX 字段名重复")
					}
					seen[v] = true
					headers[col] = v
				}
			}
			if !seen["id"] && !seen["acc_id"] {
				return nil, fmt.Errorf("XLSX 不是 AdsPower 导出格式")
			}
			continue
		}
		r := map[string]string{}
		nonempty := false
		for col, value := range values {
			if key, ok := headers[col]; ok {
				r[key] = value
				nonempty = nonempty || value != ""
			}
		}
		if nonempty {
			rows = append(rows, r)
		}
	}
	return validateAdsPowerRows(rows)
}
func adsPowerID(kind, value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("adspower-%s-%x", kind, sum[:12])
}
func adsPowerProxy(r map[string]string) (string, error) {
	raw := strings.TrimSpace(r["proxy"])
	kind := strings.ToLower(strings.TrimSpace(r["proxytype"]))
	if kind == "no_proxy" || kind == "noproxy" || kind == "none" || kind == "direct" {
		if raw != "" {
			return "", fmt.Errorf("直连类型与代理内容冲突")
		}
		return "", nil
	}
	if raw == "" {
		if kind != "" {
			return "", fmt.Errorf("代理内容为空")
		}
		return "", nil
	}
	if kind == "socks5h" {
		return "", fmt.Errorf("不支持静默转换 socks5h 的远程 DNS 语义，请使用连接栈已确认的代理格式")
	}
	if kind != "http" && kind != "https" && kind != "socks5" {
		return "", fmt.Errorf("代理类型不受支持")
	}
	var u *url.URL
	if strings.Contains(raw, "://") {
		var e error
		u, e = url.Parse(raw)
		if e != nil || u.Scheme != kind {
			return "", fmt.Errorf("代理 URL 无效")
		}
	} else {
		parts := strings.SplitN(raw, ":", 4)
		if len(parts) != 2 && len(parts) != 4 {
			return "", fmt.Errorf("代理格式无效")
		}
		u = &url.URL{Scheme: kind, Host: net.JoinHostPort(parts[0], parts[1])}
		if len(parts) == 4 {
			u.User = url.UserPassword(parts[2], parts[3])
		}
	}
	port, e := strconv.Atoi(u.Port())
	if e != nil || port < 1 || port > 65535 || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("代理地址无效")
	}
	return u.String(), nil
}
func adsPowerCookies(raw string) ([]map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var src []map[string]any
	if json.Unmarshal([]byte(raw), &src) != nil {
		return nil, fmt.Errorf("Cookie JSON 无效")
	}
	if len(src) > 20000 {
		return nil, fmt.Errorf("Cookie 数量超出限制")
	}
	out := []map[string]any{}
	for _, c := range src {
		n, nok := c["name"].(string)
		v, vok := c["value"].(string)
		d, dok := c["domain"].(string)
		if !nok || !vok || !dok || n == "" || d == "" {
			return nil, fmt.Errorf("Cookie 必要字段无效")
		}
		if hostOnly, _ := c["hostOnly"].(bool); hostOnly {
			d = strings.TrimPrefix(d, ".")
		}
		x := map[string]any{"name": n, "value": v, "domain": d, "path": "/"}
		if p, ok := c["path"].(string); ok && p != "" {
			x["path"] = p
		}
		for _, k := range []string{"secure", "httpOnly"} {
			if v, ok := c[k].(bool); ok {
				x[k] = v
			}
		}
		if session, _ := c["session"].(bool); !session {
			expiry := c["expires"]
			if expiry == nil {
				expiry = c["expirationDate"]
			}
			var exp float64
			switch v := expiry.(type) {
			case float64:
				exp = v
			case string:
				var err error
				exp, err = strconv.ParseFloat(v, 64)
				if err != nil {
					return nil, fmt.Errorf("Cookie 有效期无效")
				}
			}
			if exp > 0 {
				x["expires"] = exp
			}

		}
		if s, ok := c["sameSite"].(string); ok {
			switch strings.ToLower(s) {
			case "none", "no_restriction":
				x["sameSite"] = "None"
			case "lax":
				x["sameSite"] = "Lax"
			case "strict":
				x["sameSite"] = "Strict"
			}
		}
		out = append(out, x)
	}
	return out, nil
}
func adsPowerList(raw string) []string {
	var a []string
	if json.Unmarshal([]byte(raw), &a) == nil {
		return a
	}
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
}
func buildAdsPowerContents(rows []map[string]string) (*profilePackageContents, error) {
	if _, err := validateAdsPowerRows(rows); err != nil {
		return nil, err
	}
	snapshot := &ProfilePackageDatabase{Format: profilePackageDatabaseFormat, Version: profilePackageDatabaseVersion, Warnings: []string{"AdsPower 导出不含完整浏览器目录及专有指纹；已转换可用字段，原始字段保存在实例目录 .ant-import/adspower.json。Cookie 与标签页将在首次启动时恢复；登录状态仍可能需要重新验证。UA/屏幕需在目标内核核对一致性；代理实例已禁止非代理 UDP，但无法还原源文件未包含的完整防泄漏设置。"}}
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	groups := map[string]bool{}
	for i, r := range rows {
		sourceID := r["id"]
		if sourceID == "" {
			sourceID = r["acc_id"]
		}
		id := adsPowerID("profile", sourceID)
		name := strings.TrimSpace(r["name"])
		if name == "" {
			suffix := r["acc_id"]
			if suffix == "" {
				suffix = sourceID
			}
			name = "AdsPower " + suffix
		}
		p := browser.Profile{ProfileId: id, ProfileName: name, Tags: adsPowerList(r["tags"]), FingerprintArgs: []string{}, LaunchArgs: []string{}}
		proxy, err := adsPowerProxy(r)
		if err != nil {
			return nil, fmt.Errorf("AdsPower 第 %d 条记录：%w", i+1, err)
		}
		p.ProxyConfig = proxy
		if proxy == "" {
			snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("第 %d 条记录将使用直连，未配置有效代理。", i+1))
		} else {
			p.FingerprintArgs = append(p.FingerprintArgs, "--disable-non-proxied-udp")
		}

		if group := strings.TrimSpace(r["group"]); group != "" {
			p.GroupId = adsPowerID("group", group)
			if !groups[group] {
				snapshot.Groups = append(snapshot.Groups, ProfilePackageDatabaseGroup{GroupId: p.GroupId, GroupName: group})
				groups[group] = true
			}
		}
		if ua := r["ua"]; ua != "" {
			p.LaunchArgs = append(p.LaunchArgs, "--user-agent="+ua)
			switch {
			case strings.Contains(ua, "Windows NT"):
				p.FingerprintArgs = append(p.FingerprintArgs, "--fingerprint-platform=windows")
			case strings.Contains(ua, "Macintosh"):
				p.FingerprintArgs = append(p.FingerprintArgs, "--fingerprint-platform=macos")
			case strings.Contains(ua, "Linux"):
				p.FingerprintArgs = append(p.FingerprintArgs, "--fingerprint-platform=linux")
			}

		}
		if resolution := strings.TrimSpace(r["resolution"]); resolution != "" && !strings.EqualFold(resolution, "default") {
			dims := strings.FieldsFunc(resolution, func(c rune) bool { return c == 'x' || c == 'X' || c == '*' || c == ',' || c == '×' })
			if len(dims) != 2 {
				return nil, fmt.Errorf("AdsPower 第 %d 条记录分辨率无效", i+1)
			}
			for _, d := range dims {
				n, e := strconv.Atoi(d)
				if e != nil || n < 1 || n > 32768 {
					return nil, fmt.Errorf("AdsPower 第 %d 条记录分辨率无效", i+1)
				}
			}
			p.FingerprintArgs = append(p.FingerprintArgs, "--window-size="+strings.Join(dims, ","), "--fingerprint-screen-width="+dims[0], "--fingerprint-screen-height="+dims[1])
		}
		cookieRaw := r["cookie"]
		lowerCookie := strings.ToLower(cookieRaw)
		if (strings.Contains(cookieRaw, "过长") && strings.Contains(cookieRaw, "单元格上限")) || (strings.Contains(lowerCookie, "cookie") && strings.Contains(lowerCookie, "cell") && strings.Contains(lowerCookie, "limit")) {
			snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("第 %d 条记录的 Cookie 超出 Excel 单元格上限，源文件未包含 Cookie；请改用 AdsPower TXT 导出恢复登录态。", i+1))
			cookieRaw = ""
		}
		if strings.TrimSpace(r["cookie"]) == "" {
			snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("第 %d 条记录缺少 Cookie 数据，无法恢复源登录态；如需登录态请使用包含 Cookie 的 TXT 导出。", i+1))
		}
		cookies, err := adsPowerCookies(cookieRaw)
		if err != nil {
			return nil, fmt.Errorf("AdsPower 第 %d 条记录：%w", i+1, err)
		}
		tabs := []string{}
		for _, tab := range adsPowerTabs(r["tab"]) {
			tab = strings.TrimSpace(tab)
			if tab == "" {
				continue
			}
			u, e := url.Parse(tab)
			if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
				return nil, fmt.Errorf("AdsPower 第 %d 条记录标签页地址无效", i+1)
			}
			tabs = append(tabs, tab)
		}
		if len(tabs) > 100 {
			return nil, fmt.Errorf("AdsPower 第 %d 条记录标签页过多（上限100）", i+1)
		}
		for _, item := range []struct {
			name  string
			value any
		}{{adsPowerMetadataPath, r}, {adsPowerPendingPath, adsPowerPending{Cookies: cookies, Tabs: tabs}}} {
			header := &zip.FileHeader{Name: "user-data/" + id + "/" + item.name, Method: zip.Deflate}
			header.SetMode(0600)
			w, e := zw.CreateHeader(header)
			if e != nil {
				return nil, e
			}
			if e = json.NewEncoder(w).Encode(item.value); e != nil {
				return nil, e
			}
		}
		snapshot.Profiles = append(snapshot.Profiles, p)
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		return nil, err
	}
	return &profilePackageContents{Reader: zr, Profiles: snapshot.Profiles, DatabaseSnapshot: snapshot}, nil
}
func openAdsPowerContents(p string) (*profilePackageContents, error) {
	var rows []map[string]string
	var err error
	if strings.EqualFold(filepath.Ext(p), ".xlsx") {
		rows, err = parseAdsPowerXLSX(p)
	} else {
		var b []byte
		b, err = readAdsPowerFile(p)
		if err == nil {
			rows, err = parseAdsPowerTXT(b)
		}
	}
	if err != nil {
		return nil, err
	}
	return buildAdsPowerContents(rows)
}

// This runs before imported tabs navigate. The pending file stays intact on failure.
func restoreAdsPowerSession(dir string, port int) error {
	p := filepath.Join(dir, adsPowerPendingPath)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("AdsPower 会话恢复文件无法读取")
	}
	b, err := readAdsPowerFile(p)
	if err != nil {
		return fmt.Errorf("AdsPower 会话恢复文件无法读取")
	}
	var pending adsPowerPending
	if json.Unmarshal(b, &pending) != nil {
		return fmt.Errorf("AdsPower 会话恢复数据无效")
	}
	// Revalidate after reading: pending files can also arrive inside ordinary ZIP imports.
	rawCookies, err := json.Marshal(pending.Cookies)
	if err != nil {
		return fmt.Errorf("AdsPower 会话 Cookie 无效")
	}
	pending.Cookies, err = adsPowerCookies(string(rawCookies))
	if err != nil {
		return fmt.Errorf("AdsPower 会话 Cookie 无效")
	}
	for _, tab := range pending.Tabs {
		u, e := url.Parse(tab)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return fmt.Errorf("AdsPower 会话标签页地址无效")
		}
	}

	live := []map[string]any{}
	for _, c := range pending.Cookies {
		if e, ok := c["expires"].(float64); ok && e < float64(time.Now().Unix()) {
			continue
		}
		if c["sameSite"] == "None" && c["secure"] != true {
			pending.SkippedCookies++
			continue
		}
		live = append(live, c)
	}
	if len(live) > 0 {
		err = cdpBrowserCall(port, "Storage.setCookies", map[string]any{"cookies": live})
		if err != nil {
			if !strings.Contains(err.Error(), "浏览器级 CDP 错误:") {
				return fmt.Errorf("AdsPower Cookie 恢复连接失败；已暂缓启动页，保留数据以便下次启动重试")
			}
			deadline := time.Now().Add(30 * time.Second)
			for i, c := range live {
				if time.Now().After(deadline) {
					pending.Cookies = live[i:]
					if e := saveAdsPowerPending(p, pending); e != nil {
						return e
					}
					return fmt.Errorf("AdsPower Cookie 恢复超时；已暂缓启动页，下次启动仅重试剩余数据")
				}
				e := cdpBrowserCall(port, "Storage.setCookies", map[string]any{"cookies": []map[string]any{c}})
				if e != nil && !strings.Contains(e.Error(), "浏览器级 CDP 错误:") {
					pending.Cookies = live[i:]
					if e = saveAdsPowerPending(p, pending); e != nil {
						return e
					}
					return fmt.Errorf("AdsPower Cookie 恢复连接中断；已暂缓启动页，下次启动仅重试剩余数据")
				}
				if e != nil {
					pending.SkippedCookies++
				}
			}
		}
	}
	// Commit cookie progress before any site can establish a newer login session.
	pending.Cookies = nil
	if err = saveAdsPowerPending(p, pending); err != nil {
		return err
	}
	for len(pending.Tabs) > 0 {
		if err = createBrowserStartTarget(port, pending.Tabs[0]); err != nil {
			return fmt.Errorf("AdsPower 标签页恢复失败；已暂缓启动页，下次启动仅重试剩余标签页")
		}
		pending.Tabs = pending.Tabs[1:]
		if err = saveAdsPowerPending(p, pending); err != nil {
			return err
		}
	}
	if err = os.Remove(p); err != nil {
		return fmt.Errorf("AdsPower 会话已恢复，但待恢复标记清理失败")
	}
	if pending.SkippedCookies > 0 {
		return &adsPowerRestoreWarning{count: pending.SkippedCookies}
	}
	return nil
}

type adsPowerRestoreWarning struct{ count int }

func (w *adsPowerRestoreWarning) Error() string {
	return fmt.Sprintf("AdsPower 会话恢复完成；%d 个无效或被浏览器拒绝的 Cookie 未恢复，原始字段仍保留在导入元数据中", w.count)
}
func adsPowerRestoreBlocksNavigation(err error) bool {
	var warning *adsPowerRestoreWarning
	return err != nil && !errors.As(err, &warning)
}
func saveAdsPowerPending(p string, pending adsPowerPending) error {
	b, err := json.Marshal(pending)
	if err != nil {
		return fmt.Errorf("AdsPower 恢复进度编码失败")
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".adspower-progress-*")
	if err != nil {
		return fmt.Errorf("AdsPower 恢复进度无法保存；已暂缓启动页")
	}
	name := f.Name()
	defer os.Remove(name)
	_, writeErr := f.Write(b)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return fmt.Errorf("AdsPower 恢复进度无法保存；已暂缓启动页")
	}
	if err = os.Rename(name, p); err != nil {
		return fmt.Errorf("AdsPower 恢复进度无法保存；已暂缓启动页")
	}
	return nil
}

func (a *App) restoreAdsPowerSession(dir string, port int) error {
	a.adsPowerSessionMu.Lock()
	defer a.adsPowerSessionMu.Unlock()
	return restoreAdsPowerSession(dir, port)
}

// A comma inside a URL is legal. AdsPower's separator is a comma followed by
// another absolute URL; JSON arrays and line-delimited exports are accepted too.
func adsPowerTabs(raw string) []string {
	var tabs []string
	if json.Unmarshal([]byte(raw), &tabs) == nil {
		return tabs
	}
	for _, line := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' }) {
		start := 0
		for i := 0; i < len(line); i++ {
			if line[i] != ',' {
				continue
			}
			tail := strings.TrimLeft(line[i+1:], " \t")
			if strings.HasPrefix(tail, "https://") || strings.HasPrefix(tail, "http://") {
				tabs = append(tabs, strings.TrimSpace(line[start:i]))
				start = i + 1
			}
		}
		tabs = append(tabs, strings.TrimSpace(line[start:]))
	}
	return tabs
}

func validateAdsPowerSheetShape(file *zip.File) error {
	if file == nil {
		return fmt.Errorf("XLSX 工作表缺失")
	}
	r, err := file.Open()
	if err != nil {
		return fmt.Errorf("XLSX 工作表无法读取")
	}
	defer r.Close()
	decoder := xml.NewDecoder(io.LimitReader(r, adsPowerMaxBytes+1))
	rows, cells, rowCells := 0, 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("XLSX 工作表XML无效")
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "row":
			rows++
			rowCells = 0
		case "c":
			cells++
			rowCells++
		}
		if rows > adsPowerMaxRows+1 || rowCells > 128 || cells > 500000 {
			return fmt.Errorf("AdsPower XLSX 行/列/单元格数量超出限制")
		}
	}
}
