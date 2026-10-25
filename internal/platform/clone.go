package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"repodock/internal/domain"
	"strconv"
	"strings"
)

var ErrDestinationExists = errors.New("同名目录已存在，请选择其他保存位置")

type Cloner struct {
	Runner  Runner
	GHPath  string
	GitPath string
}

func getPath() string { return os.Getenv("PATH") }

func (s *Cloner) Clone(ctx context.Context, ref domain.RepositoryRef, parent string, report func(domain.Progress)) (result string, resultErr error) {
	target, err := CloneTargetPath(ref, parent)
	if err != nil {
		return "", err
	}
	parent = filepath.Clean(parent)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return "", fmt.Errorf("保存文件夹不存在或无法访问")
	}
	targetStatus := (TargetInspector{}).Inspect(ctx, ref, parent)
	if targetStatus.State == "exists" || targetStatus.State == "file" {
		return "", ErrDestinationExists
	} else if targetStatus.State != "ready" {
		return "", fmt.Errorf("%s", targetStatus.Message)
	}
	stage, err := os.MkdirTemp(parent, ".repodock-"+ref.Name+"-")
	if err != nil {
		return "", fmt.Errorf("无法在此位置创建目录：%w", err)
	}
	defer func() {
		if err := os.RemoveAll(stage); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("临时目录未能清理 %s：%w", stage, err))
		}
	}()
	report(domain.Progress{Stage: "连接仓库", Percent: -1})
	_, err = s.Runner.Run(ctx, Command{Path: s.GHPath, Args: []string{"repo", "clone", ref.URL() + ".git", stage, "--no-upstream", "--", "--progress"}, Env: commandEnvironment(s.GitPath)}, func(line string) {
		report(ParseProgress(line))
	})
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Confirm that a successful external process actually produced a checkout.
	report(domain.Progress{Stage: "验证仓库", Percent: -1})
	if _, err := s.Runner.Run(ctx, Command{Path: s.GitPath, Args: []string{"-C", stage, "rev-parse", "--show-toplevel"}, Env: commandEnvironment(s.GitPath)}, nil); err != nil {
		return "", fmt.Errorf("克隆后的仓库验证失败：%w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	report(domain.Progress{Stage: "保存项目", Percent: -1})
	if err := renameNoReplace(stage, target); err != nil {
		if _, statErr := os.Lstat(target); statErr == nil {
			return "", ErrDestinationExists
		}
		return "", fmt.Errorf("保存克隆结果：%w", err)
	}
	return target, nil
}

var percentPattern = regexp.MustCompile(`(\d{1,3})%`)

func ParseProgress(line string) domain.Progress {
	line = Redact(line)
	p := domain.Progress{Stage: "克隆中", Percent: -1, Line: line}
	for _, part := range []struct{ match, label string }{
		{"Enumerating objects", "枚举对象"}, {"Counting objects", "统计对象"}, {"Compressing objects", "压缩对象"},
		{"Receiving objects", "接收对象"}, {"Resolving deltas", "解析差异"}, {"Updating files", "检出文件"}, {"Checking out files", "检出文件"},
	} {
		if strings.Contains(line, part.match) {
			p.Stage = part.label
			if match := percentPattern.FindStringSubmatch(line); len(match) == 2 {
				p.Percent, _ = strconv.Atoi(match[1])
				p.Percent = min(100, p.Percent)
			}
			break
		}
	}
	return p
}

func validWindowsName(name string) bool {
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return false
	}
	base := strings.ToUpper(strings.Split(name, ".")[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	return !(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9')
}
