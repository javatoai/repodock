package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"repodock/internal/domain"
	"time"
)

type ProxyTester struct {
	// Endpoint and Transport are injectable for local integration tests.
	Endpoint  string
	Transport *http.Transport
}

func (t ProxyTester) Test(ctx context.Context, config domain.ProxyConfig) error {
	config, err := domain.NormalizeProxy(config)
	if err != nil {
		return err
	}
	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = "https://api.github.com/rate_limit"
	}
	proxy, _ := url.Parse(config.URL)
	base := t.Transport
	if base == nil {
		base = http.DefaultTransport.(*http.Transport)
	}
	transport := base.Clone()
	transport.Proxy = http.ProxyURL(proxy)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "RepoDock")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("代理连接失败（%s）：%w", config.URL, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GitHub API 返回 HTTP %d（代理 %s）", response.StatusCode, config.URL)
	}
	var payload struct {
		Resources map[string]struct{ Limit int }
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return fmt.Errorf("代理未返回有效的 GitHub API 响应：%w", err)
	}
	if payload.Resources["core"].Limit <= 0 {
		return fmt.Errorf("代理未返回有效的 GitHub API 响应")
	}
	return nil
}
