package desktop

import (
	"fmt"
	"path/filepath"
	"repodock/internal/application"
	"repodock/internal/domain"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type Backend interface {
	Snapshot() application.Model
	Refresh()
	BeginClone(string, ...string)
	BeginPublish(domain.PublishRequest)
	PreviewPublication(domain.PublishRequest)
	EndPublication()
	CancelClone()
	ResetTask()
	OpenProject(string, bool)
	RemoveLocalProject(string)
	CheckCloneTarget(string, string)
	TestProxy(domain.ProxyConfig)
	SetTheme(string)
	ApplySettings(string, string, string, ...domain.ProxyConfig)
	DismissNotice()
	ReportError(error)
}

type View struct {
	Backend                                  Backend
	Actions                                  application.DesktopActions
	Query, Page, SelectedURL, CloneParent    string
	ClonedQuery, HighlightPath               string
	Creation                                 bool
	CreateMode, Visibility                   int
	ProjectName, ProjectParent, ExistingPath string
	settingsInitialized                      bool
	gitPath, ghPath, defaultDir              string
	proxyEnabled                             bool
	proxyProtocol                            int
	proxyHost, proxyPort                     string
	targetKey                                string
	showLogs, showFiles, picking, reveal     bool
	openSequence                             uint64
	contentWidth                             float32
	logs, cloned, home, settingsScroll       ui.ScrollState
}

func (v *View) Render(c *ui.Context) {
	m := v.Backend.Snapshot()
	p := applyTheme(c, m.Preferences.Theme)
	if m.OpenSequence != v.openSequence {
		v.openSequence = m.OpenSequence
		v.gotoCloned(m.OpenedPath)
	}
	width, _ := c.Size()
	navWidth := float32(180)
	compact := width < 760
	if compact {
		navWidth = 64
	}
	v.contentWidth = min(float32(560), width-navWidth-48)
	ui.Row(c).Fill().Background(p.surface).Children(func() {
		v.navigation(c, p, navWidth, compact, m)
		ui.Column(c.Key("workspace")).Grow(1).FillHeight().Padding(24).Gap(16).Children(func() {
			if m.Notice != "" {
				ui.Row(c.Key("notice")).Gap(8).Padding(10, 12).Radius(8).Background(p.soft).Children(func() {
					icon(c, "warning", 16, p.danger)
					ui.Text(c, shortError(m.Notice)).Grow(1).FontSize(12).TextColor(p.danger).Tooltip(m.Notice)
					if iconButton(c, "dismiss-notice", "close", "关闭提示", p, false) {
						v.Backend.DismissNotice()
					}
				})
			}
			switch v.Page {
			case "cloned":
				v.clonedPage(c, m, p)
			case "settings":
				v.settingsPage(c, m, p)
			default:
				v.homePage(c, m, p)
			}
			v.taskBar(c, m, p)
		})
	})
}

func (v *View) navigation(c *ui.Context, p palette, width float32, compact bool, m application.Model) {
	ui.Column(c.Key("navigation")).Width(width).FillHeight().Padding(20, 12).Gap(8).Background(p.background).Children(func() {
		ui.Row(c).Height(42).Padding(0, 6).Gap(10).Children(func() {
			ui.Box(c).Size(28, 28).Radius(8).Center().Background(ui.Hex("#356AE6")).Children(func() { icon(c, "dock", 19, ui.Hex("#FFFFFF")) })
			if !compact {
				ui.Text(c, "RepoDock").FontSize(15).Bold().TextColor(p.text)
			}
		})
		ui.Box(c).Height(22)
		if v.navButton(c, "home", "首页", "home", v.Page == "" || v.Page == "home", compact, p) {
			v.Page = "home"
			v.Creation = false
		}
		if v.navButton(c, "cloned", "已克隆", "folder", v.Page == "cloned", compact, p) {
			v.Page = "cloned"
		}
		ui.Spacer(c)
		if v.navButton(c, "settings", "设置", "settings", v.Page == "settings", compact, p) {
			v.openSettings(m)
		}
	})
}

func (v *View) navButton(c *ui.Context, key, label, glyph string, selected, compact bool, p palette) bool {
	b := ui.ButtonBase(c.Key("nav-"+key)).Row().Height(42).Gap(10).Padding(0, 12).Radius(8).Label(label).Tooltip(label).Selected(selected)
	fg := p.muted
	if selected {
		b.Background(p.accentSoft)
		fg = p.accent
	} else if b.Hovered() {
		b.Background(p.soft)
		fg = p.text
	}
	if compact {
		b.Padding(0).Center()
	}
	b.Children(func() {
		icon(c, glyph, 18, fg)
		if !compact {
			ui.Text(c, label).FontSize(13).TextColor(fg)
		}
	})
	return b.Clicked()
}

func (v *View) homePage(c *ui.Context, m application.Model, p palette) {
	ui.Scroll(c.Key("home-page")).Grow(1).TrackScroll(&v.home).Children(func() {
		top := float32(92)
		if v.Creation || v.SelectedURL != "" || m.Task.Status != "idle" {
			top = 20
		}
		ui.Column(c).FillWidth().MaxWidth(560).AlignSelf(ui.Center).Margin(top, 0, 24).Gap(20).Children(func() {
			if v.Creation {
				v.createForm(c, m, p)
				return
			}
			v.repositoryInput(c, m, p)
			if v.SelectedURL == "" {
				if ref, err := domain.ParseRepository(v.Query); err == nil {
					v.SelectedURL = ref.URL()
				}
			}
			if v.SelectedURL != "" {
				v.cloneForm(c, m, p)
			} else {
				ui.Row(c).Justify(ui.End).Children(func() {
					if ui.Button(c.Key("create-project"), "创建项目").Disabled(m.Task.Busy()).Clicked() {
						v.startCreation(m)
					}
				})
			}
		})
	})
}

func (v *View) repositoryInput(c *ui.Context, m application.Model, p palette) {
	var cb ui.ComboboxParts
	field := ui.Row(c.Key("search-field")).Height(52).Padding(0, 12).Gap(10).Radius(10).Background(p.soft).Border(1, p.line)
	field.Children(func() {
		icon(c, "search", 19, p.muted)
		cb = ui.ComboboxBase(c.Key("repository-input"), &v.Query)
		cb.Input.Grow(1).Height(50).FontSize(14).Label("搜索 GitHub 项目").Placeholder("搜索 GitHub 项目，或粘贴地址").Disabled(m.Task.Busy())
		if cb.Input.Changed() {
			v.SelectedURL, v.CloneParent = "", ""
		}
		expand := ui.ButtonBase(c.Key("repository-expand")).Size(26, 32).KeepFocus().Center().Label("展开 GitHub 项目").Tooltip("选择项目").Disabled(m.Task.Busy())
		expand.Children(func() { icon(c, "chevron", 16, p.muted) })
		if expand.Clicked() {
			cb.Input.Focus()
			cb.SetOpen(!cb.Open())
		}
	})
	cb.Popup(func(panel ui.Element) {
		panel.AttachTo(field, ui.AnchorBottomLeft, ui.AnchorTopLeft).Width(v.contentWidth).Margin(6, 0, 0).Padding(6).Radius(10).Background(p.surface).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.14))
		ui.Scroll(c.Key("repository-options")).MaxHeight(280).Gap(2).Children(func() {
			count := 0
			query := strings.ToLower(strings.TrimSpace(v.Query))
			parsed, parseErr := domain.ParseRepository(v.Query)
			for _, repo := range m.Repositories {
				matches := strings.Contains(strings.ToLower(repo.Ref.FullName()), query)
				if parseErr == nil {
					matches = strings.EqualFold(repo.Ref.FullName(), parsed.FullName())
				}
				if cb.Filtering() && !matches {
					continue
				}
				v.repositoryOption(c, cb, repo, p)
				count++
			}
			if ref, err := domain.ParseRepository(v.Query); err == nil {
				found := false
				for _, repo := range m.Repositories {
					if strings.EqualFold(repo.Ref.FullName(), ref.FullName()) {
						found = true
						break
					}
				}
				if !found {
					v.repositoryOption(c, cb, domain.Repository{Ref: ref}, p)
					count++
				}
			}
			if count == 0 {
				status := "没有匹配的项目"
				color := p.muted
				if m.Loading {
					status = "正在加载…"
				}
				if m.AccountError != "" {
					status, color = shortError(m.AccountError), p.danger
				}
				cb.Item("").Disabled(true).Label(status).Padding(14, 12).Children(func() { ui.Text(c, status).FontSize(12).TextColor(color).Tooltip(m.AccountError) })
			}
			if count > 0 && m.Loading {
				ui.Text(c, "正在加载…").Padding(10, 12).FontSize(12).TextColor(p.muted)
			}
			if count > 0 && m.AccountError != "" {
				ui.Text(c, shortError(m.AccountError)).Padding(8, 12).FontSize(12).TextColor(p.danger).Tooltip(m.AccountError)
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.Button(c.Key("refresh-repos"), "刷新").Disabled(m.Loading).Clicked() {
					v.Backend.Refresh()
				}
				if m.Account == "" && ui.Button(c.Key("connect-github"), "连接 GitHub").Clicked() {
					v.openSettings(m)
				}
			})
		})
	})
	if chosen, ok := cb.Chosen(); ok {
		v.selectRepository(chosen, m)
	}
}

