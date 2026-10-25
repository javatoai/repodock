package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"runtime"
)

type TargetInspector struct {
	stat func(string) (os.FileInfo, error)
}

func CloneTargetPath(ref domain.RepositoryRef, parent string) (string, error) {
	checked, err := domain.ParseRepository(ref.URL())
	if err != nil || checked != ref {
		return "", fmt.Errorf("仓库地址无效")
	}
	if runtime.GOOS == "windows" && !validWindowsName(ref.Name) {
		return "", fmt.Errorf("仓库名不能作为 Windows 文件夹名")
	}
	if !filepath.IsAbs(parent) {
		return "", fmt.Errorf("请选择绝对路径的保存文件夹")
	}
	return filepath.Join(filepath.Clean(parent), ref.Name), nil
}

func (i TargetInspector) Inspect(ctx context.Context, ref domain.RepositoryRef, parent string) domain.CloneTarget {
	result := domain.CloneTarget{Repository: ref, Parent: parent, State: "error"}
	target, err := CloneTargetPath(ref, parent)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.Path = target
	if err = ctx.Err(); err != nil {
		result.Message = err.Error()
		return result
	}
	stat := i.stat
	if stat == nil {
		stat = os.Lstat
	}
	info, err := stat(target)
	if os.IsNotExist(err) {
		// Nonexistent ancestors are allowed, but an ancestor that is a file or
		// unreadable must not be mistaken for a free destination.
		ancestor := filepath.Clean(parent)
		for {
			parentInfo, parentErr := stat(ancestor)
			if parentErr == nil {
				if !parentInfo.IsDir() {
					result.Message = "保存位置不是文件夹"
					result.OpenDirectory = filepath.Dir(ancestor)
					return result
				}
				result.State = "ready"
				return result
			}
			if !os.IsNotExist(parentErr) {
				result.Message = "无法检查保存位置：" + parentErr.Error()
				return result
			}
			next := filepath.Dir(ancestor)
			if next == ancestor {
				result.Message = "无法访问保存位置"
				return result
			}
			ancestor = next
		}
	}
	if err != nil {
		result.Message = "无法检查目标目录：" + err.Error()
		return result
	}
	if info.IsDir() {
		result.State, result.Message, result.OpenDirectory = "exists", "目标目录已存在，请更改保存位置", target
	} else {
		result.State, result.Message, result.OpenDirectory = "file", "目标路径已存在，请更改保存位置", filepath.Dir(target)
	}
	return result
}
