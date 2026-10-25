package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"sync/atomic"
	"testing"
)

type targetFunc func(context.Context, domain.RepositoryRef, string) domain.CloneTarget

func (f targetFunc) Inspect(ctx context.Context, ref domain.RepositoryRef, parent string) domain.CloneTarget {
	return f(ctx, ref, parent)
}

type proxyTestFunc func(context.Context, domain.ProxyConfig) error

func (f proxyTestFunc) Test(ctx context.Context, config domain.ProxyConfig) error {
	return f(ctx, config)
}

func TestTargetChecksDiscardStaleSelectionsAndRecheckSamePath(t *testing.T) {
	c, changed := newTestController(t, &memoryStore{prefs: domain.DefaultPreferences()}, actionsStub{}, githubStub{}, nil)
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c.factory = func(Dependencies) Services {
		return Services{Targets: targetFunc(func(_ context.Context, ref domain.RepositoryRef, parent string) domain.CloneTarget {
			state := "ready"
			if parent == first {
				close(started)
				<-release
				state = "file"
			} else if calls.Add(1) > 1 {
				state = "exists"
			}
			return domain.CloneTarget{Repository: ref, Parent: parent, State: state}
		})}
	}
	c.CheckCloneTarget("https://github.com/you/repo", first)
	<-started
	if c.Snapshot().CloneTarget.State != "checking" {
		t.Fatal(c.Snapshot().CloneTarget)
	}
	c.CheckCloneTarget("https://github.com/you/repo", second)
	await(t, c, changed, func(m Model) bool { return m.CloneTarget.Parent == second && m.CloneTarget.State == "ready" })
	close(release)
	c.workers.Wait()
	if c.Snapshot().CloneTarget.Parent != second || c.Snapshot().CloneTarget.State != "ready" {
		t.Fatal(c.Snapshot().CloneTarget)
	}
	c.RecheckCloneTarget()
	await(t, c, changed, func(m Model) bool { return m.CloneTarget.State == "exists" })
}

func TestCloneBackendRechecksBeforeCreatingParent(t *testing.T) {
	c, changed := newTestController(t, &memoryStore{prefs: domain.DefaultPreferences()}, actionsStub{}, githubStub{}, nil)
	parent := filepath.Join(t.TempDir(), "must-not-create")
	c.factory = func(Dependencies) Services {
		return Services{GitHub: githubStub{}, Targets: targetFunc(func(_ context.Context, ref domain.RepositoryRef, parent string) domain.CloneTarget {
			return domain.CloneTarget{Repository: ref, Parent: parent, State: "exists", Message: "目标目录已存在"}
		}), Cloner: cloneFunc(func(context.Context, domain.RepositoryRef, string, func(domain.Progress)) (string, error) {
			t.Error("cloner invoked after conflict")
			return "", errors.New("unexpected")
		})}
	}
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return m.Complete })
	c.BeginClone("https://github.com/you/repo", parent)
	await(t, c, changed, func(m Model) bool { return m.Task.Status == "error" })
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatal("directory created before check", err)
	}
}

func TestRemovingLocalRecordClearsHistoryAndNeverDeletesFiles(t *testing.T) {
	root := t.TempDir()
	store := &memoryStore{prefs: domain.DefaultPreferences()}
	store.prefs.Remember(domain.RecentProject{FullName: "https://github.com/you/repo", Path: root})
	c, changed := newTestController(t, store, actionsStub{}, githubStub{}, nil)
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return m.Complete })
	c.RemoveLocalProject(root)
	m := c.Snapshot()
	if len(m.Preferences.Recent) != 0 || len(m.Preferences.LocalProjects) != 0 || len(m.Available) != 0 {
		t.Fatal(m.Preferences, m.Available)
	}
	store.prefs.Normalize()
	if len(store.prefs.LocalProjects) != 0 {
		t.Fatal("record resurrected", store.prefs)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal("deleted project", err)
	}
}