func (v *View) repositoryOption(c *ui.Context, cb ui.ComboboxParts, repo domain.Repository, p palette) {
	item := cb.Item(repo.Ref.URL()).Row().Height(44).Padding(0, 12).Gap(9).Radius(7).Label(repo.Ref.FullName())
	if item.Highlighted() {
		item.Background(p.accentSoft)
	}
	item.Children(func() {
		if repo.Private {
			icon(c, "lock", 14, p.muted)
		} else {
			ui.Box(c).Size(14, 14)
		}
		ui.Text(c, repo.Ref.FullName()).Grow(1).FontSize(13).TextColor(p.text).SingleLine()
		if !repo.UpdatedAt.IsZero() {
			ui.Text(c, updatedText(repo.UpdatedAt)).FontSize(11).TextColor(p.muted).NoWrap()
		}
	})
}

func (v *View) selectRepository(raw string, m application.Model) {
	ref, err := domain.ParseRepository(raw)
	if err != nil {
		return
	}
	for _, project := range m.Preferences.LocalProjects {
		if strings.EqualFold(project.FullName, ref.FullName()) {
			v.Query = ""
			v.gotoCloned(project.Path)
			return
		}
	}
	if pending := m.Preferences.Publication; pending != nil && strings.EqualFold(pending.Owner+"/"+pending.Request.Name, ref.FullName()) {
		v.startCreation(m)
		return
	}
	v.Query, v.SelectedURL, v.CloneParent = ref.URL(), ref.URL(), m.Preferences.DefaultCloneDir
	v.targetKey = ""
	if !m.Task.Busy() && m.Task.Kind != "publish" {
		v.Backend.ResetTask()
	}
}

