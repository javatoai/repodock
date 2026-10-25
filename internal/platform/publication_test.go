package platform

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"repodock/internal/domain"
	"slices"
	"strings"
	"testing"
)

type publicationFixture struct {
	t                                 *testing.T
	git, remote                       string
	created, commits, pushes, ghCalls int
	failPush, occupied                bool
	failPhase                         string
	private                           bool
}

func newPublicationFixture(t *testing.T) *publicationFixture {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git unavailable")
	}
	f := &publicationFixture{t: t, git: git, remote: filepath.Join(t.TempDir(), "remote.git"), private: true}
	if _, err := f.Run(context.Background(), Command{Path: git, Args: []string{"init", "--bare", f.remote}}, nil); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *publicationFixture) repository() []byte {
	data, _ := json.Marshal(remoteRepository{ID: 42, FullName: "you/project", CloneURL: "https://github.com/you/project.git", Private: f.private})
	return data
}

func (f *publicationFixture) Run(ctx context.Context, cmd Command, emit func(string)) ([]byte, error) {
	if cmd.Path == "fixture-gh" {
		f.ghCalls++
		if slices.Contains(cmd.Args, "user") {
			if f.failPhase == "account" {
				f.failPhase = ""
				return nil, errors.New("not logged in")
			}
			return []byte(`{"login":"you"}`), nil
		}
		if slices.Contains(cmd.Args, "POST") {
			if f.failPhase == "create-unknown" {
				f.failPhase = ""
				f.created++
				return nil, errors.New("response lost after remote creation")
			}
			if f.failPhase == "create" {
				f.failPhase = ""
				return nil, errors.New("connection lost")
			}
			f.created++
			f.private = slices.Contains(cmd.Args, "private=true")
			return f.repository(), nil
		}
		if f.created > 0 || f.occupied {
			return append([]byte("HTTP/2.0 200 OK\r\nContent-Type: application/json\r\n\r\n"), f.repository()...), nil
		}
		if f.failPhase == "lookup" {
			f.failPhase = ""
			return []byte("HTTP/2.0 403 Forbidden\r\n\r\n{}"), errors.New("HTTP 403")
		}
		return []byte("HTTP/2.0 404 Not Found\r\n\r\n{}"), errors.New("HTTP 404")
	}
	cmd.Path = f.git
	if slices.Contains(cmd.Args, "commit") {
		f.commits++
		if f.failPhase == "commit" {
			f.failPhase = ""
			return nil, errors.New("commit hook failed")
		}
	}
	if slices.Contains(cmd.Args, "GIT_AUTHOR_IDENT") && f.failPhase == "identity" {
		f.failPhase = ""
		return nil, errors.New("identity unknown")
	}
	if slices.Contains(cmd.Args, "ls-remote") && f.failPhase == "verify" {
		f.failPhase = ""
		return nil, errors.New("verification network error")
	}
	if slices.Contains(cmd.Args, "push") {
		f.pushes++
		if f.failPush {
			f.failPush = false
			return nil, errors.New("network failed")
		}
	}
	if slices.Contains(cmd.Args, "push") || slices.Contains(cmd.Args, "ls-remote") {
		cmd.Args = slices.Clone(cmd.Args)
		for i, arg := range cmd.Args {
			if arg == "origin" {
				cmd.Args[i] = f.remote
			}
		}
	}
	cmd.Args = append([]string{"-c", "user.name=RepoDock Test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false"}, cmd.Args...)
	return (ExecRunner{}).Run(ctx, cmd, emit)
}

func (f *publicationFixture) publisher() *Publisher {
	return &Publisher{Runner: f, GitPath: f.git, GHPath: "fixture-gh"}
}

func TestPublishNewProjectAndResumeAfterPushFailure(t *testing.T) {
	f := newPublicationFixture(t)
	f.failPush = true
	state := domain.PublishState{Owner: "you", Request: domain.PublishRequest{Name: "project", Parent: t.TempDir(), Private: true}}
	var saved domain.PublishState
	checkpoint := func(s domain.PublishState) error { saved = s; return nil }
	result, err := f.publisher().Publish(context.Background(), state, checkpoint, func(domain.Progress) {})
	if err == nil || result.RemoteID != 42 || saved.RemoteID != 42 || saved.Published {
		t.Fatal(result, err)
	}
	result, err = f.publisher().Publish(context.Background(), saved, checkpoint, func(domain.Progress) {})
	if err != nil || !result.Published || f.created != 1 || f.commits != 1 || f.pushes != 2 {
		t.Fatal(result, err, f.created, f.commits, f.pushes)
	}
	head, err := f.Run(context.Background(), Command{Path: f.git, Args: []string{"--git-dir", f.remote, "rev-parse", "refs/heads/main"}}, nil)
	if err != nil || strings.TrimSpace(string(head)) != result.Commit {
		t.Fatal(string(head), err)
	}
	if _, err := os.Stat(filepath.Join(result.Path, "README.md")); err != nil {
		t.Fatal(err)
	}
	_, err = f.publisher().Publish(context.Background(), saved, checkpoint, func(domain.Progress) {})
	if err != nil || f.created != 1 || f.pushes != 2 {
		t.Fatal("completed job repeated publication", err)
	}
}

