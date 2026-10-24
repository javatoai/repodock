package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"repodock/internal/application"
	"runtime"
	"strings"
	"time"
)

type ToolResolver struct{}

func (ToolResolver) Resolve(gitPath, ghPath string) application.Dependencies {
	git, gitErr := resolveTool("git", gitPath)
	gh, ghErr := resolveTool("gh", ghPath)
	if gitErr == nil {
		gitErr = validateTool(git, "git version")
	}
	if ghErr == nil {
		ghErr = validateTool(gh, "gh version")
	}
	return application.Dependencies{GitPath: git, GHPath: gh, GitErr: gitErr, GHErr: ghErr}
}

func validateTool(path, prefix string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := (ExecRunner{}).Run(ctx, Command{Path: path, Args: []string{"--version"}, Env: commandEnvironment("")}, nil)
	if err != nil {
		return fmt.Errorf("检查工具版本失败：%w", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(out)), prefix) {
		return fmt.Errorf("所选文件不是预期的 %s 工具", strings.Fields(prefix)[0])
	}
	return nil
}

func resolveTool(name, configured string) (string, error) {
	if configured != "" {
		if !filepath.IsAbs(configured) {
			return "", fmt.Errorf("%s 路径必须是绝对路径", name)
		}
		return executable(configured)
	}
	if path, err := exec.LookPath(name); err == nil {
		return filepath.Abs(path)
	}
	var candidates []string
	if runtime.GOOS == "darwin" {
		candidates = []string{"/opt/homebrew/bin/" + name, "/usr/local/bin/" + name, "/usr/bin/" + name}
	} else if runtime.GOOS == "windows" {
		for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			base := os.Getenv(key)
			if base == "" {
				continue
			}
			candidates = append(candidates, filepath.Join(base, "Git", "cmd", "git.exe"))
			if name == "gh" {
				candidates[len(candidates)-1] = filepath.Join(base, "GitHub CLI", "gh.exe")
			}
		}
	}
	for _, path := range candidates {
		if path, err := executable(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 %s，请安装或指定可执行文件", name)
}

func executable(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
		return "", fmt.Errorf("无法使用可执行文件 %s", path)
	}
	return path, nil
}