func (v *View) cloneForm(c *ui.Context, m application.Model, p palette) {
	ref, err := domain.ParseRepository(v.SelectedURL)
	if err != nil {
		return
	}
	if v.CloneParent == "" {
		v.CloneParent = m.Preferences.DefaultCloneDir
	}
	var local string
	for _, project := range m.Preferences.LocalProjects {
		if strings.EqualFold(project.FullName, ref.FullName()) {
			local = project.Path
			break
		}
	}
	if local == "" && m.Task.Status == "success" && m.Task.Repository == ref {
		local = m.Task.Path
	}
	target := m.CloneTarget
	if local == "" {
		key := ref.URL() + "\n" + v.CloneParent
		if key != v.targetKey {
			v.targetKey = key
			v.Backend.CheckCloneTarget(ref.URL(), v.CloneParent)
			target = domain.CloneTarget{State: "checking"}
		}
		if target.Repository != ref || !domain.SamePath(target.Parent, v.CloneParent) {
			target = domain.CloneTarget{State: "checking"}
		}
	}
	ui.Column(c.Key("clone-form")).Gap(18).Padding(8, 0).Children(func() {
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, ref.FullName()).Grow(1).FontSize(19).Bold().TextColor(p.text).SingleLine()
			if iconButton(c, "clear-selection", "close", "清除选择", p, m.Task.Busy()) {
				v.Query, v.SelectedURL = "", ""
			}
		})
		path := filepath.Join(v.CloneParent, ref.Name)
		if local != "" {
			path = local
		}
		v.pathRow(c, path, "更改克隆位置", p, m.Task.Busy(), func() { v.chooseDirectory(v.CloneParent, func(path string) { v.CloneParent = path }) }, local == "")
		if local == "" {
			ui.Box(c.Key("target-status")).Height(18).Children(func() {
				message := target.Message
				if target.State == "checking" {
					message = "正在检查目录…"
				}
				if message != "" {
					ui.Text(c, message).FontSize(12).TextColor(p.muted).Tooltip(message)
				}
			})
		}
		ui.Row(c).Justify(ui.End).Gap(10).Children(func() {
			if local != "" {
				if button(c, "open-local", "launch", "用 Codex 打开", p, true, !m.Available[local]) {
					v.Backend.OpenProject(local, false)
				}
			} else {
				if target.OpenDirectory != "" && ui.Button(c.Key("open-target-directory"), "打开目录").Clicked() {
					v.Backend.OpenProject(target.OpenDirectory, true)
				}
				if button(c, "clone", "arrow", "克隆项目", p, true, target.State != "ready" || m.Task.Busy() || !m.DependenciesReady || m.Dependencies.GitErr != nil || m.Dependencies.GHErr != nil) {
					for _, project := range m.Preferences.LocalProjects {
						if strings.EqualFold(project.FullName, ref.FullName()) {
							v.gotoCloned(project.Path)
							return
						}
					}
					v.showLogs = false
					v.Backend.BeginClone(ref.URL(), v.CloneParent)
				}
			}
		})
		if m.DependenciesReady && (m.Dependencies.GitErr != nil || m.Dependencies.GHErr != nil) {
			if ui.Button(c, "配置 Git 与 gh").Clicked() {
				v.openSettings(m)
			}
		}
	})
}

