package platform

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"repodock/internal/domain"
	"slices"
	"strings"
	"testing"
)

func localRemote(t *testing.T) (string, string) {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git not installed")
	}
	root := t.TempDir()
	remote, seed := filepath.Join(root, "remote.git"), filepath.Join(root, "seed")
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(git, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	run("init", "--bare", remote)
	run("init", "-b", "main", seed)
	run("-C", seed, "config", "user.name", "RepoDock Test")
	run("-C", seed, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("local fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("-C", seed, "add", "README.md")
	run("-C", seed, "commit", "-m", "fixture")
	run("-C", seed, "push", remote, "main")
	run("--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/main")
	return git, remote
}

func TestCloneRealRepositoryAndDoNotOverwrite(t *testing.T) {
	git, remote := localRemote(t)
	parent := filepath.Join(t.TempDir(), "中文 Projects")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	ref := domain.RepositoryRef{Owner: "you", Name: "project"}
	cloner := Cloner{GHPath: "fixture-gh", GitPath: git, Runner: runnerFunc(func(ctx context.Context, cmd Command, output func(string)) ([]byte, error) {
		if cmd.Path == "fixture-gh" {
			if !slices.Contains(cmd.Args, "--no-upstream") || cmd.Args[2] != ref.URL()+".git" {
				t.Fatal(cmd.Args)
			}
			cmd.Path, cmd.Args = git, []string{"clone", "--progress", "--", remote, cmd.Args[3]}
		}
		return (ExecRunner{}).Run(ctx, cmd, output)
	})}
	path, err := cloner.Clone(context.Background(), ref, parent, func(domain.Progress) {})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(path, "README.md"))
	if err != nil || string(data) != "local fixture\n" {
		t.Fatal(string(data), err)
	}
	if _, err := cloner.Clone(context.Background(), ref, parent, func(domain.Progress) {}); !errors.Is(err, ErrDestinationExists) {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(path, "README.md")); string(data) != "local fixture\n" {
		t.Fatal("existing project modified")
	}
	assertNoStaging(t, parent)
}

func TestCloneDestinationRaceNeverReplacesEmptyDirectory(t *testing.T) {
	git, remote := localRemote(t)
	parent := t.TempDir()
	cloner := Cloner{GHPath: "fixture-gh", GitPath: git, Runner: runnerFunc(func(ctx context.Context, cmd Command, emit func(string)) ([]byte, error) {
		if cmd.Path == "fixture-gh" {
			cmd.Path, cmd.Args = git, []string{"clone", "--", remote, cmd.Args[3]}
			return (ExecRunner{}).Run(ctx, cmd, emit)
		}
		data, err := (ExecRunner{}).Run(ctx, cmd, emit)
		if err == nil {
			if err := os.Mkdir(filepath.Join(parent, "project"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		return data, err
	})}
	_, err := cloner.Clone(context.Background(), domain.RepositoryRef{Owner: "you", Name: "project"}, parent, func(domain.Progress) {})
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(parent, "project"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("existing directory changed: %v %v", entries, err)
	}
	assertNoStaging(t, parent)
}

func TestCloneFailureAndCancellationCleanStaging(t *testing.T) {
	for _, cause := range []error{errors.New("authentication failed"), context.Canceled} {
		t.Run(cause.Error(), func(t *testing.T) {
			parent := t.TempDir()
			stopped := false
			cloner := Cloner{GHPath: "gh", Runner: runnerFunc(func(ctx context.Context, cmd Command, emit func(string)) ([]byte, error) {
				if err := os.WriteFile(filepath.Join(cmd.Args[3], "partial"), []byte("partial clone"), 0600); err != nil {
					t.Fatal(err)
				}
				stopped = true
				return nil, cause
			})}
			_, err := cloner.Clone(context.Background(), domain.RepositoryRef{Owner: "you", Name: "project"}, parent, func(domain.Progress) {})
			if !errors.Is(err, cause) || !stopped {
				t.Fatal(err, stopped)
			}
			if _, err := os.Stat(filepath.Join(parent, "project")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			assertNoStaging(t, parent)
		})
	}
}

func assertNoStaging(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".repodock-") {
			t.Fatalf("leaked staging: %s", entry.Name())
		}
	}
}

func TestProgressAndCredentialRedaction(t *testing.T) {
	p := ParseProgress("Receiving objects: 68% (68/100)")
	if p.Stage != "接收对象" || p.Percent != 68 {
		t.Fatal(p)
	}
	var lines []string
	w := &lineWriter{emit: func(s string) { lines = append(lines, s) }}
	_, _ = w.Write([]byte("中文进度\rReceiving objects: 10%\r\nfatal ghp_secret https://user:password@github.com/repo\n"))
	w.flush()
	if len(lines) != 3 || lines[0] != "中文进度" || strings.Contains(strings.Join(lines, ""), "password") || strings.Contains(strings.Join(lines, ""), "ghp_secret") {
		t.Fatal(lines)
	}
	for _, name := range []string{"CON", "nul.txt", "COM1", "LPT9.log", "project."} {
		if validWindowsName(name) {
			t.Fatal(name)
		}
	}
	if !validWindowsName("project.v1") {
		t.Fatal("rejected normal name")
	}
}
