package desktop

import (
	"path/filepath"
	"repodock/internal/domain"
	"testing"

	"github.com/egoist/mygo/ui"
)

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}

func TestHomeSearchKeyboardCloneAndSingleUseDirectory(t *testing.T) {
	v := Preview("ready")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 960, 680)
	if tt.HasText("you/atlas") || !tt.HasText("创建项目") {
		t.Fatal(tt.Texts())
	}
	click(t, tt, "搜索 GitHub 项目")
	tt.Type("studio")
	if !tt.HasText("you/studio") || tt.HasText("you/atlas") {
		t.Fatal(tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if v.SelectedURL != "https://github.com/you/studio" {
		t.Fatal(v.SelectedURL)
	}
	v.CloneParent = t.TempDir()
	original := b.model.Preferences.DefaultCloneDir
	tt.Frame()
	tt.Frame() // The new location must finish its inspection before cloning.
	click(t, tt, "克隆项目")
	if b.clones != 1 || b.lastPath != v.CloneParent || b.model.Preferences.DefaultCloneDir != original {
		t.Fatal(b.clones, b.lastPath, v.CloneParent)
	}
}

func TestOpenedSearchResultNavigatesWithoutCloneOrLaunch(t *testing.T) {
	v := Preview("ready")
	v.ClonedQuery = "not-visible"
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 960, 680)
	click(t, tt, "搜索 GitHub 项目")
	tt.Type("atlas")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if v.Page != "cloned" || v.ClonedQuery != "" || v.HighlightPath != b.model.Preferences.Recent[0].Path || b.opens != 0 || b.clones != 0 {
		t.Fatal(v.Page, v.HighlightPath, b.opens, b.clones)
	}
	if !tt.HasText("you/atlas") {
		t.Fatal(tt.Texts())
	}
}

func TestCloneSuccessOnlyEntersOpenedAfterLaunch(t *testing.T) {
	v := Preview("success")
	b := v.Backend.(*previewBackend)
	before := len(b.model.Preferences.Recent)
	tt := ui.NewTester(v.Render, 960, 680)
	click(t, tt, "用 Codex 打开")
	tt.Frame()
	if b.opens != 1 || v.Page != "cloned" || len(b.model.Preferences.Recent) != before+1 {
		t.Fatal(b.opens, v.Page, b.model.Preferences.Recent)
	}
}

func TestTaskSurvivesNavigationAndCanCancel(t *testing.T) {
	v := Preview("progress")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 960, 680)
	click(t, tt, "已克隆")
	if !tt.HasText("接收对象") || !tt.HasText("68%") {
		t.Fatal(tt.Texts())
	}
	click(t, tt, "取消")
	if b.model.Task.Status != "cancelled" {
		t.Fatal(b.model.Task)
	}
}

func TestCreateFormPrivateDefaultPreviewAndPublish(t *testing.T) {
	v := Preview("ready")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 960, 680)
	click(t, tt, "创建项目")
	click(t, tt, "项目名称")
	tt.Type("sample")
	click(t, tt, "查看待提交文件")
	if !tt.HasText("README.md") {
		t.Fatal(tt.Texts())
	}
	click(t, tt, "创建并打开")
	if b.publishes != 1 || b.lastRequest.Name != "sample" || !b.lastRequest.Private {
		t.Fatal(b.lastRequest, b.publishes)
	}
	click(t, tt, "公开")
	click(t, tt, "创建并打开")
	if b.lastRequest.Private {
		t.Fatal("public choice ignored")
	}
}

func TestOpenedMenuAndUnavailableProject(t *testing.T) {
	v := Preview("cloned")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 960, 680)
	click(t, tt, "项目操作 you/atlas")
	if err := tt.ChooseMenuItem("打开文件夹"); err != nil {
		t.Fatal(err)
	}
	if b.folders != 1 {
		t.Fatal(b.folders)
	}
	click(t, tt, "项目操作 you/atlas")
	if err := tt.ChooseMenuItem("移除记录"); err != nil {
		t.Fatal(err)
	}
	if b.removes != 1 || len(b.model.Preferences.Recent) != 1 {
		t.Fatal(b.model.Preferences.Recent)
	}
	b.model.Available[b.model.Preferences.Recent[0].Path] = false
	tt.Frame()
	click(t, tt, "用 Codex 打开")
	if b.opens != 0 {
		t.Fatal("unavailable path launched")
	}
}

func TestSettingsDefaultDirectoryAndThemesLayouts(t *testing.T) {
	for _, state := range []string{"ready", "dark", "cloned", "dark-cloned", "empty", "create", "create-existing", "progress", "success", "error", "settings", "settings-dark"} {
		for _, size := range [][2]int{{960, 680}, {640, 520}} {
			t.Run(state, func(t *testing.T) {
				v := Preview(state)
				tt := ui.NewTester(v.Render, size[0], size[1])
				if !tt.HasText("首页") || !tt.HasText("已克隆") || tt.Image().Bounds().Dx() != size[0] {
					t.Fatal(tt.Texts())
				}
			})
		}
	}
	v := Preview("settings")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 960, 680)
	v.defaultDir = filepath.Join(t.TempDir(), "项目")
	click(t, tt, "保存并检测")
	if b.model.Preferences.DefaultCloneDir != v.defaultDir {
		t.Fatal(b.model.Preferences)
	}
	v = Preview("empty")
	v.Backend.(*previewBackend).model.Preferences = domain.DefaultPreferences()
	tt = ui.NewTester(v.Render, 960, 680)
	light := tt.Image().At(1, 1)
	tt.SetDark(true)
	if tt.Image().At(1, 1) == light {
		t.Fatal("system theme did not change")
	}
}

func TestEmptyDisconnectedAndPastedSearchResults(t *testing.T) {
	v := Preview("ready")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 640, 520)
	click(t, tt, "搜索 GitHub 项目")
	tt.Type("not-a-match")
	if !tt.HasText("没有匹配的项目") {
		t.Fatal(tt.Texts())
	}
	v = Preview("ready")
	b = v.Backend.(*previewBackend)
	b.model.Repositories = nil
	b.model.Account = ""
	b.model.AccountError = "GitHub 未连接"
	tt = ui.NewTester(v.Render, 640, 520)
	click(t, tt, "搜索 GitHub 项目")
	if !tt.HasText("GitHub 未连接") || !tt.HasText("连接 GitHub") {
		t.Fatal(tt.Texts())
	}
	v = Preview("ready")
	tt = ui.NewTester(v.Render, 640, 520)
	click(t, tt, "搜索 GitHub 项目")
	tt.Type("git@github.com:you/atlas.git")
	tt.Key(0, ui.KeyEnter)
	if v.Page != "cloned" {
		t.Fatal("pasted known repository did not navigate", v.Page, tt.Texts())
	}
}

func TestEachNewProjectStartsPrivateInDefaultDirectory(t *testing.T) {
	v := Preview("ready")
	b := v.Backend.(*previewBackend)
	v.ProjectName, v.ExistingPath, v.ProjectParent = "previous", "old-path", "one-off"
	v.Visibility, v.CreateMode = 1, 1
	v.startCreation(b.model)
	if v.ProjectName != "" || v.ExistingPath != "" || v.ProjectParent != b.model.Preferences.DefaultCloneDir || v.Visibility != 0 || v.CreateMode != 0 {
		t.Fatal(v.publishRequest())
	}
}