func (v *View) pathRow(c *ui.Context, path, label string, p palette, disabled bool, change func(), editable bool) {
	ui.Row(c).Height(46).Padding(0, 12).Gap(10).Radius(8).Background(p.soft).Children(func() {
		icon(c, "folder", 16, p.muted)
		ui.Text(c, displayPath(path)).Grow(1).SingleLine().FontSize(12).TextColor(p.muted).Tooltip(path)
		if editable && ui.Button(c.Key(label), "更改").Label(label).Disabled(disabled || v.picking).Clicked() {
			change()
		}
	})
}

func (v *View) startCreation(m application.Model) {
	v.Creation, v.Page, v.showFiles = true, "home", false
	if v.ProjectParent == "" {
		v.ProjectParent = m.Preferences.DefaultCloneDir
	}
	if saved := m.Preferences.Publication; saved != nil {
		r := saved.Request
		v.ProjectName, v.ProjectParent, v.ExistingPath = r.Name, r.Parent, r.Path
		v.CreateMode, v.Visibility = 0, 0
		if r.Existing {
			v.CreateMode = 1
		}
		if !r.Private {
			v.Visibility = 1
		}
	} else {
		v.ProjectName, v.ExistingPath = "", ""
		v.ProjectParent = m.Preferences.DefaultCloneDir
		v.CreateMode, v.Visibility = 0, 0
	}
}

func (v *View) publishRequest() domain.PublishRequest {
	request := domain.PublishRequest{Name: strings.TrimSpace(v.ProjectName), Parent: v.ProjectParent, Existing: v.CreateMode == 1, Private: v.Visibility == 0}
	if request.Existing {
		request.Path, request.Parent = v.ExistingPath, ""
	}
	return request
}

