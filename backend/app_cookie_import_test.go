package backend

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"ant-chrome/backend/internal/browser"
	"github.com/gorilla/websocket"
)

func TestParseImportedCookiesFormatsAndValidation(t *testing.T) {
	for _, content := range []string{
		`[{"name":"session","value":"synthetic","domain":".example.test","path":"/","expirationDate":2000000000,"sameSite":"no_restriction","secure":true,"httpOnly":true}]`,
		"# Netscape HTTP Cookie File\n#HttpOnly_.example.test\tTRUE\t/\tTRUE\t2000000000\tsession\tsynthetic\n",
	} {
		cookies, err := parseImportedCookies(content)
		if err != nil || len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].Expires != 2000000000 {
			t.Fatalf("parse failed: %v", err)
		}
	}
	for _, content := range []string{`[]`, `[{"domain":"example.test","name":"session","sameSite":"None","secure":false}]`, `[{"name":"x","value":"synthetic"}]`, `[{"domain":"example.test","name":"bad\nname","value":"synthetic"}]`, `[{"domain":"example.test","name":"x","sameSite":"invalid"}]`, `{broken synthetic-secret`, strings.Repeat("x", 2*1024*1024+1)} {
		if _, err := parseImportedCookies(content); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("invalid import accepted or leaked contents")
		}
	}
}

func TestBrowserImportCookiesUsesValidatedCDPCommand(t *testing.T) {
	var command cdpMessage
	var server *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"type":"page","webSocketDebuggerUrl":"ws%s/ws"}]`, strings.TrimPrefix(server.URL, "http"))
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if err = conn.ReadJSON(&command); err != nil {
			return
		}
		// Notifications are not replies to the command.
		conn.WriteJSON(map[string]any{"method": "Network.notification"})
		result := map[string]any{}
		if command.Method == "Network.getAllCookies" {
			result["cookies"] = []CookieInfo{{Name: "session", Value: "synthetic", Domain: ".example.test", Path: "/", HttpOnly: true, Secure: true}}
		}
		conn.WriteJSON(map[string]any{"id": command.Id, "result": result})
	})
	server = httptest.NewServer(mux)
	defer server.Close()
	_, portText, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	port, _ := strconv.Atoi(portText)
	app := &App{browserMgr: &browser.Manager{Profiles: map[string]*browser.Profile{"p": {Running: true, DebugReady: true, DebugPort: port}}}}
	count, err := app.BrowserImportCookies("p", `[{"name":"session","value":"synthetic","domain":"example.test","expires":-1}]`)
	if err != nil || count != 1 || command.Method != "Network.setCookies" {
		t.Fatalf("import command failed: %v", err)
	}
	cookies := command.Params["cookies"].([]any)
	if _, exists := cookies[0].(map[string]any)["expires"]; exists {
		t.Fatal("session cookies must not receive past expiry")
	}
	exported, err := app.BrowserExportCookies("p")
	if err != nil || !strings.Contains(exported, "#HttpOnly_.example.test") {
		t.Fatal("export lost HttpOnly flag")
	}

}

func TestNetscapeImportKeepsEmptyCookieValue(t *testing.T) {
	cookies, err := parseImportedCookies("example.test\tFALSE\t/\tFALSE\t0\tempty\t\n")
	if err != nil || len(cookies) != 1 || cookies[0].Value != "" {
		t.Fatal("empty cookie value should round-trip")
	}
}
