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
	for _, state := range []string{"ready", "dark", "empty", "dropdown", "progress", "success", "error", "settings"} {
		view := desktop.Preview(state)
		tt := ui.NewTester(view.Render, 800, 640)
		if state == "dropdown" {
			if err := tt.Click("展开私人仓库"); err != nil {
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
	fmt.Println("已生成八种离线界面预览：" + out)
	return nil
}
