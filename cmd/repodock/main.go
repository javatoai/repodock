package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"repodock/internal/desktop"
	"repodock/internal/storage"
)

var version = "0.1.0"

func main() {
	dir := flag.String("config-dir", "", "使用独立的设置目录")
	showVersion := flag.Bool("version", false, "显示版本")
	flag.Parse()
	if *showVersion {
		fmt.Println("RepoDock " + version)
		return
	}
	path, err := storage.DefaultPath()
	if *dir != "" {
		path, err = filepath.Abs(filepath.Join(*dir, "state.json"))
	}
	if err == nil {
		err = desktop.Run(path)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