func (v *View) createForm(c *ui.Context, m application.Model, p palette) {
	busy := m.Task.Busy()
	locked := m.Preferences.Publication != nil
	ui.Row(c).Children(func() {
		if ui.Button(c.Key("create-back"), "返回首页").Disabled(busy).Clicked() {
			v.Creation = false
		}
	})
	ui.Segmented(c.Key("create-mode"), &v.CreateMode, "新建项目", "已有目录").Disabled(busy || locked)
	ui.TextInput(c.Key("project-name"), &v.ProjectName).Height(44).Label("项目名称").Placeholder("项目名称").FontSize(14).Disabled(busy || locked)
	if v.CreateMode == 1 {
		label := "选择项目目录"
		if v.ExistingPath != "" {
			label = v.ExistingPath
		}
		v.pathRow(c, label, "选择已有项目目录", p, busy || locked, func() {
			v.chooseDirectory(v.ProjectParent, func(path string) {
				v.ExistingPath = path
				if strings.TrimSpace(v.ProjectName) == "" {
					v.ProjectName = filepath.Base(path)
				}
				v.showFiles = false
			})
		}, true)
	} else {
		v.pathRow(c, filepath.Join(v.ProjectParent, strings.TrimSpace(v.ProjectName)), "更改项目位置", p, busy || locked, func() {
			v.chooseDirectory(v.ProjectParent, func(path string) { v.ProjectParent = path })
		}, true)
	}
	ui.Segmented(c.Key("visibility"), &v.Visibility, "私人", "公开").Disabled(busy || locked)
	request := v.publishRequest()
	valid := request.Name != "" && (!request.Existing || request.Path != "")
	ui.Row(c).Gap(10).Children(func() {
		if ui.Button(c.Key("preview-files"), "查看待提交文件").Disabled(!valid || busy || locked || m.PreviewLoading).Clicked() {
			v.showFiles = true
			v.Backend.PreviewPublication(request)
		}
		ui.Spacer(c)
		label := "创建并打开"
		if locked {
			label = "继续发布"
		}
		if saved := m.Preferences.Publication; saved != nil && saved.Published {
			label = "重试打开 Codex"
		}
		if button(c, "publish", "launch", label, p, true, !valid || busy || m.Account == "" && !locked || !m.DependenciesReady) {
			v.Backend.BeginPublish(request)
		}
	})
	if m.Account == "" && !busy {
		if ui.Button(c.Key("create-connect"), "连接 GitHub").Clicked() {
			v.openSettings(m)
		}
	}
	if v.showFiles && m.PreviewRequest == request {
		if m.PreviewLoading {
			ui.Text(c, "正在检查…").FontSize(12).TextColor(p.muted)
		}
		if m.PreviewError != "" {
			ui.Text(c, shortError(m.PreviewError)).FontSize(12).TextColor(p.danger).Tooltip(m.PreviewError)
		}
		ui.Scroll(c.Key("file-preview")).MaxHeight(140).Padding(12).Radius(8).Background(p.soft).Gap(5).Children(func() {
			for _, file := range m.PreviewFiles {
				ui.Text(c, file).FontSize(11).TextColor(p.muted).SingleLine().Tooltip(file)
			}
			if !m.PreviewLoading && m.PreviewError == "" && len(m.PreviewFiles) == 0 {
				ui.Text(c, "没有可列出的文件").FontSize(12).TextColor(p.muted)
			}
		})
	}
}

func (v *View) gotoCloned(path string) {
	v.Page, v.HighlightPath, v.ClonedQuery, v.reveal = "cloned", path, "", true
	v.Creation, v.SelectedURL = false, ""
}