func TestProxyDraftTestingAndSavedSnapshot(t *testing.T) {
	store := &memoryStore{prefs: domain.DefaultPreferences()}
	c, changed := newTestController(t, store, actionsStub{}, githubStub{}, nil)
	captured := make(chan Dependencies, 8)
	tested := make(chan domain.ProxyConfig, 1)
	c.factory = func(deps Dependencies) Services {
		captured <- deps
		return Services{GitHub: githubStub{}, ProxyTester: proxyTestFunc(func(_ context.Context, config domain.ProxyConfig) error { tested <- config; return nil })}
	}
	draft := domain.ProxyConfig{Enabled: true, URL: "socks5://localhost:1080"}
	c.TestProxy(draft)
	await(t, c, changed, func(m Model) bool { return !m.ProxyTest.Loading && m.ProxyTest.Message != "" })
	if got := <-tested; got != draft {
		t.Fatal(got)
	}
	if c.Snapshot().Preferences.Proxy.Enabled {
		t.Fatal("test saved draft")
	}
	<-captured
	c.ApplySettings("git", "gh", t.TempDir(), draft)
	await(t, c, changed, func(m Model) bool { return m.Complete })
	if got := <-captured; got.Proxy != draft {
		t.Fatal(got)
	}
	if store.prefs.Proxy != draft {
		t.Fatal(store.prefs.Proxy)
	}
	c.mu.Lock()
	c.model.Task = Task{Status: "publishing"}
	c.mu.Unlock()
	c.ApplySettings("git", "gh", t.TempDir(), domain.DefaultProxy())
	c.TestProxy(domain.DefaultProxy())
	if c.Snapshot().Preferences.Proxy != draft || c.Snapshot().ProxyTest.Config != draft {
		t.Fatal("busy task settings changed")
	}
}

func TestFailedProxySaveRetainsCurrentRuntimeSettings(t *testing.T) {
	store := &memoryStore{prefs: domain.DefaultPreferences(), fail: true}
	c, _ := newTestController(t, store, actionsStub{}, githubStub{}, nil)
	c.ApplySettings("git", "gh", t.TempDir(), domain.ProxyConfig{Enabled: true, URL: domain.DefaultProxyURL})
	if c.Snapshot().Preferences.Proxy.Enabled || c.Snapshot().Notice == "" {
		t.Fatal(c.Snapshot())
	}
}

func TestFailedRemovalKeepsVisibleRecord(t *testing.T) {
	store := &memoryStore{prefs: domain.DefaultPreferences()}
	path := t.TempDir()
	store.prefs.Remember(domain.RecentProject{FullName: "you/repo", Path: path})
	store.fail = true
	c, _ := newTestController(t, store, actionsStub{}, githubStub{}, nil)
	c.RemoveLocalProject(path)
	if len(c.Snapshot().Preferences.LocalProjects) != 1 || len(c.Snapshot().Preferences.Recent) != 1 || c.Snapshot().Notice == "" {
		t.Fatal(c.Snapshot())
	}
}

type folderActions struct {
	actionsStub
	folders chan string
}

func (a folderActions) OpenFolder(path string) error { a.folders <- path; return nil }

func TestOfflineLocalOpenAndExplorerAreIndependentOfGH(t *testing.T) {
	path := t.TempDir()
	prefs := domain.DefaultPreferences()
	prefs.RememberLocal(domain.RecentProject{FullName: "you/repo", Path: path})
	c, changed := newTestController(t, &memoryStore{prefs: prefs}, actionsStub{}, githubStub{}, nil)
	folders, opened := make(chan string, 1), make(chan string, 1)
	c.actions = folderActions{actionsStub: actionsStub{opened: opened}, folders: folders}
	c.resolver = resolverFunc(func(string, string) Dependencies { return Dependencies{GHErr: errors.New("gh unavailable")} })
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return !m.Loading && m.AccountError != "" })
	c.OpenProject(path, true)
	if got := <-folders; got != path {
		t.Fatal(got)
	}
	c.workers.Wait()
	if len(opened) != 0 || c.Snapshot().OpenSequence != 0 || len(c.Snapshot().Preferences.Recent) != 0 {
		t.Fatal("Explorer dispatched Codex")
	}
	c.OpenProject(path, false)
	await(t, c, changed, func(m Model) bool { return m.OpenSequence == 1 })
	if got := <-opened; got != path {
		t.Fatal(got)
	}
}
