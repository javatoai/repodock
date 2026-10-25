package domain

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const DefaultProxyURL = "http://127.0.0.1:7897"

type ProxyConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

func DefaultProxy() ProxyConfig { return ProxyConfig{URL: DefaultProxyURL} }

func NormalizeProxy(config ProxyConfig) (ProxyConfig, error) {
	value := strings.TrimSpace(config.URL)
	if value == "" {
		value = DefaultProxyURL
	}
	u, err := url.Parse(value)
	if err != nil {
		return config, fmt.Errorf("代理地址格式无效")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" && scheme != "socks5" {
		return config, fmt.Errorf("代理仅支持 HTTP、HTTPS 或 SOCKS5")
	}
	if u.Opaque != "" || u.User != nil || strings.ContainsAny(value, "?#") || u.RawPath != "" || (u.Path != "" && u.Path != "/") {
		return config, fmt.Errorf("代理地址只能包含协议、主机和端口")
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if host == "" || strings.ContainsAny(host, " \t\r\n/\\[]") || (strings.Contains(host, ":") && net.ParseIP(host) == nil) || err != nil || port < 1 || port > 65535 {
		return config, fmt.Errorf("请填写有效代理主机和 1–65535 范围内的端口")
	}
	config.URL = scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port))
	return config, nil
}

type CloneTarget struct {
	Repository    RepositoryRef
	Parent        string
	Path          string
	State         string // checking, ready, exists, file, error
	Message       string
	OpenDirectory string
}
