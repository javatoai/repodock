package desktop

import (
	"repodock/internal/domain"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRepositoryPickerSearchKeyboardAndClone(t *testing.T) {
	v := Preview("ready")
	b := v.Backend.(*previewBackend)
	b.model.Preferences.Recent = nil
	tt := ui.NewTester(v.Render, 800, 640)
	if err := tt.Click("选择位置并克隆"); err != nil {
		t.Fatal(err)
	}
	if b.clones != 0 {
		t.Fatal("empty URL cloned")
	}
	if err := tt.Click("仓库地址或名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("studio")
	if !tt.HasText("you/studio") || tt.HasText("you/atlas") {
		t.Fatal(tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if v.Query != "https://github.com/you/studio" {
		t.Fatal("keyboard choice failed", v.Query)
	}
	if err := tt.Click("选择位置并克隆"); err != nil {
		t.Fatal(err)
	}
	if b.clones != 1 {
		t.Fatal("valid repository did not clone", b.clones)
	}
}

func TestPastedRepositoryAndBusyState(t *testing.T) {
	v := Preview("empty")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 800, 640)
	if err := tt.Click("仓库地址或名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("git@github.com:you/private.git")
	if err := tt.Click("选择位置并克隆"); err != nil {
		t.Fatal(err)
	}
	if b.clones != 1 {
		t.Fatal(b.clones)
	}
	v = Preview("progress")
	b = v.Backend.(*previewBackend)
	tt = ui.NewTester(v.Render, 800, 640)
	if !tt.HasText("接收对象") || !tt.HasText("68%") || tt.HasText("使用 Codex 打开") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if b.model.Task.Status != "cancelled" {
		t.Fatal(b.model.Task)
	}
}

func TestSuccessAndRecentProjectActions(t *testing.T) {
	v := Preview("success")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 800, 640)
	if err := tt.Click("使用 Codex 打开"); err != nil {
		t.Fatal(err)
	}
	if b.opens != 1 || b.lastPath != b.model.Task.Path {
		t.Fatal(b.opens, b.lastPath)
	}
	if err := tt.Click("克隆另一个"); err != nil {
		t.Fatal(err)
	}
	if b.model.Task.Status != "idle" {
		t.Fatal(b.model.Task)
	}
	if err := tt.Click("打开文件夹 you/atlas"); err != nil {
		t.Fatal(err)
	}
	if b.folders != 1 {
		t.Fatal(b.folders)
	}
	if err := tt.Click("移除记录 you/atlas"); err != nil {
		t.Fatal(err)
	}
	if b.removes != 1 {
		t.Fatal(b.removes)
	}
	b.model.Available[b.model.Preferences.Recent[0].Path] = false
	tt.Frame()
	before := b.opens
	if err := tt.Click("用 Codex 打开 you/atlas"); err != nil {
		t.Fatal(err)
	}
	if b.opens != before {
		t.Fatal("missing directory opened")
	}
}

func TestThemesLayoutsAndOfflineStates(t *testing.T) {
	for _, state := range []string{"ready", "dark", "empty", "progress", "success", "error", "settings"} {
		for _, size := range [][2]int{{800, 640}, {640, 520}} {
			t.Run(state, func(t *testing.T) {
				v := Preview(state)
				tt := ui.NewTester(v.Render, size[0], size[1])
				if !tt.HasText("RepoDock") {
					t.Fatal(tt.Texts())
				}
				if tt.Image().Bounds().Dx() != size[0] {
					t.Fatal("wrong dimensions")
				}
			})
		}
	}
	v := Preview("empty")
	b := v.Backend.(*previewBackend)
	b.model.Preferences = domain.DefaultPreferences()
	tt := ui.NewTester(v.Render, 800, 640)
	light := tt.Image().At(1, 1)
	tt.SetDark(true)
	if tt.Image().At(1, 1) == light {
		t.Fatal("system theme did not follow dark mode")
	}
}