func (v *View) clonedPage(c *ui.Context, m application.Model, p palette) {
	ui.TextInput(c.Key("cloned-search"), &v.ClonedQuery).Height(42).Label("搜索已克隆项目").Placeholder("搜索已克隆项目").FontSize(13)
	if len(m.Preferences.LocalProjects) == 0 {
		ui.Column(c).Grow(1).Center().Gap(16).Children(func() {
			ui.Text(c, "还没有克隆过项目").FontSize(14).TextColor(p.muted)
			if ui.Button(c, "去首页").Clicked() {
				v.Page = "home"
			}
		})
		return
	}
	ui.Scroll(c.Key("cloned-list")).Grow(1).TrackScroll(&v.cloned).Gap(4).Children(func() {
		count := 0
		for _, project := range m.Preferences.LocalProjects {
			query := strings.ToLower(strings.TrimSpace(v.ClonedQuery))
			if !strings.Contains(strings.ToLower(project.FullName+" "+project.Path), query) {
				continue
			}
			count++
			available := m.Available[project.Path]
			row := ui.Row(c.Key(project.Path)).Height(78).Shrink(0).Padding(12).Gap(12).Radius(9)
			if v.HighlightPath != "" && domain.SamePath(project.Path, v.HighlightPath) {
				row.Background(p.accentSoft)
				if v.reveal {
					row.ScrollIntoView()
					v.reveal = false
				}
			}
			row.Children(func() {
				icon(c, "repo", 21, p.muted)
				ui.Column(c).Grow(1).Gap(6).Children(func() {
					ui.Text(c, project.FullName).FontSize(14).Bold().TextColor(p.text).SingleLine()
					path := displayPath(project.Path)
					if !available {
						path = "目录不可用 · " + path
					}
					ui.Text(c, path).FontSize(11).TextColor(p.muted).SingleLine().Tooltip(project.Path)
				})
				if button(c, "cloned-open", "", "用 Codex 打开", p, true, !available) {
					v.Backend.OpenProject(project.Path, false)
				}
				more := ui.ButtonBase(c.Key("cloned-more")).Size(28, 34).Center().Label("项目操作 " + project.FullName).Tooltip("项目操作")
				more.Children(func() { icon(c, "more", 18, p.muted) })
				more.Menu(func(menu *ui.Menu) {
					if menu.Item("打开文件夹").Disabled(!available).Chosen() {
						v.Backend.OpenProject(project.Path, true)
					}
					menu.Separator()
					if menu.Item("移除记录").Disabled(m.Task.Busy() && domain.SamePath(m.Task.Path, project.Path)).Chosen() {
						v.Backend.RemoveLocalProject(project.Path)
					}
				})
			})
		}
		if count == 0 {
			ui.Text(c, "没有匹配的项目").Padding(20).FontSize(13).TextColor(p.muted)
		}
	})
}

