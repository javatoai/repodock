package desktop

import (
	"path/filepath"
	"repodock/internal/application"
	"repodock/internal/domain"
	"runtime"
	"slices"
	"time"
)

// Preview is a fully offline fixture, used only by cmd/preview and UI tests.
// It never discovers accounts, launches applications, or changes user files.
func Preview(state string) *View {
	p := domain.DefaultPreferences()
	p.Theme = "light"
	p.LastParent = filepath.FromSlash("/Users/you/Projects")
	if runtime.GOOS == "windows" {
		p.LastParent = "C:\\Projects"
	}
	p.DefaultCloneDir = p.LastParent
	model := application.Model{Preferences: p, Account: "you", Complete: true, DependenciesReady: true, Available: map[string]bool{}, Task: application.Task{Status: "idle"}}
	for i, name := range []string{"atlas", "studio", "notes"} {
		ref := domain.RepositoryRef{Owner: "you", Name: name}
		model.Repositories = append(model.Repositories, domain.Repository{Ref: ref, Private: i == 1, UpdatedAt: time.Now().Add(-time.Duration(i+1) * time.Hour)})
		path := filepath.Join(p.LastParent, name)
		if name != "studio" {
			project := domain.RecentProject{FullName: ref.FullName(), Path: path, LastOpenedAt: time.Now().Add(-time.Duration(i) * time.Hour), PreparedAt: time.Now().Add(-time.Duration(i+1) * time.Hour)}
			model.Preferences.Recent = append(model.Preferences.Recent, project)
			model.Preferences.LocalProjects = append(model.Preferences.LocalProjects, project)
		}
		model.Available[path] = true
	}
	if state == "dark" {
		model.Preferences.Theme = "dark"
	}
	if state == "empty" {
		model.Preferences.Recent = nil
		model.Preferences.LocalProjects = nil
	}
	view := &View{Backend: &previewBackend{model: model}}
	if state == "cloned" || state == "dark-cloned" {
		view.Page = "cloned"
	}
	if state == "dark-cloned" {
		view.Backend.(*previewBackend).model.Preferences.Theme = "dark"
	}
	if state == "create" || state == "create-existing" {
		view.Creation, view.ProjectName, view.ProjectParent = true, "new-project", p.DefaultCloneDir
		if state == "create-existing" {
			view.CreateMode, view.ExistingPath = 1, filepath.Join(p.DefaultCloneDir, "existing-project")
		}
	}
	if state == "progress" {
		view.Query = "https://github.com/you/workbench"
		view.SelectedURL = view.Query
		b := view.Backend.(*previewBackend)
		b.model.Task = application.Task{Kind: "clone", Status: "cloning", Repository: domain.RepositoryRef{Owner: "you", Name: "workbench"}, Progress: domain.Progress{Stage: "接收对象", Percent: 68}, Logs: []string{"Receiving objects: 68% (328/482)"}}
	}
	if state == "success" {
		view.SelectedURL = "https://github.com/you/workbench"
		b := view.Backend.(*previewBackend)
		b.model.Task = application.Task{Kind: "clone", Status: "success", Repository: domain.RepositoryRef{Owner: "you", Name: "workbench"}, Path: filepath.Join(p.LastParent, "workbench")}
		b.model.Preferences.RememberLocal(domain.RecentProject{FullName: "you/workbench", Path: b.model.Task.Path})
		b.model.Available[b.model.Task.Path] = true
	}
	if state == "error" {
		view.Backend.(*previewBackend).model.Task = application.Task{Status: "error", Error: "同名目录已存在，请选择其他保存位置"}
	}
	if state == "settings" || state == "settings-dark" {
		if state == "settings-dark" {
			view.Backend.(*previewBackend).model.Preferences.Theme = "dark"
		}
		view.openSettings(view.Backend.Snapshot())
	}
	if state == "occupied" || state == "occupied-dark" || state == "target-file" || state == "checking" {
		b := view.Backend.(*previewBackend)
		view.SelectedURL, view.CloneParent = "https://github.com/you/workbench", p.DefaultCloneDir
		view.Query = view.SelectedURL
		b.targetState = "exists"
		if state == "target-file" {
			b.targetState = "file"
		}
		if state == "checking" {
			b.targetState = "checking"
		}
		if state == "occupied-dark" {
			b.model.Preferences.Theme = "dark"
		}
		b.CheckCloneTarget(view.SelectedURL, view.CloneParent)
		view.targetKey = view.SelectedURL + "\n" + view.CloneParent
	}
	if state == "cloned-unopened" {
		b := view.Backend.(*previewBackend)
		path := filepath.Join(p.LastParent, "workbench")
		b.model.Preferences.RememberLocal(domain.RecentProject{FullName: "you/workbench", Path: path, PreparedAt: time.Now()})
		b.model.Available[path] = true
		view.Page = "cloned"
	}
	if state == "proxy" || state == "proxy-dark" || state == "proxy-error" {
		b := view.Backend.(*previewBackend)
		b.model.Preferences.Proxy.Enabled = true
		if state == "proxy-dark" {
			b.model.Preferences.Theme = "dark"
		}
		view.openSettings(b.model)
		b.TestProxy(b.model.Preferences.Proxy)
		if state == "proxy-error" {
			b.model.ProxyTest.Message = ""
			b.model.ProxyTest.Error = "代理连接失败：127.0.0.1:7897 无法连接"
		}
	}
	return view
}

