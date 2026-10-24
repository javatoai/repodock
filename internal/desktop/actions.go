package desktop

import (
	"fmt"
	"net/url"
	"repodock/internal/platform"
	"strings"

	"github.com/egoist/mygo"
)

type NativeActions struct {
	Window *mygo.Window
}

func (a *NativeActions) DetectCodex() error {
	apps, err := mygo.Shell.InstalledApps()
	if err != nil {
		return fmt.Errorf("暂时无法检测 Codex：%w", err)
	}
	for _, app := range apps {
		if strings.EqualFold(app.Name, "Codex") || strings.Contains(strings.ToLower(app.ID), "openai.codex") {
			return nil
		}
	}
	return fmt.Errorf("未检测到 Codex 桌面应用；便携安装仍可通过系统协议尝试打开")
}

func (a *NativeActions) PickDirectory(initial string) (string, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.Window, Title: "选择保存位置", ButtonLabel: "克隆到此处", DefaultPath: initial,
		Directory: true, CreateDirectories: true,
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (a *NativeActions) OpenCodex(path string) error {
	uri, err := platform.BuildCodexURI(path)
	if err != nil {
		return err
	}
	if err := mygo.Shell.OpenExternal(uri); err != nil {
		return fmt.Errorf("无法打开 Codex，请确认已安装桌面应用。项目已保留，可再次尝试：%w", err)
	}
	return nil
}

func (a *NativeActions) OpenFolder(path string) error { return mygo.Shell.OpenPath(path) }

func (a *NativeActions) OpenURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return fmt.Errorf("链接无效")
	}
	return mygo.Shell.OpenExternal(raw)
}
