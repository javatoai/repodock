package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"repodock/internal/desktop"

	"github.com/egoist/mygo/ui"
)

func main() {
	out := flag.String("out", "artifacts", "离线预览图片目录")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out string) error {
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	states := []string{"ready", "dark", "empty", "dropdown", "cloned", "dark-cloned", "cloned-unopened", "create", "create-existing", "progress", "success", "error", "settings", "settings-dark", "occupied", "occupied-dark", "target-file", "checking", "proxy", "proxy-dark", "proxy-error"}
	for _, state := range states {
		view := desktop.Preview(state)
		tt := ui.NewTester(view.Render, 960, 680)
		tt.Frame()
		if state == "dropdown" {
			if err := tt.Click("展开 GitHub 项目"); err != nil {
				return err
			}
			tt.Frame()
		}
		f, err := os.Create(filepath.Join(out, state+".png"))
		if err != nil {
			return err
		}
		err = png.Encode(f, tt.Image())
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	narrow := []string{"ready", "cloned", "cloned-unopened", "create", "settings", "dropdown", "occupied", "occupied-dark", "target-file", "proxy", "proxy-dark", "proxy-error"}
	for _, state := range narrow {
		view := desktop.Preview(state)
		tt := ui.NewTester(view.Render, 640, 520)
		tt.Frame()
		if state == "dropdown" {
			if err := tt.Click("展开 GitHub 项目"); err != nil {
				return err
			}
			tt.Frame()
		}
		f, err := os.Create(filepath.Join(out, state+"-narrow.png"))
		if err != nil {
			return err
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			return err
		}
	}
	fmt.Printf("已生成 %d 种界面及 %d 种窄窗口离线预览：%s\n", len(states), len(narrow), out)
	return nil
}
