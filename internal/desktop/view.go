package desktop

import (
	"fmt"
	"path/filepath"
	"repodock/internal/application"
	"repodock/internal/domain"
	"strings"

	"github.com/egoist/mygo/ui"
)

type Backend interface {
	Snapshot() application.Model
	Refresh()
	BeginClone(string)
	CancelClone()
	ResetTask()
	OpenProject(string, bool)
	RemoveRecent(string)
	SetTheme(string)
	ApplyTools(string, string)
	DismissNotice()
	ReportError(error)
}

type View struct {
	Backend         Backend
	Actions         application.DesktopActions
	Query           string
	SettingsOpen    bool
	gitPath, ghPath string
	showLogs        bool
	logs            ui.ScrollState
	recent          ui.ScrollState
}

func (v *View) Render(c *ui.Context) {
	m := v.Backend.Snapshot()
	p := applyTheme(c, m.Preferences.Theme)
	ui.Column(c).Fill().Children(func() {
		v.header(c, m, p)
		ui.Scroll(c.Key("page")).Grow(1).Padding(20, 28, 28).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(640).AlignSelf(ui.Center).Gap(22).Children(func() {
				if m.Notice != "" {
					ui.Row(c).Gap(10).Padding(10, 14).Radius(10).Background(p.surface).Border(1, p.line).Children(func() {
						icon(c, "warning", 16, p.danger)
						ui.Text(c, shortError(m.Notice)).Grow(1).FontSize(12).TextColor(p.danger).Tooltip(m.Notice)
						if iconButton(c, "dismiss-notice", "close", "关闭提示", p, false) {
							v.Backend.DismissNotice()
						}
					})
				}
				ui.Column(c).Gap(7).Children(func() {
					ui.Text(c, "开始一个项目").FontSize(25).Bold().TextColor(p.text)
					ui.Row(c).Gap(9).Children(func() {
						ui.Text(c, "GitHub").FontSize(12).TextColor(p.muted)
						icon(c, "arrow", 12, p.muted)
						ui.Text(c, "本地").FontSize(12).TextColor(p.muted)
						icon(c, "arrow", 12, p.muted)
						ui.Text(c, "Codex").FontSize(12).TextColor(p.muted)
					})
				})
				v.cloneCard(c, m, p)
				v.recentProjects(c, m, p)
			})
		})
		ui.Row(c).Padding(10, 28).Children(func() {
			ui.Text(c, "原生 · 本地优先").FontSize(11).TextColor(p.muted)
			ui.Spacer(c)
			ui.Text(c, "RepoDock  /  0.1.0").FontSize(11).TextColor(p.muted)
		})
	})
	v.settings(c, m, p)
}

func (v *View) header(c *ui.Context, m application.Model, p palette) {
	ui.Row(c).Padding(18, 28).Gap(11).Children(func() {
		ui.Box(c).Size(32, 32).Radius(10).Background(ui.Hex("#356AE6")).Center().Children(func() { icon(c, "dock", 21, ui.Hex("#FFFFFF")) })
		ui.Text(c, "RepoDock").FontSize(16).Bold().TextColor(p.text)
		ui.Spacer(c)
		account := "连接 GitHub"
		if m.Account != "" {
			account = m.Account
		}
		accountButton := ui.ButtonBase(c.Key("account")).Row().Gap(7).Padding(7, 10).Radius(20).Label("GitHub 账户").Tooltip("查看账户与连接设置")
		if accountButton.Hovered() {
			accountButton.Background(p.surface)
		}
		accountButton.Children(func() {
			if m.Loading {
				ui.Spinner(c).Size(14, 14)
			} else {
				icon(c, "github", 16, p.muted)
			}
			ui.Text(c, account).FontSize(12).TextColor(p.text).NoWrap()
			if m.Account != "" {
				ui.Box(c).Size(5, 5).Radius(3).Background(p.success)
			}
		})
		if accountButton.Clicked() {
			v.openSettings(m)
		}
		if iconButton(c, "settings", "settings", "设置", p, false) {
			v.openSettings(m)
		}
	})
}

