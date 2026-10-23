package backend

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserProxyFetchWebshareProxiesUsesTokenAndPaginates(t *testing.T) {
	oldEndpoint := webshareProxyListEndpoint
	t.Cleanup(func() { webshareProxyListEndpoint = oldEndpoint })
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Token secret-key" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		if request.URL.Query().Get("mode") != "direct" || request.URL.Query().Get("page_size") != "100" {
			t.Fatalf("unexpected query: %s", request.URL.RawQuery)
		}
		if request.URL.Query().Get("page") == "1" {
			fmt.Fprint(writer, `{"count":2,"results":[{"id":"d-1","username":"u","password":"p","proxy_address":"1.2.3.4","port":8080,"valid":true}]}`)
			return
		}
		fmt.Fprint(writer, `{"count":2,"results":[{"id":"d-2","username":"u2","password":"p2","proxy_address":"5.6.7.8","port":8081,"valid":true}]}`)
	}))
	defer server.Close()
	webshareProxyListEndpoint = server.URL

	result, err := (&App{}).BrowserProxyFetchWebshareProxies("secret-key")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if got := len(result["results"].([]webshareProxyRecord)); got != 2 {
		t.Fatalf("results = %d, want 2", got)
	}
}

func TestBrowserProxyFetchWebshareProxiesRejectsMissingKey(t *testing.T) {
	_, err := (&App{}).BrowserProxyFetchWebshareProxies(" \t")
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("error = %v", err)
	}
}
