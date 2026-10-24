package platform

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func BuildCodexURI(path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("项目路径必须是绝对路径")
	}
	encoded := strings.ReplaceAll(url.QueryEscape(filepath.Clean(path)), "+", "%20")
	return "codex://threads/new?path=" + encoded, nil
}
