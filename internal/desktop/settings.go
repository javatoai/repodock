package desktop

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"repodock/internal/application"
	"repodock/internal/domain"
	"strings"

	"github.com/egoist/mygo/ui"
)

func (v *View) openSettings(m application.Model) {
	if !v.settingsInitialized {
		v.gitPath, v.ghPath = m.Preferences.GitPath, m.Preferences.GHPath
		v.defaultDir = m.Preferences.DefaultCloneDir
		v.proxyEnabled = m.Preferences.Proxy.Enabled
		proxy, err := domain.NormalizeProxy(m.Preferences.Proxy)
		if err != nil {
			proxy = domain.DefaultProxy()
		}
		u, _ := url.Parse(proxy.URL)
		v.proxyHost, v.proxyPort = u.Hostname(), u.Port()
		switch u.Scheme {
		case "https":
			v.proxyProtocol = 1
		case "socks5":
			v.proxyProtocol = 2
		}
		v.settingsInitialized = true
	}
	v.Page = "settings"
}

func (v *View) settingsPage(c *ui.Context, m application.Model, p palette) {
	ui.Column(c.Key("settings-page")).Grow(1).FillWidth().MaxWidth(680).AlignSelf(ui.Center).Gap(18).Children(func() {
		ui.Text(c, "设置").FontSize(20).Bold().TextColor(p.text)
		ui.Scroll(c.Key("settings-content")).Grow(1).TrackScroll(&v.settingsScroll).Gap(17).Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, "默认项目目录").FontSize(13).Bold().TextColor(p.text)
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c.Key("default-project-dir"), &v.defaultDir).Grow(1).Label("默认项目目录").FontSize(12).Disabled(m.Task.Busy())
					if ui.Button(c.Key("choose-default-dir"), "选择").Disabled(m.Task.Busy() || v.picking).Clicked() {
						v.chooseDirectory(v.defaultDir, func(path string) { v.defaultDir = path })
					}
				})
			})
			ui.Divider(c)
			ui.Column(c).Gap(9).Children(func() {
				ui.Text(c, "外观").FontSize(12).Bold().TextColor(p.muted)
				index := 0
				if m.Preferences.Theme == "light" {
					index = 1
				} else if m.Preferences.Theme == "dark" {
					index = 2
				}
				if ui.Segmented(c.Key("theme"), &index, "跟随系统", "浅色", "深色").Changed() {
					v.Backend.SetTheme([]string{"system", "light", "dark"}[index])
				}
			})
			ui.Divider(c)
			v.proxySettings(c, m, p)
			ui.Divider(c)
			v.toolSetting(c, "Git", "Git 可执行文件", &v.gitPath, m.Dependencies.GitPath, m.Dependencies.GitErr, "https://git-scm.com/downloads", p, m.Task.Busy())
			v.toolSetting(c, "GitHub CLI", "GitHub CLI 可执行文件", &v.ghPath, m.Dependencies.GHPath, m.Dependencies.GHErr, "https://cli.github.com/", p, m.Task.Busy())
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, "Codex 桌面应用").FontSize(13).TextColor(p.text).Grow(1)
				if m.CodexChecked {
					if m.CodexError == "" {
						icon(c, "check", 13, p.success)
					} else {
						ui.Text(c, "未检测到").FontSize(11).TextColor(p.muted).Tooltip(m.CodexError)
					}
				}
				if ui.Button(c.Key("codex-install"), "安装").Clicked() {
					v.openURL("https://chatgpt.com/codex")
				}
			})
			ui.Divider(c)
			ui.Row(c).Gap(8).Children(func() {
				icon(c, "github", 17, p.muted)
				account := "尚未连接 GitHub"
				if m.Account != "" {
					account = "已连接 · " + m.Account
				}
				ui.Text(c, account).FontSize(12).Grow(1).TextColor(p.text)
				if ui.Button(c.Key("settings-refresh"), "重新检测").Disabled(m.Loading).Clicked() {
					v.Backend.Refresh()
				}
			})
			if m.AccountError != "" {
				ui.Text(c, shortError(m.AccountError)).FontSize(11).TextColor(p.danger).Tooltip(m.AccountError)
			}
			if m.Account == "" {
				ui.Text(c, "在终端运行一次，即可连接本机账户").FontSize(11).TextColor(p.muted)
				ui.Row(c).Padding(10).Radius(8).Background(p.soft).Gap(8).Children(func() {
					ui.Text(c, "gh auth login --hostname github.com --web").FontSize(11).Grow(1).TextColor(p.text).Selectable()
					if iconButton(c, "copy-login", "copy", "复制登录命令", p, false) {
						c.WriteClipboard("gh auth login --hostname github.com --web")
					}
				})
			}
		})
		ui.Row(c).Justify(ui.End).Children(func() {
			if button(c, "save-settings", "check", "保存并检测", p, true, m.Task.Busy()) {
				if !filepath.IsAbs(strings.TrimSpace(v.defaultDir)) {
					v.Backend.ReportError(fmt.Errorf("默认项目目录必须是绝对路径"))
					return
				}
				proxy, err := domain.NormalizeProxy(v.proxyDraft())
				if err != nil {
					v.Backend.ReportError(err)
					return
				}
				v.Backend.ApplySettings(v.gitPath, v.ghPath, v.defaultDir, proxy)
			}
		})
	})
}