type previewBackend struct {
	targetState                     string
	checks                          int
	model                           application.Model
	clones, opens, folders, removes int
	lastPath                        string
	lastRequest                     domain.PublishRequest
	publishes                       int
	settingsSaves, refreshes        int
	settingsError                   string
}

func (b *previewBackend) Snapshot() application.Model { return b.model }
func (b *previewBackend) Refresh()                    { b.refreshes++ }
func (b *previewBackend) CheckCloneTarget(raw, parent string) {
	b.checks++
	ref, _ := domain.ParseRepository(raw)
	b.model.CloneTarget = domain.CloneTarget{Repository: ref, Parent: parent, Path: filepath.Join(parent, ref.Name), State: "ready"}
	switch b.targetState {
	case "exists":
		b.model.CloneTarget.State, b.model.CloneTarget.Message, b.model.CloneTarget.OpenDirectory = "exists", "目标目录已存在，请更改保存位置", b.model.CloneTarget.Path
	case "file":
		b.model.CloneTarget.State, b.model.CloneTarget.Message, b.model.CloneTarget.OpenDirectory = "file", "目标路径已存在，请更改保存位置", parent
	case "checking":
		b.model.CloneTarget.State = "checking"
	}
}
func (b *previewBackend) TestProxy(config domain.ProxyConfig) {
	config, err := domain.NormalizeProxy(config)
	b.model.ProxyTest = application.ProxyTest{Config: config, Message: "GitHub API 连接成功"}
	if err != nil {
		b.model.ProxyTest.Error = err.Error()
	}
}
func (b *previewBackend) BeginClone(_ string, parent ...string) {
	b.clones++
	if len(parent) > 0 {
		b.lastPath = parent[0]
	}
}
func (b *previewBackend) BeginPublish(request domain.PublishRequest) {
	b.publishes++
	b.lastRequest = request
}
func (b *previewBackend) EndPublication() {
	b.model.Preferences.Publication = nil
	b.model.Task = application.Task{Status: "idle"}
}
func (b *previewBackend) PreviewPublication(request domain.PublishRequest) {
	b.model.PreviewRequest = request
	b.model.PreviewFiles = []string{"README.md"}
}
func (b *previewBackend) CancelClone() { b.model.Task.Status = "cancelled" }
func (b *previewBackend) ResetTask()   { b.model.Task = application.Task{Status: "idle"} }
func (b *previewBackend) OpenProject(path string, folder bool) {
	b.lastPath = path
	if folder {
		b.folders++
	} else {
		b.opens++
		for _, project := range b.model.Preferences.LocalProjects {
			if project.Path == path {
				project.LastOpenedAt = time.Now()
				b.model.Preferences.Remember(project)
				break
			}
		}
		b.model.OpenedPath, b.model.OpenSequence = path, b.model.OpenSequence+1
	}
}
func (b *previewBackend) RemoveLocalProject(path string) {
	b.removes++
	b.lastPath = path
	b.model.Preferences.Recent = slices.DeleteFunc(b.model.Preferences.Recent, func(p domain.RecentProject) bool { return p.Path == path })
	b.model.Preferences.LocalProjects = slices.DeleteFunc(b.model.Preferences.LocalProjects, func(p domain.RecentProject) bool { return p.Path == path })
}
func (b *previewBackend) SetTheme(theme string) { b.model.Preferences.Theme = theme }
func (b *previewBackend) ApplySettings(git, gh, dir string, proxies ...domain.ProxyConfig) {
	b.settingsSaves++
	if b.settingsError != "" {
		b.model.Notice = b.settingsError
		return
	}
	b.model.Preferences.GitPath, b.model.Preferences.GHPath, b.model.Preferences.DefaultCloneDir = git, gh, dir
	if len(proxies) > 0 {
		b.model.Preferences.Proxy = proxies[0]
	}
}
func (b *previewBackend) DismissNotice() { b.model.Notice = "" }
func (b *previewBackend) ReportError(err error) {
	if err != nil {
		b.model.Notice = err.Error()
	}
}