func (v *View) cloneCard(c *ui.Context, m application.Model, p palette) {
	ui.Column(c.Key("clone-card")).Padding(24).Gap(17).Radius(18).Background(p.surface).Border(1, p.line).Children(func() {
		if m.Task.Status == "success" {
			v.success(c, m, p)
			return
		}
		ui.Row(c).Gap(8).Children(func() {
			icon(c, "repo", 16, p.accent)
			ui.Text(c, "GitHub 仓库").FontSize(13).Bold().TextColor(p.text)
		})
		v.repositoryInput(c, m, p)
		ui.Row(c).Gap(7).MinHeight(22).Children(func() {
			status := fmt.Sprintf("%d 个私人仓库", len(m.Repositories))
			if m.Loading {
				status = "正在读取私人仓库…"
			} else if m.AccountError != "" {
				status = "仓库未加载完整 · 点击重试"
				if m.Account == "" {
					status = "连接 GitHub 后可选择私人仓库"
				}
			} else if len(m.Repositories) == 0 && m.Complete {
				status = "暂无本人拥有的私人仓库"
			}
			icon(c, "lock", 12, p.muted)
			ui.Text(c, status).FontSize(11).TextColor(p.muted).Grow(1).Tooltip(m.AccountError)
			if iconButton(c, "refresh-repos", "refresh", "刷新仓库", p, m.Loading) {
				v.Backend.Refresh()
			}
		})
		if m.Task.Busy() {
			v.progress(c, m, p)
		} else {
			if m.Task.Error != "" {
				ui.Text(c, shortError(m.Task.Error)).FontSize(12).TextColor(p.danger).Tooltip(m.Task.Error)
			}
			if m.Task.Status == "cancelled" {
				ui.Text(c, "克隆已取消").FontSize(12).TextColor(p.muted)
			}
			ui.Row(c).Gap(12).Children(func() {
				if m.Preferences.LastParent != "" {
					icon(c, "folder", 15, p.muted)
					ui.Text(c, displayPath(m.Preferences.LastParent)).Grow(1).FontSize(11).TextColor(p.muted).SingleLine().Tooltip(m.Preferences.LastParent)
				} else {
					ui.Spacer(c)
				}
				label := "选择位置并克隆"
				if m.Task.Status == "error" {
					label = "重新选择位置"
				}
				_, parseErr := domain.ParseRepository(v.Query)
				disabled := parseErr != nil || !m.DependenciesReady || m.Dependencies.GitErr != nil || m.Dependencies.GHErr != nil
				if button(c, "clone", "arrow", label, p, true, disabled) {
					v.showLogs = false
					v.Backend.BeginClone(v.Query)
				}
			})
			if m.DependenciesReady && (m.Dependencies.GitErr != nil || m.Dependencies.GHErr != nil) {
				if ui.Button(c.Key("configure-tools"), "配置 Git 与 GitHub CLI").Clicked() {
					v.openSettings(m)
				}
			}
		}
		v.taskLogs(c, m, p)
	})
}

