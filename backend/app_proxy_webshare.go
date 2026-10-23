package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const webshareProxyListURL = "https://proxy.webshare.io/api/v2/proxy/list/"

var webshareProxyListEndpoint = webshareProxyListURL

type webshareProxyRecord struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	ProxyAddress string `json:"proxy_address"`
	Port         int    `json:"port"`
	Valid        bool   `json:"valid"`
	CountryCode  string `json:"country_code"`
	CityName     string `json:"city_name"`
}

type webshareProxyListResponse struct {
	Count   int                   `json:"count"`
	Results []webshareProxyRecord `json:"results"`
}

// BrowserProxyFetchWebshareProxies retrieves direct HTTP proxies. The API key is never persisted.
func (a *App) BrowserProxyFetchWebshareProxies(apiKey string) (map[string]interface{}, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, fmt.Errorf("Webshare API key 不能为空")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 20 * time.Second}
	const pageSize = 100
	all := make([]webshareProxyRecord, 0)
	count := 0
	for page := 1; page <= 10; page++ {
		endpoint, err := url.Parse(webshareProxyListEndpoint)
		if err != nil {
			return nil, fmt.Errorf("Webshare API 地址无效")
		}
		query := endpoint.Query()
		query.Set("mode", "direct")
		query.Set("page", strconv.Itoa(page))
		query.Set("page_size", strconv.Itoa(pageSize))
		endpoint.RawQuery = query.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("创建 Webshare 请求失败")
		}
		request.Header.Set("Authorization", "Token "+apiKey)
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("请求 Webshare 失败")
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
		response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("读取 Webshare 响应失败")
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, fmt.Errorf("Webshare API 返回 HTTP %d", response.StatusCode)
		}
		var payload webshareProxyListResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("解析 Webshare 响应失败")
		}
		if page == 1 {
			count = payload.Count
		}
		all = append(all, payload.Results...)
		if len(payload.Results) == 0 || len(all) >= count {
			break
		}
	}
	return map[string]interface{}{"count": count, "results": all}, nil
}
