package platform

import (
	"net/url"
	"path/filepath"
	"testing"
)

func TestCodexURIEncodesAbsoluteWorkspace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "中文 folder #1 & 2+3?")
	raw, err := BuildCodexURI(path)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "codex" || u.Host != "threads" || u.Path != "/new" || u.Query().Get("path") != path {
		t.Fatal(raw, err)
	}
	if _, err := BuildCodexURI("relative/project"); err == nil {
		t.Fatal("accepted relative path")
	}
}