func (v *View) repositoryInput(c *ui.Context, m application.Model, p palette) {
	var cb ui.ComboboxParts
	ui.Row(c.Key("repository-field")).Height(52).Gap(10).Padding(0, 12).Radius(11).Background(p.soft).Border(1, p.line).Children(func() {
		icon(c, "github", 19, p.muted)
		cb = ui.ComboboxBase(c.Key("repository-input"), &v.Query)
		cb.Input.Grow(1).Height(50).FontSize(13).Label("仓库地址或名称").Placeholder("粘贴 GitHub 地址，或搜索私人仓库").Disabled(m.Task.Busy())
		if cb.Input.Changed() && !m.Task.Busy() && m.Task.Status != "idle" {
			v.Backend.ResetTask()
		}
		arrow := ui.ButtonBase(c.Key("repository-expand")).Size(26, 30).KeepFocus().Center().Label("展开私人仓库").Tooltip("选择私人仓库").Disabled(m.Task.Busy())
		arrow.Children(func() { icon(c, "chevron", 16, p.muted) })
		if arrow.Clicked() {
			cb.Input.Focus()
			cb.SetOpen(!cb.Open())
		}
	})
	cb.Popup(func(panel ui.Element) {
		width, _ := c.Size()
		panel.Width(min(560, width-120)).Margin(7, 0, 0).Padding(6).Radius(12).Background(p.surface).Border(1, p.line).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.12))
		ui.Scroll(c.Key("repository-options")).MaxHeight(240).Gap(2).Children(func() {
			for _, repo := range m.Repositories {
				if cb.Filtering() && !strings.Contains(strings.ToLower(repo.Ref.FullName()), strings.ToLower(strings.TrimSpace(v.Query))) {
					continue
				}
				item := cb.Item(repo.Ref.URL()).Padding(11, 12).Gap(10).Radius(7).Label(repo.Ref.FullName())
				if item.Highlighted() {
					item.Background(p.accentSoft)
				}
				item.Children(func() {
					icon(c, "lock", 14, p.muted)
					ui.Text(c, repo.Ref.FullName()).FontSize(13).TextColor(p.text).SingleLine()
				})
			}
		})
	})
	if chosen, ok := cb.Chosen(); ok {
		v.Query = chosen
		if !m.Task.Busy() && m.Task.Status != "idle" {
			v.Backend.ResetTask()
		}
	}
}

func (v *View) progress(c *ui.Context, m application.Model, p palette) {
	stage := m.Task.Progress.Stage
	if stage == "" {
		stage = "准备克隆"
	}
	if m.Task.Status == "picking" {
		stage = "选择保存位置…"
	}
	if m.Task.Status == "cancelling" {
		stage = "正在取消并清理…"
	}
	ui.Column(c).Gap(10).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, stage).FontSize(12).TextColor(p.text).Grow(1)
			if m.Task.Progress.Percent >= 0 && m.Task.Progress.Stage != "" {
				ui.Text(c, fmt.Sprintf("%d%%", m.Task.Progress.Percent)).FontSize(12).TextColor(p.muted)
			}
			if m.Task.Status != "picking" && ui.Button(c.Key("cancel-clone"), "取消").Disabled(m.Task.Status == "cancelling").Clicked() {
				v.Backend.CancelClone()
			}
		})
		value := float64(-1)
		if m.Task.Progress.Percent >= 0 && m.Task.Progress.Stage != "" {
			value = float64(m.Task.Progress.Percent) / 100
		}
		ui.Progress(c, value).Height(4).Label("克隆进度")
	})
}

func (v *View) success(c *ui.Context, m application.Model, p palette) {
	ui.Row(c).Gap(14).Children(func() {
		ui.Box(c).Size(44, 44).Radius(22).Background(p.soft).Center().Children(func() { icon(c, "check", 24, p.success) })
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "项目已就绪").FontSize(18).Bold().TextColor(p.text)
			ui.Text(c, m.Task.Repository.FullName()).FontSize(12).TextColor(p.muted)
		})
	})
	ui.Row(c).Gap(8).Padding(13).Radius(10).Background(p.soft).Children(func() {
		icon(c, "folder", 16, p.muted)
		ui.Text(c, displayPath(m.Task.Path)).FontSize(12).Grow(1).SingleLine().TextColor(p.muted).Tooltip(m.Task.Path)
		if iconButton(c, "success-folder", "launch", "打开项目文件夹", p, false) {
			v.Backend.OpenProject(m.Task.Path, true)
		}
	})
	ui.Row(c).Gap(12).Justify(ui.SpaceBetween).Children(func() {
		if ui.Button(c.Key("another-project"), "克隆另一个").Clicked() {
			v.Query = ""
			v.Backend.ResetTask()
		}
		if button(c, "open-codex", "launch", "使用 Codex 打开", p, true, false) {
			v.Backend.OpenProject(m.Task.Path, false)
		}
	})
}

