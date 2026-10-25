package desktop

import (
	"path/filepath"
	"repodock/internal/domain"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCloneTargetConflictDisablesCloneAndOpensOnlyExplorer(t *testing.T) {
	for _, state := range []string{"occupied", "target-file", "checking"} {
		t.Run(state, func(t *testing.T) {
			v := Preview(state)
			b := v.Backend.(*previewBackend)
			tt := ui.NewTester(v.Render, 640, 520)
			click(t, tt, "克隆项目")
			if b.clones != 0 {
				t.Fatal("cloned during conflict/check")
			}
			if state != "checking" {
				click(t, tt, "打开目录")
				want := filepath.Join(v.CloneParent, "workbench")
				if state == "target-file" {
					want = v.CloneParent
				}
				if b.folders != 1 || b.opens != 0 || b.lastPath != want {
					t.Fatal(b.folders, b.opens, b.lastPath, want)
				}
			}
			b.targetState = ""
			v.CloneParent = filepath.Join(t.TempDir(), "another")
			tt.Frame()
			tt.Frame()
			click(t, tt, "克隆项目")
			if b.clones != 1 || b.lastPath != v.CloneParent {
				t.Fatal(b.clones, b.lastPath)
			}
		})
	}
}

func TestClonedCatalogIncludesUnopenedAndSearchLocatesIt(t *testing.T) {
	v := Preview("cloned-unopened")
	b := v.Backend.(*previewBackend)
	if len(b.model.Preferences.LocalProjects) != len(b.model.Preferences.Recent)+1 {
		t.Fatal("invalid fixture")
	}
	tt := ui.NewTester(v.Render, 960, 680)
	if !tt.HasText("you/workbench") {
		t.Fatal(tt.Texts())
	}
	v.ClonedQuery = "hide-target"
	v.Page = "home"
	tt.Frame()
	click(t, tt, "搜索 GitHub 项目")
	tt.Type("https://github.com/you/workbench")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if v.Page != "cloned" || v.ClonedQuery != "" || !tt.HasText("you/workbench") || b.opens != 0 || b.clones != 0 {
		t.Fatal(v.Page, tt.Texts())
	}
	click(t, tt, "项目操作 you/workbench")
	if err := tt.ChooseMenuItem("移除记录"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("you/workbench") {
		t.Fatal("removed catalog entry remains")
	}
	if len(b.model.Preferences.LocalProjects) != len(b.model.Preferences.Recent) {
		t.Fatal("catalog/history cleanup mismatch")
	}
}

func TestProxyDraftPreservedTestedAndDisabledDuringTask(t *testing.T) {
	v := Preview("settings")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 640, 520)
	if v.proxyEnabled || v.proxyHost != "127.0.0.1" || v.proxyPort != "7897" {
		t.Fatal(v.proxyDraft())
	}
	click(t, tt, "启用网络代理")
	click(t, tt, "SOCKS5")
	v.proxyPort = "1080"
	tt.Frame()
	click(t, tt, "测试连接")
	want := domain.ProxyConfig{Enabled: true, URL: "socks5://127.0.0.1:1080"}
	if b.model.ProxyTest.Config != want || b.model.Preferences.Proxy.Enabled {
		t.Fatal(b.model.ProxyTest, b.model.Preferences.Proxy)
	}
	click(t, tt, "首页")
	click(t, tt, "设置")
	if v.proxyDraft() != want {
		t.Fatal(v.proxyDraft())
	}
	click(t, tt, "保存并检测")
	if b.model.Preferences.Proxy != want || v.Page != "settings" {
		t.Fatal(b.model.Preferences.Proxy, v.Page)
	}
	b.model.Task.Status = "cloning"
	tt.Frame()
	click(t, tt, "启用网络代理")
	click(t, tt, "HTTP")
	click(t, tt, "保存并检测")
	if v.proxyDraft() != want || b.settingsSaves != 1 {
		t.Fatal(v.proxyDraft(), b.settingsSaves)
	}
}
