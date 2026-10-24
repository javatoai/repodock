package desktop

import (
	"repodock/internal/application"

	"github.com/egoist/mygo/ui"
)

func (v *View) openSettings(m application.Model) {
	v.gitPath, v.ghPath = m.Preferences.GitPath, m.Preferences.GHPath
	v.SettingsOpen = true
}

func (v *View) settings(c *ui.Context, m application.Model, p palette) {
	ui.DialogBase(c.Key("settings-dialog"), &v.SettingsOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.34))
		panel.Width(500).MaxWidthPercent(92).MaxHeightPercent(90).Padding(24).Gap(18).Radius(18).Background(p.surface).Border(1, p.line)
		ui.Row(c).Children(func() {
			ui.Text(c, "设置").FontSize(20).Bold().Grow(1).TextColor(p.text)
			if iconButton(c, "settings-close", "close", "关闭设置", p, false) {
				v.SettingsOpen = false
			}
		})
		ui.Scroll(c.Key("settings-content")).MaxHeight(420).Gap(17).Children(func() {
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
				v.Backend.ApplyTools(v.gitPath, v.ghPath)
				v.SettingsOpen = false
			}
		})
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