func (v *View) taskLogs(c *ui.Context, m application.Model, p palette) {
	if len(m.Task.Logs) == 0 && m.Task.Error == "" {
		return
	}
	if ui.Button(c.Key("toggle-logs"), "查看详情").Clicked() {
		v.showLogs = !v.showLogs
	}
	if !v.showLogs {
		return
	}
	ui.Scroll(c.Key("logs")).Height(110).TrackScroll(&v.logs).Padding(10).Gap(4).Background(p.soft).Radius(8).Children(func() {
		for _, line := range m.Task.Logs {
			ui.Text(c, line).FontSize(11).TextColor(p.muted).Selectable()
		}
		if m.Task.Error != "" {
			ui.Text(c, m.Task.Error).FontSize(11).TextColor(p.danger).Selectable()
		}
	})
}

func (v *View) recentProjects(c *ui.Context, m application.Model, p palette) {
	ui.Column(c.Key("recent")).Gap(13).Children(func() {
		ui.Row(c).Gap(7).Padding(0, 2).Children(func() {
			icon(c, "clock", 14, p.muted)
			ui.Text(c, "最近项目").FontSize(12).Bold().TextColor(p.muted)
			ui.Spacer(c)
			if len(m.Preferences.Recent) > 0 {
				ui.Text(c, fmt.Sprintf("%02d", len(m.Preferences.Recent))).FontSize(11).TextColor(p.muted)
			}
		})
		if len(m.Preferences.Recent) == 0 {
			ui.Column(c).Padding(24).Gap(7).Center().Children(func() {
				icon(c, "folder", 25, p.line)
				ui.Text(c, "克隆的项目会保留在这里").FontSize(12).TextColor(p.muted)
			})
			return
		}
		_, windowHeight := c.Size()
		visibleRows := min(len(m.Preferences.Recent), min(4, max(2, int((windowHeight-480)/66))))
		ui.Scroll(c.Key("recent-list")).Height(float32(visibleRows*66 - 4)).TrackScroll(&v.recent).Gap(4).Children(func() {
			for _, project := range m.Preferences.Recent {
				available := m.Available[project.Path]
				ui.Row(c.Key(project.Path)).Height(62).Shrink(0).Padding(11, 12).Gap(12).Radius(11).Background(p.surface).Children(func() {
					ui.Box(c).Size(34, 34).Radius(9).Background(p.soft).Center().Children(func() { icon(c, "repo", 17, p.muted) })
					ui.Column(c).Grow(1).Gap(5).Children(func() {
						color := p.text
						if !available {
							color = p.muted
						}
						ui.Text(c, project.FullName).FontSize(13).Bold().TextColor(color).SingleLine()
						path := displayPath(project.Path)
						if !available {
							path = "目录不可用 · " + path
						}
						ui.Text(c, path).FontSize(11).TextColor(p.muted).SingleLine().Tooltip(project.Path)
					})
					if iconButton(c, "recent-open", "launch", "用 Codex 打开 "+project.FullName, p, !available) {
						v.Backend.OpenProject(project.Path, false)
					}
					if iconButton(c, "recent-folder", "folder", "打开文件夹 "+project.FullName, p, !available) {
						v.Backend.OpenProject(project.Path, true)
					}
					if iconButton(c, "recent-remove", "trash", "移除记录 "+project.FullName, p, false) {
						v.Backend.RemoveRecent(project.Path)
					}
				})
			}
		})
	})
}

func shortError(s string) string {
	first := strings.Split(strings.TrimSpace(s), "\n")[0]
	runes := []rune(first)
	if len(runes) > 90 {
		return string(runes[:90]) + "…"
	}
	return first
}

func displayPath(path string) string {
	return filepath.Clean(path)
}
