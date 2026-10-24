package desktop

import (
	"path/filepath"
	"repodock/internal/application"
	"repodock/internal/domain"
	"time"
)

// Preview is a fully offline fixture, used only by cmd/preview and UI tests.
// It never discovers accounts, launches applications, or changes user files.
func Preview(state string) *View {
	p := domain.DefaultPreferences()
	p.Theme = "light"
	p.LastParent = filepath.FromSlash("/Users/you/Projects")
	model := application.Model{Preferences: p, Account: "you", Complete: true, DependenciesReady: true, Available: map[string]bool{}, Task: application.Task{Status: "idle"}}
	for _, name := range []string{"atlas", "studio", "notes"} {
		ref := domain.RepositoryRef{Owner: "you", Name: name}
		model.Repositories = append(model.Repositories, domain.Repository{Ref: ref, UpdatedAt: time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)})
		path := filepath.Join(p.LastParent, name)
		model.Preferences.Recent = append(model.Preferences.Recent, domain.RecentProject{FullName: ref.FullName(), Path: path})
		model.Available[path] = true
	}
	if state == "dark" {
		model.Preferences.Theme = "dark"
	}
	if state == "empty" {
		model.Preferences.Recent = nil
	}
	view := &View{Backend: &previewBackend{model: model}}
	if state == "progress" {
		view.Query = "https://github.com/you/workbench"
		b := view.Backend.(*previewBackend)
		b.model.Task = application.Task{Status: "cloning", Repository: domain.RepositoryRef{Owner: "you", Name: "workbench"}, Progress: domain.Progress{Stage: "接收对象", Percent: 68}, Logs: []string{"Receiving objects: 68% (328/482)"}}
	}
	if state == "success" {
		view.Backend.(*previewBackend).model.Task = application.Task{Status: "success", Repository: domain.RepositoryRef{Owner: "you", Name: "workbench"}, Path: filepath.Join(p.LastParent, "workbench")}
	}
	if state == "error" {
		view.Backend.(*previewBackend).model.Task = application.Task{Status: "error", Error: "同名目录已存在，请选择其他保存位置"}
	}
	if state == "settings" {
		view.openSettings(view.Backend.Snapshot())
	}
	return view
}

type previewBackend struct {
	model                           application.Model
	clones, opens, folders, removes int
	lastPath                        string
}

func (b *previewBackend) Snapshot() application.Model { return b.model }
func (b *previewBackend) Refresh()                    {}
func (b *previewBackend) BeginClone(string)           { b.clones++ }
func (b *previewBackend) CancelClone()                { b.model.Task.Status = "cancelled" }
func (b *previewBackend) ResetTask()                  { b.model.Task = application.Task{Status: "idle"} }
func (b *previewBackend) OpenProject(path string, folder bool) {
	b.lastPath = path
	if folder {
		b.folders++
	} else {
		b.opens++
	}
}
func (b *previewBackend) RemoveRecent(path string)  { b.removes++; b.lastPath = path }
func (b *previewBackend) SetTheme(theme string)     { b.model.Preferences.Theme = theme }
func (b *previewBackend) ApplyTools(string, string) {}
func (b *previewBackend) DismissNotice()            { b.model.Notice = "" }
func (b *previewBackend) ReportError(error)         {}
