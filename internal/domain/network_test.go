package domain

import (
	"path/filepath"
	"testing"
	"time"
)

func TestProxyDefaultsAndValidation(t *testing.T) {
	config := DefaultPreferences().Proxy
	if config.Enabled || config.URL != DefaultProxyURL {
		t.Fatal(config)
	}
	for _, value := range []string{"http://127.0.0.1:7897", "https://proxy.example:443", "socks5://[::1]:1080"} {
		got, err := NormalizeProxy(ProxyConfig{Enabled: true, URL: value})
		if err != nil || got.URL != value || !got.Enabled {
			t.Fatal(got, err)
		}
	}
	for _, value := range []string{"ftp://localhost:80", "http://host", "http://host:0", "http://host:65536", "http://host:abc", "http://user:pass@host:80", "http://host:80/a", "http://host:80?", "http://host:80#", "http://[bad:ipv6]:80", "http://:7897", "host:7897"} {
		if _, err := NormalizeProxy(ProxyConfig{URL: value}); err == nil {
			t.Fatal("accepted", value)
		}
	}
}

func TestLocalCatalogPreparationOrderIsStableAcrossOpen(t *testing.T) {
	root := t.TempDir()
	older := time.Unix(100, 0)
	newer := time.Unix(200, 0)
	p := DefaultPreferences()
	p.LocalProjects = []RecentProject{
		{Path: filepath.Join(root, "unknown-first")},
		{Path: filepath.Join(root, "legacy"), LastOpenedAt: older},
		{Path: filepath.Join(root, "unknown-second")},
		{Path: filepath.Join(root, "fresh"), PreparedAt: newer},
	}
	p.Normalize()
	if filepath.Base(p.LocalProjects[0].Path) != "fresh" || filepath.Base(p.LocalProjects[1].Path) != "legacy" || filepath.Base(p.LocalProjects[2].Path) != "unknown-first" {
		t.Fatal(p.LocalProjects)
	}
	p.Remember(RecentProject{Path: filepath.Join(root, "legacy"), LastOpenedAt: time.Unix(300, 0)})
	if filepath.Base(p.LocalProjects[0].Path) != "fresh" || !p.LocalProjects[1].PreparedAt.Equal(older) {
		t.Fatal(p.LocalProjects)
	}
}
