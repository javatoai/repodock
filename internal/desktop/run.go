package desktop

import (
	"repodock/assets"
	"repodock/internal/application"
	"repodock/internal/platform"
	"repodock/internal/storage"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"
)

func Run(configPath string) error {
	mygo.App.SetName("RepoDock")
	mygo.App.SetLanguages("zh-Hans")
	mygo.App.WhenReady(func() {
		var win *mygo.Window
		actions := &NativeActions{}
		backend := application.NewController(&storage.Preferences{Path: configPath}, actions, platform.ToolResolver{}, func(deps application.Dependencies) application.Services {
			return application.Services{
				GitHub: &platform.GitHub{Runner: platform.ExecRunner{}, GHPath: deps.GHPath, GitPath: deps.GitPath},
				Cloner: &platform.Cloner{Runner: platform.ExecRunner{}, GHPath: deps.GHPath, GitPath: deps.GitPath},
			}
		}, func() {
			if win != nil {
				win.Invalidate()
			}
		})
		view := &View{Backend: backend, Actions: actions}
		win = mygo.NewWindow(mygo.WindowOptions{Title: "RepoDock", Width: 800, Height: 640, MinWidth: 640, MinHeight: 520, Content: ui.View(view.Render)})
		actions.Window = win
		if err := win.SetIcon(assets.Icon); err != nil {
			backend.ReportError(err)
		}
		closing, mayClose := false, false
		win.OnClose(func(event *mygo.CloseEvent) {
			if mayClose {
				return
			}
			event.PreventDefault()
			if closing {
				return
			}
			closing = true
			backend.CancelClone()
			go func() {
				backend.Close()
				win.Update(func() { mayClose = true; win.Close(); mygo.App.Quit() })
			}()
		})
		win.OnFocus(func() {
			if !backend.Snapshot().Loading {
				backend.Refresh()
			}
		})
		backend.Refresh()
	})
	return mygo.App.Run()
}
