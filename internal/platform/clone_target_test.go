package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"testing"
)

func TestCloneTargetChecksOnlyActualDestination(t *testing.T) {
	ref := domain.RepositoryRef{Owner: "you", Name: "project"}
	for _, kind := range []string{"absent", "empty", "populated", "file", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			parent := filepath.Join(t.TempDir(), "中文 保存目录")
			if err := os.MkdirAll(parent, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(parent, ref.Name)
			if err := os.Mkdir(filepath.Join(parent, "other-project"), 0700); err != nil {
				t.Fatal(err)
			}
			if kind == "empty" || kind == "populated" {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				if kind == "populated" {
					if err := os.WriteFile(filepath.Join(target, "keep"), []byte("keep"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if kind == "file" {
				if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			inspector := TargetInspector{}
			if kind == "unreadable" {
				inspector.stat = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
			}
			got := inspector.Inspect(context.Background(), ref, parent)
			want := map[string]string{"absent": "ready", "empty": "exists", "populated": "exists", "file": "file", "unreadable": "error"}[kind]
			if got.State != want || got.Path != target {
				t.Fatal(got)
			}
			if (want == "exists" && got.OpenDirectory != target) || (want == "file" && got.OpenDirectory != parent) {
				t.Fatal(got)
			}
			if kind != "absent" {
				called := false
				cloner := Cloner{Runner: runnerFunc(func(context.Context, Command, func(string)) ([]byte, error) {
					called = true
					return nil, errors.New("must not clone")
				})}
				if kind != "unreadable" {
					if _, err := cloner.Clone(context.Background(), ref, parent, func(domain.Progress) {}); !errors.Is(err, ErrDestinationExists) || called {
						t.Fatal(err, called)
					}
				}
			}
		})
	}
}

func TestCloneTargetNonexistentParentAndInvalidAncestor(t *testing.T) {
	root := t.TempDir()
	ref := domain.RepositoryRef{Owner: "you", Name: "repo"}
	parent := filepath.Join(root, "new", "folder")
	if got := (TargetInspector{}).Inspect(context.Background(), ref, parent); got.State != "ready" {
		t.Fatal(got)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := (TargetInspector{}).Inspect(context.Background(), ref, filepath.Join(file, "child")); got.State == "ready" {
		t.Fatal(got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := (TargetInspector{}).Inspect(ctx, ref, root); got.State == "ready" {
		t.Fatal(got)
	}
}
