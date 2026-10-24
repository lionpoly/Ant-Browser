package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"ant-chrome/backend/internal/proxy"
	"archive/zip"
	"bytes"
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdsPowerTXT(t *testing.T) {
	rows, err := parseAdsPowerTXT([]byte("\xef\xbb\xbfacc_id=12\nid=abc\nname=示例\nremark=a=b\ncookie=[]\nproxytype=http\nproxy=localhost:8080:u:p=q\n********\nacc_id=13\nid=def\nname=第二个\ncookie=[]\n"))
	if err != nil || len(rows) != 2 || rows[0]["remark"] != "a=b" {
		t.Fatalf("rows parse failed: %v", err)
	}
	c, err := buildAdsPowerContents(rows)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if len(c.Profiles) != 2 || c.Profiles[0].ProxyConfig != "http://u:p=q@localhost:8080" {
		t.Fatal("mapping failed")
	}
	if _, err = buildAdsPowerContents([]map[string]string{{"id": "x", "proxytype": "http", "proxy": "broken"}}); err == nil {
		t.Fatal("malformed proxy silently accepted")
	}
}
func TestAdsPowerXLSX(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	files := map[string]string{"xl/workbook.xml": `<workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="data" r:id="rId1"/></sheets></workbook>`, "xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`, "xl/sharedStrings.xml": `<sst><si><t>id</t></si><si><r><t>na</t></r><r><t>me</t></r></si><si><t>sample</t></si></sst>`, "xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c r="A1" t="s"><v>0</v></c><c r="C1" t="s"><v>1</v></c></row><row><c r="A2"><v>123</v></c><c r="C2" t="inlineStr"><is><t>A&amp;B</t></is></c></row></sheetData></worksheet>`}
	for n, s := range files {
		w, _ := z.Create(n)
		w.Write([]byte(s))
	}
	z.Close()
	p := filepath.Join(t.TempDir(), "sample.xlsx")
	os.WriteFile(p, b.Bytes(), 0600)
	rows, err := parseAdsPowerXLSX(p)
	if err != nil || len(rows) != 1 || rows[0]["name"] != "A&B" || rows[0]["id"] != "123" {
		t.Fatalf("sparse/shared/inline parse: %v", err)
	}
}

func TestAdsPowerProvidedFiles(t *testing.T) {
	dir := os.Getenv("ANT_ADSPOWER_SAMPLE_DIR")
	if dir == "" {
		t.Skip("local private samples only")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal("sample directory unavailable")
	}
	count := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".txt" && filepath.Ext(e.Name()) != ".xlsx" {
			continue
		}
		c, err := openAdsPowerContents(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("sample import: %v", err)
		}
		if len(c.Profiles) != 2 || len(c.DatabaseSnapshot.Groups) == 0 {
			t.Fatal("sample mapping counts failed")
		}
		for _, p := range c.Profiles {
			if p.ProxyConfig == "" || len(p.LaunchArgs) == 0 {
				t.Fatal("sample mapping incomplete")
			}
		}
		c.Close()
		count++
	}
	if count != 2 {
		t.Fatal("expected both sample formats")
	}
}

func TestAdsPowerImportUsesConflictAndStagingPipeline(t *testing.T) {
	root := t.TempDir()
	db := newProfilePackageDatabase(t, root)
	cfg := config.DefaultConfig()
	cfg.Browser.UserDataRoot = filepath.Join(root, "user-data")
	app := NewApp(root)
	app.config = cfg
	app.db = db
	app.browserMgr = browser.NewManager(cfg, root)
	app.browserMgr.ProfileDAO = browser.NewSQLiteProfileDAO(db.GetConn())
	app.browserMgr.GroupDAO = browser.NewSQLiteGroupDAO(db.GetConn())
	file := filepath.Join(root, "source.txt")
	os.WriteFile(file, []byte("id=synthetic\nacc_id=1\nname=合成实例\ngroup=导入组\nremark=a=b\nusername=example-user\npassword=example-only\ncookie=[]\nproxytype=http\nproxy=localhost:8080:user:example\nresolution=default\n"), 0600)
	preview, err := app.prepareProfilePackageImportFromPath(file)
	if err != nil || preview.ProfileCount != 1 {
		t.Fatalf("preview failed: %v", err)
	}
	result, err := app.importProfilePackageFromPath(file)
	if err != nil || result.ImportedCount != 1 {
		t.Fatalf("import failed: %v", err)
	}
	var id, name, proxy, group string
	if err = db.GetConn().QueryRow(`SELECT profile_id,profile_name,proxy_config,group_id FROM browser_profiles`).Scan(&id, &name, &proxy, &group); err != nil || name != "合成实例" || proxy != "http://user:example@localhost:8080" || group == "" {
		t.Fatal("persisted mapping failed")
	}
	var restoreMode string
	if err = db.GetConn().QueryRow(`SELECT restore_last_session FROM browser_profiles WHERE profile_id = ?`, id).Scan(&restoreMode); err != nil || restoreMode != browser.RestoreLastSessionEnabled {
		t.Fatal("imported session retention setting did not survive database import")
	}
	metadata := filepath.Join(cfg.Browser.UserDataRoot, id, adsPowerMetadataPath)
	b, err := os.ReadFile(metadata)
	if err != nil || !bytes.Contains(b, []byte("example-user")) {
		t.Fatal("source metadata missing")
	}
	st, _ := os.Stat(metadata)
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0600 {
		t.Fatalf("private metadata permissions %o", st.Mode().Perm())
	}
	exported := filepath.Join(root, "export.zip")
	if _, err = app.writeProfilePackage(exported, []browser.Profile{{ProfileId: id, ProfileName: name, UserDataDir: id, ProxyConfig: proxy, GroupId: group}}); err != nil {
		t.Fatal(err)
	}
	exportStat, _ := os.Stat(exported)
	if runtime.GOOS != "windows" && exportStat.Mode().Perm() != 0600 {
		t.Fatal("credential export not private")
	}
	zr, err := zip.OpenReader(exported)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, adsPowerMetadataPath) && f.Mode().Perm() != 0600 {
			t.Fatal("private ZIP entry mode lost")
		}
	}
	zr.Close()
	preview, err = app.prepareProfilePackageImportFromPath(file)
	if err != nil || preview.ConflictCount != 1 {
		t.Fatal("repeat import must offer conflict resolution")
	}
}

func TestAdsPowerSessionRestoreOrderingAndRetry(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700)
	pending := adsPowerPending{Cookies: []map[string]any{{"name": "synthetic", "value": "example", "domain": "example.invalid", "path": "/"}}, Tabs: []string{"https://example.invalid/"}}
	b, _ := json.Marshal(pending)
	p := filepath.Join(dir, adsPowerPendingPath)
	os.WriteFile(p, b, 0600)
	var mu sync.Mutex
	var methods []string
	fail := true
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": "ws://" + r.Host + "/ws"})
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var msg cdpMessage
		if conn.ReadJSON(&msg) != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		methods = append(methods, msg.Method)
		if fail {
			conn.Close()
		} else {
			conn.WriteJSON(map[string]any{"id": msg.Id, "result": map[string]any{}})
		}
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	if restoreAdsPowerSession(dir, port) == nil {
		t.Fatal("expected restore failure")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("failed restore lost pending data")
	}
	mu.Lock()
	firstOrderOK := len(methods) == 1 && methods[0] == "Storage.setCookies"
	fail = false
	methods = nil
	mu.Unlock()
	if !firstOrderOK {
		t.Fatal("navigation ran before cookie success")
	}
	if err := restoreAdsPowerSession(dir, port); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	secondOrderOK := len(methods) == 2 && methods[0] == "Storage.setCookies" && methods[1] == "Target.createTarget"
	mu.Unlock()
	if !secondOrderOK {
		t.Fatal("restore order incorrect")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("successful restore not consumed")
	}
	if err := restoreAdsPowerSession(dir, port); err != nil {
		t.Fatal("second launch should not replay")
	}
}

func TestAdsPowerCookiePlaceholderAndMalformedData(t *testing.T) {
	c, err := buildAdsPowerContents([]map[string]string{{"id": "x", "cookie": "Cookie过长，超出Excel单元格上限"}})
	if err != nil || !strings.Contains(strings.Join(c.DatabaseSnapshot.Warnings, " "), "单元格上限") {
		t.Fatal("Excel omitted cookies must be reported")
	}
	if _, err := buildAdsPowerContents([]map[string]string{{"id": "x", "cookie": "[{broken]"}}); err == nil {
		t.Fatal("malformed cookie silently accepted")
	}
	if _, err := parseAdsPowerTXT([]byte("id=a\nid=b\n")); err == nil {
		t.Fatal("duplicate TXT field accepted")
	}
}

// Opt-in local smoke: fresh disposable profile, synthetic cookie and localhost only.
func TestAdsPowerRealBrowserSessionSmoke(t *testing.T) {
	binary := os.Getenv("ANT_TEST_BROWSER_BINARY")
	if binary == "" {
		t.Skip("optional local browser smoke")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><title>synthetic-import</title></html>"))
	}))
	defer server.Close()
	dir := t.TempDir()
	port, err := nextAvailablePort()
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := buildCloakBrowserFingerprintLaunchPlan("smoke", []string{"--fingerprint=123", "--fingerprint-locale=ja-JP", "--fingerprint-timezone=Asia/Tokyo", "--fingerprint-device-memory=8", "--fingerprint-screen-width=1440", "--fingerprint-screen-height=900"}, "145.0.7632.109").launchArgs
	if err = writeBrowserLanguagePreferences(dir, fingerprint); err != nil {
		t.Fatal(err)
	}
	args := []string{"--headless=new", "--no-first-run", "--no-default-browser-check", "--disable-background-networking", "--disable-component-update", "--remote-debugging-port=" + strconv.Itoa(port), "--user-data-dir=" + dir}
	args = append(args, fingerprint...)
	args = append(args, "about:blank")
	cmd := exec.Command(binary, args...)

	if err = cmd.Start(); err != nil {
		t.Fatal("local browser failed to start")
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	deadline := time.Now().Add(20 * time.Second)
	for !browserDebugPortReady(port) {
		if time.Now().After(deadline) {
			t.Fatal("local browser debug port unavailable")
		}
		time.Sleep(100 * time.Millisecond)
	}
	pending := adsPowerPending{Cookies: []map[string]any{{"name": "ant-synthetic-import", "value": "example-only", "domain": "127.0.0.1", "path": "/"}}, Tabs: []string{server.URL}}
	os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700)
	b, _ := json.Marshal(pending)
	os.WriteFile(filepath.Join(dir, adsPowerPendingPath), b, 0600)
	if err = restoreAdsPowerSession(dir, port); err != nil {
		t.Fatal(err)
	}
	result, err := cdpCall(port, "Network.getAllCookies", nil)
	if err != nil {
		t.Fatal("cookie query failed")
	}
	cookies, _ := result["cookies"].([]any)
	found := false
	for _, entry := range cookies {
		c, _ := entry.(map[string]any)
		if c["name"] == "ant-synthetic-import" && c["value"] == "example-only" {
			found = true
		}
	}
	if !found {
		t.Fatal("imported synthetic cookie not present")
	}
	runtimeInfo, err := evaluateBrowserFingerprintRuntime(port)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeInfo.Language != "ja-JP" || runtimeInfo.Timezone != "Asia/Tokyo" || runtimeInfo.ScreenWidth != 1440 || runtimeInfo.ScreenHeight != 900 {
		t.Fatalf("native overrides: language=%q timezone=%q screen=%dx%d", runtimeInfo.Language, runtimeInfo.Timezone, runtimeInfo.ScreenWidth, runtimeInfo.ScreenHeight)
	}
}

func TestAdsPowerTabURLCommaPreserved(t *testing.T) {
	c, err := buildAdsPowerContents([]map[string]string{{"id": "tabs", "tab": "https://example.invalid/?q=a,b,https://example.invalid/second"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range c.Reader.File {
		if strings.HasSuffix(f.Name, adsPowerPendingPath) {
			r, _ := f.Open()
			defer r.Close()
			var p adsPowerPending
			json.NewDecoder(r).Decode(&p)
			if len(p.Tabs) != 2 || p.Tabs[0] != "https://example.invalid/?q=a,b" {
				t.Fatal("URL commas corrupted")
			}
			return
		}
	}
	t.Fatal("pending missing")
}

func TestAdsPowerPendingRejectsNonWebTabBeforeCDP(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700)
	p := filepath.Join(dir, adsPowerPendingPath)
	os.WriteFile(p, []byte(`{"cookies":[],"tabs":["file:///example-only"]}`), 0600)
	err := restoreAdsPowerSession(dir, 1)
	if err == nil || !strings.Contains(err.Error(), "标签页地址无效") {
		t.Fatal("pending tabs must be validated before any CDP call")
	}
	if _, e := os.Stat(p); e != nil {
		t.Fatal("rejected pending data lost")
	}
}

func TestAdsPowerImportGuards(t *testing.T) {
	if _, err := adsPowerProxy(map[string]string{"proxytype": "socks5h", "proxy": "localhost:1080"}); err == nil {
		t.Fatal("remote DNS silently downgraded")
	}
	c, err := buildAdsPowerContents([]map[string]string{{"id": "source-only", "cookie": "Cookie exceeds Excel cell limit"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Profiles[0].ProfileName != "AdsPower source-only" {
		t.Fatal("fallback name missing source ID")
	}
	cookies, err := adsPowerCookies(`[{"name":"persistent","value":"example","domain":".example.invalid","hostOnly":true,"expirationDate":"2000000000"}]`)
	if err != nil || cookies[0]["expires"] != float64(2000000000) || cookies[0]["domain"] != "example.invalid" {
		t.Fatal("persistent/host-only cookie mapping failed")
	}
}

func TestAdsPowerSheetBounds(t *testing.T) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("sheet.xml")
	w.Write([]byte(`<worksheet><sheetData><row>`))
	for i := 0; i < 129; i++ {
		w.Write([]byte(`<c r="A1"><v>x</v></c>`))
	}
	w.Write([]byte(`</row></sheetData></worksheet>`))
	z.Close()
	r, _ := zip.NewReader(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if err := validateAdsPowerSheetShape(r.File[0]); err == nil {
		t.Fatal("oversized/sparse sheet shape accepted")
	}
}

func TestAdsPowerRestoreProgressSurvivesTabFailure(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700)
	p := filepath.Join(dir, adsPowerPendingPath)
	pending := adsPowerPending{Cookies: []map[string]any{{"name": "synthetic", "value": "example", "domain": "example.invalid"}}, Tabs: []string{"https://example.invalid/first", "https://example.invalid/second"}}
	b, _ := json.Marshal(pending)
	os.WriteFile(p, b, 0600)
	var mu sync.Mutex
	calls := []string{}
	upgrader := websocket.Upgrader{}
	failSecond := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": "ws://" + r.Host + "/ws"})
			return
		}
		conn, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer conn.Close()
		var msg cdpMessage
		conn.ReadJSON(&msg)
		call := msg.Method
		if msg.Method == "Target.createTarget" {
			call, _ = msg.Params["url"].(string)
		}
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, call)
		if strings.HasSuffix(call, "/second") && failSecond {
			conn.Close()
			return
		}
		conn.WriteJSON(map[string]any{"id": msg.Id, "result": map[string]any{}})
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	if restoreAdsPowerSession(dir, port) == nil {
		t.Fatal("expected second tab failure")
	}
	b, _ = os.ReadFile(p)
	var after adsPowerPending
	json.Unmarshal(b, &after)
	if len(after.Cookies) != 0 || len(after.Tabs) != 1 || !strings.HasSuffix(after.Tabs[0], "/second") {
		t.Fatal("completed steps were not persisted")
	}
	mu.Lock()
	failSecond = false
	calls = nil
	mu.Unlock()
	if err := restoreAdsPowerSession(dir, port); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	retryOK := len(calls) == 1 && strings.HasSuffix(calls[0], "/second")
	mu.Unlock()
	if !retryOK {
		t.Fatal("retry replayed cookies or already-open tab")
	}
}

func TestAdsPowerAuthenticatedProxyUsesCurrentStackWithoutPoolID(t *testing.T) {
	raw, err := adsPowerProxy(map[string]string{"proxytype": "http", "proxy": "localhost:8080:synthetic:example"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ stack, engine string }{{config.BrowserConnectorXray, proxy.ProxyKernelXray}, {config.BrowserConnectorMihomo, proxy.ProxyKernelMihomo}} {
		r, err := proxy.ResolveProxyKernelForConnector(raw, nil, "", tc.stack)
		if err != nil || r.Kernel != tc.engine {
			t.Fatalf("proxy stack %s failed", tc.stack)
		}
	}
}

func TestAdsPowerSyntheticExportFixtures(t *testing.T) {
	for _, name := range []string{"profiles.txt", "profiles.xlsx"} {
		c, err := openAdsPowerContents(filepath.Join("testdata", "adspower", name))
		if err != nil {
			t.Fatal(err)
		}
		if len(c.Profiles) != 2 || c.Profiles[0].ProxyConfig == "" || len(c.Profiles[0].Tags) != 1 || c.Profiles[0].GroupId == "" {
			t.Fatal("fixture mapping incomplete")
		}
		c.Close()
	}
}

func TestAdsPowerPermanentlyRejectedCookieDoesNotBlockTabs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ant-import"), 0700)
	p := filepath.Join(dir, adsPowerPendingPath)
	b, _ := json.Marshal(adsPowerPending{Cookies: []map[string]any{{"name": "good", "value": "example", "domain": "example.invalid"}, {"name": "bad", "value": "example", "domain": "example.invalid"}, {"name": "insecure-none", "value": "example", "domain": "example.invalid", "sameSite": "None", "secure": false}}, Tabs: []string{"https://example.invalid/"}})
	os.WriteFile(p, b, 0600)
	var opened atomic.Bool
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": "ws://" + r.Host + "/ws"})
			return
		}
		conn, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer conn.Close()
		var msg cdpMessage
		conn.ReadJSON(&msg)
		bad := false
		if msg.Method == "Storage.setCookies" {
			for _, x := range msg.Params["cookies"].([]any) {
				if x.(map[string]any)["name"] == "bad" {
					bad = true
				}
			}
		}
		if bad {
			conn.WriteJSON(map[string]any{"id": msg.Id, "error": map[string]any{"code": -32602, "message": "synthetic invalid cookie"}})
			return
		}
		if msg.Method == "Target.createTarget" {
			opened.Store(true)
		}
		conn.WriteJSON(map[string]any{"id": msg.Id, "result": map[string]any{}})
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	err := restoreAdsPowerSession(dir, port)
	if err == nil || adsPowerRestoreBlocksNavigation(err) || !strings.Contains(err.Error(), "2 个") || !opened.Load() {
		t.Fatal("permanent cookie rejection blocked restored session")
	}
	if _, err = os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("permanent failure left an endless retry")
	}
}