func (v *View) taskBar(c *ui.Context, m application.Model, p palette) {
	task := m.Task
	if task.Status == "" || task.Status == "idle" || task.Kind == "publish" && task.Status == "success" {
		return
	}
	if task.Status == "success" && task.Kind == "clone" && v.Page != "cloned" && !v.Creation && v.SelectedURL == task.Repository.URL() {
		return
	}
	ui.Column(c.Key("task-bar")).Padding(12, 14).Gap(10).Radius(10).Background(p.soft).Children(func() {
		ui.Row(c).Gap(10).Children(func() {
			if task.Busy() {
				ui.Spinner(c).Size(14, 14)
			} else {
				icon(c, "repo", 15, p.muted)
			}
			stage := task.Progress.Stage
			if stage == "" {
				stage = task.Repository.FullName()
			}
			if task.Status == "success" {
				stage = task.Repository.FullName()
			}
			if task.Status == "cancelled" {
				stage = "任务已取消"
			}
			if task.Status == "cancelling" {
				stage = "正在取消…"
			}
			ui.Text(c, stage).Grow(1).FontSize(12).TextColor(p.text).SingleLine()
			if task.Busy() {
				if task.Progress.Percent >= 0 && task.Progress.Stage != "" {
					ui.Text(c, fmt.Sprintf("%d%%", task.Progress.Percent)).FontSize(12).TextColor(p.muted)
				}
				if ui.Button(c.Key("cancel-task"), "取消").Disabled(task.Status == "cancelling" || task.Status == "opening").Clicked() {
					v.Backend.CancelClone()
				}
			} else if task.Kind == "publish" && m.Preferences.Publication != nil {
				label := "继续发布"
				if m.Preferences.Publication.Published {
					label = "重试打开 Codex"
				}
				if ui.Button(c.Key("retry-publication"), label).Clicked() {
					v.startCreation(m)
					v.Backend.BeginPublish(m.Preferences.Publication.Request)
				}
			} else if task.Status == "success" && task.Path != "" {
				if ui.Button(c.Key("task-open"), "用 Codex 打开").Clicked() {
					v.Backend.OpenProject(task.Path, false)
				}
			}
			if len(task.Logs) > 0 || task.Error != "" {
				if ui.Button(c.Key("toggle-logs"), "详情").Clicked() {
					v.showLogs = !v.showLogs
				}
			}
			if task.Kind == "publish" && m.Preferences.Publication != nil && !task.Busy() {
				more := ui.ButtonBase(c.Key("publication-menu")).Size(28, 30).Center().Label("发布任务操作").Tooltip("发布任务操作")
				more.Children(func() { icon(c, "more", 17, p.muted) })
				more.Menu(func(menu *ui.Menu) {
					if menu.Item("打开本地目录").Disabled(task.Path == "").Chosen() {
						v.Backend.OpenProject(task.Path, true)
					}
					if menu.Item("结束发布任务，保留项目").Chosen() {
						v.Backend.EndPublication()
						v.Creation = false
					}
				})
			}
			if !task.Busy() && iconButton(c, "dismiss-task", "close", "收起任务", p, false) {
				v.Backend.ResetTask()
			}
		})
		if task.Busy() {
			value := float64(-1)
			if task.Progress.Percent >= 0 && task.Progress.Stage != "" {
				value = float64(task.Progress.Percent) / 100
			}
			ui.Progress(c, value).Height(3).Label("任务进度")
		}
		if task.Error != "" {
			ui.Text(c, shortError(task.Error)).FontSize(12).TextColor(p.danger).Tooltip(task.Error)
		}
		if v.showLogs {
			ui.Scroll(c.Key("task-logs")).Height(84).TrackScroll(&v.logs).Gap(4).Children(func() {
				for _, line := range task.Logs {
					ui.Text(c, line).FontSize(11).TextColor(p.muted).Selectable()
				}
				if task.Error != "" {
					ui.Text(c, task.Error).FontSize(11).TextColor(p.danger).Selectable()
				}
			})
		}
	})
}

func (v *View) chooseDirectory(initial string, apply func(string)) {
	if v.Actions == nil || v.picking {
		return
	}
	v.picking = true
	pick := func() {
		path, err := v.Actions.PickDirectory(initial)
		done := func() {
			v.picking = false
			if err != nil {
				v.Backend.ReportError(err)
			} else if path != "" {
				apply(path)
			}
		}
		if native, ok := v.Actions.(*NativeActions); ok && native.Window != nil {
			native.Window.Update(done)
		} else {
			done()
		}
	}
	if _, ok := v.Actions.(*NativeActions); ok {
		go pick()
	} else {
		pick()
	}
}

func shortError(s string) string {
	first := strings.Split(strings.TrimSpace(s), "\n")[0]
	runes := []rune(first)
	if len(runes) > 90 {
		return string(runes[:90]) + "…"
	}
	return first
}

func displayPath(path string) string { return filepath.Clean(path) }

func updatedText(t time.Time) string {
	elapsed := time.Since(t)
	if elapsed < time.Minute {
		return "刚刚"
	}
	if elapsed < time.Hour {
		return fmt.Sprintf("%d 分钟前", int(elapsed.Minutes()))
	}
	if elapsed < 24*time.Hour {
		return fmt.Sprintf("%d 小时前", int(elapsed.Hours()))
	}
	if elapsed < 7*24*time.Hour {
		return fmt.Sprintf("%d 天前", int(elapsed.Hours()/24))
	}
	return t.Local().Format("2006-01-02")
}
