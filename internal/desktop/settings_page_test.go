package desktop

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSettingsPageNavigationRetainsDraftAndRefreshDoesNotResetIt(t *testing.T) {
	v := Preview("ready")
	b := v.Backend.(*previewBackend)
	tt := ui.NewTester(v.Render, 960, 680)
	click(t, tt, "设置")
	if v.Page != "settings" || tt.HasText("关闭设置") || tt.HasText("搜索 GitHub 项目") {
		t.Fatal(v.Page, tt.Texts())
	}
	v.defaultDir = filepath.Join(t.TempDir(), "未保存目录")
	v.gitPath, v.ghPath = "draft-git", "draft-gh"
	wantDir := v.defaultDir
	tt.Frame()
	click(t, tt, "首页")
	if v.Page != "home" || !tt.HasText("搜索 GitHub 项目") {
		t.Fatal(v.Page, tt.Texts())
	}
	b.model.Preferences.DefaultCloneDir = filepath.Join(t.TempDir(), "new-saved-dir")
	b.model.Preferences.GitPath, b.model.Preferences.GHPath = "saved-git", "saved-gh"
	click(t, tt, "设置")
	tt.Scroll(650, 300, 0, 800)
	click(t, tt, "重新检测")
	if b.refreshes != 1 || v.defaultDir != wantDir || v.gitPath != "draft-git" || v.ghPath != "draft-gh" {
		t.Fatal(v.defaultDir, v.gitPath, v.ghPath, b.refreshes)
	}
	click(t, tt, "设置")
	if v.defaultDir != wantDir {
		t.Fatal("clicking active settings reset draft")
	}
	click(t, tt, "已克隆")
	if v.Page != "cloned" || !tt.HasText("you/atlas") {
		t.Fatal(v.Page, tt.Texts())
	}
	click(t, tt, "设置")
	if v.defaultDir != wantDir {
		t.Fatal("draft was lost after cloned page")
	}
}

func TestSettingsSaveRemainsOnPageAndPreservesInputOnFailure(t *testing.T) {
	for _, mode := range []string{"success", "save-failure", "invalid-directory"} {
		t.Run(mode, func(t *testing.T) {
			v := Preview("settings")
			b := v.Backend.(*previewBackend)
			old := b.model.Preferences.DefaultCloneDir
			v.defaultDir = filepath.Join(t.TempDir(), "项目")
			if mode == "save-failure" {
				b.settingsError = "设置未能保存：disk full"
			}
			if mode == "invalid-directory" {
				v.defaultDir = "relative-directory"
			}
			draft := v.defaultDir
			tt := ui.NewTester(v.Render, 640, 520)
			click(t, tt, "保存并检测")
			if v.Page != "settings" || v.defaultDir != draft {
				t.Fatal(v.Page, v.defaultDir)
			}
			switch mode {
			case "success":
				if b.settingsSaves != 1 || b.model.Preferences.DefaultCloneDir != draft {
					t.Fatal(b.settingsSaves, b.model.Preferences)
				}
			case "save-failure":
				if b.settingsSaves != 1 || b.model.Preferences.DefaultCloneDir != old || !tt.HasText(b.settingsError) {
					t.Fatal(b.settingsSaves, tt.Texts())
				}
			case "invalid-directory":
				if b.settingsSaves != 0 || !strings.Contains(b.model.Notice, "绝对路径") {
					t.Fatal(b.settingsSaves, b.model.Notice)
				}
			}
		})
	}
}

func TestSettingsSaveButtonPinnedInNarrowWindow(t *testing.T) {
	v := Preview("settings")
	b := v.Backend.(*previewBackend)
	b.model.Preferences.Theme = "system"
	tt := ui.NewTester(v.Render, 640, 520)
	before, found := tt.Find("保存并检测")
	if !found || before.Y < 0 || before.Y+before.H > 520 {
		t.Fatal(before, found)
	}
	v.settingsScroll.Y = 100000
	tt.Frame()
	after, found := tt.Find("保存并检测")
	if !found || before.Y != after.Y || before.H != after.H || v.settingsScroll.MaxY <= 0 {
		t.Fatal(before, after, v.settingsScroll)
	}
	draft := v.defaultDir
	light := tt.Image().At(300, 10)
	tt.SetDark(true)
	if tt.Image().At(300, 10) == light || v.defaultDir != draft {
		t.Fatal("theme change lost draft or theme did not update")
	}
}

func TestSettingsEntryPointsUseMainPageAndKeepActiveTask(t *testing.T) {
	for _, entry := range []string{"search", "clone", "create", "task"} {
		t.Run(entry, func(t *testing.T) {
			v := Preview("ready")
			b := v.Backend.(*previewBackend)
			label := "设置"
			switch entry {
			case "search":
				b.model.Account, b.model.AccountError, b.model.Repositories = "", "尚未连接 GitHub", nil
				label = "连接 GitHub"
			case "clone":
				v.SelectedURL = "https://github.com/you/new-project"
				b.model.Dependencies.GitErr = errors.New("Git missing")
				label = "配置 Git 与 gh"
			case "create":
				b.model.Account = ""
				v.startCreation(b.model)
				label = "连接 GitHub"
			case "task":
				v = Preview("progress")
				b = v.Backend.(*previewBackend)
			}
			tt := ui.NewTester(v.Render, 960, 680)
			if entry == "search" {
				click(t, tt, "搜索 GitHub 项目")
			}
			click(t, tt, label)
			if v.Page != "settings" || tt.HasText("关闭设置") || !tt.HasText("保存并检测") {
				t.Fatal(v.Page, tt.Texts())
			}
			if entry == "task" {
				if !tt.HasText("接收对象") {
					t.Fatal(tt.Texts())
				}
				click(t, tt, "保存并检测")
				if b.settingsSaves != 0 {
					t.Fatal("saved during active task")
				}
			}
		})
	}
}