func (v *View) proxyDraft() domain.ProxyConfig {
	return domain.ProxyConfig{Enabled: v.proxyEnabled, URL: []string{"http", "https", "socks5"}[v.proxyProtocol] + "://" + net.JoinHostPort(strings.TrimSpace(v.proxyHost), strings.TrimSpace(v.proxyPort))}
}

func (v *View) proxySettings(c *ui.Context, m application.Model, p palette) {
	busy := m.Task.Busy()
	ui.Column(c.Key("proxy-settings")).Gap(10).Children(func() {
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, "网络代理").FontSize(13).Bold().TextColor(p.text).Grow(1)
			ui.Switch(c.Key("proxy-enabled"), &v.proxyEnabled).Label("启用网络代理").Disabled(busy)
		})
		ui.Segmented(c.Key("proxy-protocol"), &v.proxyProtocol, "HTTP", "HTTPS", "SOCKS5").Label("代理协议").Disabled(busy)
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c.Key("proxy-host"), &v.proxyHost).Grow(1).Label("代理主机").Placeholder("127.0.0.1").FontSize(12).Disabled(busy)
			ui.TextInput(c.Key("proxy-port"), &v.proxyPort).Width(80).Label("代理端口").Placeholder("7897").FontSize(12).Disabled(busy)
			if ui.Button(c.Key("proxy-test"), "测试连接").Disabled(busy || m.ProxyTest.Loading).Clicked() {
				v.Backend.TestProxy(v.proxyDraft())
			}
		})
		draft, err := domain.NormalizeProxy(v.proxyDraft())
		if err == nil && m.ProxyTest.Config == draft {
			message, color := m.ProxyTest.Message, p.success
			if m.ProxyTest.Loading {
				message, color = "正在连接 GitHub API…", p.muted
			}
			if m.ProxyTest.Error != "" {
				message, color = shortError(m.ProxyTest.Error), p.danger
			}
			if message != "" {
				ui.Text(c, message).FontSize(11).TextColor(color).Tooltip(m.ProxyTest.Error)
			}
		}
	})
}

func (v *View) toolSetting(c *ui.Context, title, label string, value *string, resolved string, err error, installURL string, p palette, disabled bool) {
	ui.Column(c.Key(label)).Gap(7).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, title).FontSize(13).Bold().TextColor(p.text).Grow(1)
			if err != nil {
				icon(c, "warning", 13, p.danger)
			} else if resolved != "" {
				icon(c, "check", 13, p.success)
			}
			if ui.Button(c.Key("install"), "安装").Clicked() {
				v.openURL(installURL)
			}
		})
		placeholder := "自动检测"
		if resolved != "" {
			placeholder = resolved
		}
		ui.TextInput(c.Key("path"), value).Placeholder(placeholder).Label(label).Disabled(disabled).FontSize(12)
		if err != nil {
			ui.Text(c, shortError(err.Error())).FontSize(11).TextColor(p.danger)
		}
	})
}

func (v *View) openURL(url string) {
	if v.Actions != nil {
		go func() { v.Backend.ReportError(v.Actions.OpenURL(url)) }()
	}
}