func TestPublicationPreflightRejectsExistingRemoteAndOccupiedName(t *testing.T) {
	for _, kind := range []string{"remote", "name"} {
		t.Run(kind, func(t *testing.T) {
			f := newPublicationFixture(t)
			path := filepath.Join(t.TempDir(), "project")
			state := domain.PublishState{Owner: "you", Request: domain.PublishRequest{Name: "project", Parent: filepath.Dir(path), Private: true}}
			if kind == "remote" {
				os.Mkdir(path, 0755)
				for _, args := range [][]string{{"init", "-b", "feature", path}, {"-C", path, "remote", "add", "origin", "https://github.com/you/existing.git"}} {
					if _, err := f.Run(context.Background(), Command{Args: args}, nil); err != nil {
						t.Fatal(err)
					}
				}
				state.Request.Existing, state.Request.Path = true, path
			} else {
				f.occupied = true
			}
			_, err := f.publisher().Publish(context.Background(), state, func(domain.PublishState) error { return nil }, func(domain.Progress) {})
			if err == nil || f.created != 0 || f.commits != 0 || f.pushes != 0 {
				t.Fatal(err, f.created, f.commits)
			}
			if kind == "remote" && f.ghCalls != 0 {
				t.Fatal("remote rejection was not local")
			}
			if kind == "name" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("occupied GitHub name mutated local directory")
				}
			}
		})
	}
}

func TestPublishExistingDirectoryPreviewRespectsIgnoreAndPreservesBranch(t *testing.T) {
	f := newPublicationFixture(t)
	path := t.TempDir()
	for name, data := range map[string]string{".gitignore": "ignored.txt\n", "code.go": "package example\n", "ignored.txt": "ignore me\n"} {
		if err := os.WriteFile(filepath.Join(path, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	request := domain.PublishRequest{Name: "project", Path: path, Existing: true, Private: true}
	files, err := f.publisher().Inspect(context.Background(), request, "you")
	if err != nil || !slices.Contains(files, "code.go") || slices.Contains(files, "ignored.txt") {
		t.Fatal(files, err)
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); !os.IsNotExist(err) {
		t.Fatal("preview initialized user directory")
	}
	for _, args := range [][]string{{"init", "-b", "feature", path}, {"-C", path, "add", "."}, {"-C", path, "commit", "-m", "original"}} {
		if _, err := f.Run(context.Background(), Command{Args: args}, nil); err != nil {
			t.Fatal(err)
		}
	}
	f.commits = 0
	result, err := f.publisher().Publish(context.Background(), domain.PublishState{Owner: "you", Request: request}, func(domain.PublishState) error { return nil }, func(domain.Progress) {})
	if err != nil || result.Branch != "feature" || f.commits != 0 {
		t.Fatal(result, err, f.commits)
	}
}

func TestPublicationStopsWhenCheckpointCannotBeSaved(t *testing.T) {
	f := newPublicationFixture(t)
	_, err := f.publisher().Publish(context.Background(), domain.PublishState{Owner: "you", Request: domain.PublishRequest{Name: "project", Parent: t.TempDir(), Private: true}}, func(domain.PublishState) error { return errors.New("disk full") }, func(domain.Progress) {})
	if err == nil || f.created != 0 || f.commits != 0 || f.pushes != 0 {
		t.Fatal(err, f.created, f.commits)
	}
}

func TestPublicationFailureStagesAndRetry(t *testing.T) {
	for _, stage := range []string{"identity", "account", "lookup", "commit", "create", "verify"} {
		t.Run(stage, func(t *testing.T) {
			f := newPublicationFixture(t)
			f.failPhase = stage
			state := domain.PublishState{Owner: "you", Request: domain.PublishRequest{Name: "project", Parent: t.TempDir(), Private: true}}
			saved := state
			checkpoint := func(s domain.PublishState) error { saved = s; return nil }
			_, err := f.publisher().Publish(context.Background(), state, checkpoint, func(domain.Progress) {})
			if err == nil {
				t.Fatal("failure ignored", stage)
			}
			if stage != "verify" && (f.created != 0 || f.pushes != 0) {
				t.Fatal("failure continued", stage, f.created, f.pushes)
			}
			result, err := f.publisher().Publish(context.Background(), saved, checkpoint, func(domain.Progress) {})
			if err != nil || !result.Published || f.created != 1 {
				t.Fatal(result, err, f.created)
			}
			count, err := f.Run(context.Background(), Command{Dir: result.Path, Args: []string{"rev-list", "--count", "HEAD"}}, nil)
			if err != nil || strings.TrimSpace(string(count)) != "1" {
				t.Fatal(string(count), err)
			}
		})
	}
}

func TestPublishPlainDirectoryAsPublicRepository(t *testing.T) {
	f := newPublicationFixture(t)
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "hello.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	state, err := f.publisher().Publish(context.Background(), domain.PublishState{Owner: "you", Request: domain.PublishRequest{Name: "project", Path: path, Existing: true, Private: false}}, func(domain.PublishState) error { return nil }, func(domain.Progress) {})
	if err != nil || !state.Published || f.private || state.Branch != "main" {
		t.Fatal(state, err)
	}
}

func TestUnknownCreationOutcomeDoesNotAdoptOrPushExistingRepository(t *testing.T) {
	f := newPublicationFixture(t)
	f.failPhase = "create-unknown"
	state := domain.PublishState{Owner: "you", Request: domain.PublishRequest{Name: "project", Parent: t.TempDir(), Private: true}}
	saved := state
	checkpoint := func(s domain.PublishState) error { saved = s; return nil }
	_, err := f.publisher().Publish(context.Background(), state, checkpoint, func(domain.Progress) {})
	if err == nil || f.created != 1 || saved.RemoteID != 0 || !saved.RemoteAttempted {
		t.Fatal(saved, err)
	}
	_, err = f.publisher().Publish(context.Background(), saved, checkpoint, func(domain.Progress) {})
	if err == nil || f.created != 1 || f.pushes != 0 {
		t.Fatal("unknown repository reused", err, f.created, f.pushes)
	}
}
