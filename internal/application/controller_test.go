package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	prefs domain.Preferences
	fail  bool
}

func (s *memoryStore) Load() (domain.Preferences, error) { return s.prefs, nil }
func (s *memoryStore) Save(p domain.Preferences) error {
	if s.fail {
		return errors.New("disk full")
	}
	s.prefs = p
	return nil
}

type resolverFunc func(string, string) Dependencies

func (f resolverFunc) Resolve(git, gh string) Dependencies { return f(git, gh) }

type cloneFunc func(context.Context, domain.RepositoryRef, string, func(domain.Progress)) (string, error)

func (f cloneFunc) Clone(ctx context.Context, r domain.RepositoryRef, p string, report func(domain.Progress)) (string, error) {
	return f(ctx, r, p, report)
}

type githubStub struct {
	account func(context.Context) (string, error)
	list    func(context.Context, string, func([]domain.Repository)) error
}

func (g githubStub) Account(ctx context.Context) (string, error) {
	if g.account != nil {
		return g.account(ctx)
	}
	return "you", nil
}
func (g githubStub) ListPrivate(ctx context.Context, a string, page func([]domain.Repository)) error {
	if g.list != nil {
		return g.list(ctx, a, page)
	}
	return nil
}

type actionsStub struct {
	parent  string
	openErr error
	opened  chan string
	pick    func() (string, error)
}

func (a actionsStub) PickDirectory(string) (string, error) {
	if a.pick != nil {
		return a.pick()
	}
	return a.parent, nil
}
func (a actionsStub) OpenCodex(p string) error {
	if a.opened != nil {
		a.opened <- p
	}
	return a.openErr
}
func (a actionsStub) OpenFolder(string) error { return nil }
func (a actionsStub) OpenURL(string) error    { return nil }

func newTestController(t *testing.T, store *memoryStore, actions actionsStub, gh GitHub, cloner Cloner) (*Controller, chan struct{}) {
	t.Helper()
	changed := make(chan struct{}, 128)
	c := NewController(store, actions, resolverFunc(func(string, string) Dependencies { return Dependencies{GitPath: "git", GHPath: "gh"} }), func(Dependencies) Services { return Services{GitHub: gh, Cloner: cloner} }, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	t.Cleanup(c.Close)
	return c, changed
}

func await(t *testing.T, c *Controller, changed chan struct{}, condition func(Model) bool) Model {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		m := c.Snapshot()
		if condition(m) {
			return m
		}
		select {
		case <-changed:
		case <-deadline.C:
			t.Fatalf("state did not settle: %+v", m)
		}
	}
}

func TestControllerSingleCloneCancellationAndHistory(t *testing.T) {
	parent := t.TempDir()
	started := make(chan struct{}, 1)
	c, changed := newTestController(t, &memoryStore{prefs: domain.DefaultPreferences()}, actionsStub{parent: parent}, githubStub{}, cloneFunc(func(ctx context.Context, r domain.RepositoryRef, p string, report func(domain.Progress)) (string, error) {
		started <- struct{}{}
		report(domain.Progress{Stage: "接收对象", Percent: 25})
		<-ctx.Done()
		return "", ctx.Err()
	}))
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return m.Complete })
	c.BeginClone("https://github.com/you/project")
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("clone did not start")
	}
	c.BeginClone("https://github.com/you/another")
	if c.Snapshot().Task.Repository.Name != "project" {
		t.Fatal("second clone replaced active clone")
	}
	c.CancelClone()
	m := await(t, c, changed, func(m Model) bool { return m.Task.Status == "cancelled" })
	if len(m.Preferences.Recent) != 0 {
		t.Fatal("cancelled clone entered history")
	}
}

func TestControllerSuccessfulCloneOpenFailureAndRecordRemoval(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "project")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{prefs: domain.DefaultPreferences()}
	opened := make(chan string, 1)
	c, changed := newTestController(t, store, actionsStub{parent: parent, opened: opened, openErr: errors.New("Codex unavailable")}, githubStub{}, cloneFunc(func(context.Context, domain.RepositoryRef, string, func(domain.Progress)) (string, error) {
		return path, nil
	}))
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return m.Complete })
	c.BeginClone("https://github.com/you/project")
	m := await(t, c, changed, func(m Model) bool { return m.Task.Status == "success" })
	if len(m.Preferences.Recent) != 1 || m.Preferences.Recent[0].Path != path {
		t.Fatal(m.Preferences.Recent)
	}
	c.OpenProject(path, false)
	select {
	case got := <-opened:
		if got != path {
			t.Fatal(got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("not opened")
	}
	await(t, c, changed, func(m Model) bool { return m.Notice == "Codex unavailable" })
	c.RemoveRecent(path)
	if len(c.Snapshot().Preferences.Recent) != 0 {
		t.Fatal("record not removed")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("removing record deleted checkout")
	}
}

func TestControllerPreservesPartialResultsAndClearsSwitchedAccount(t *testing.T) {
	for _, cause := range []error{errors.New("network failed"), ErrAccountChanged} {
		t.Run(cause.Error(), func(t *testing.T) {
			gh := githubStub{list: func(ctx context.Context, a string, page func([]domain.Repository)) error {
				page([]domain.Repository{{Ref: domain.RepositoryRef{Owner: "you", Name: "private"}}})
				return cause
			}}
			c, changed := newTestController(t, &memoryStore{prefs: domain.DefaultPreferences()}, actionsStub{}, gh, nil)
			c.Refresh()
			m := await(t, c, changed, func(m Model) bool { return !m.Loading && m.AccountError != "" })
			if m.Complete {
				t.Fatal("partial list marked complete")
			}
			if errors.Is(cause, ErrAccountChanged) {
				if len(m.Repositories) != 0 || m.Account != "" {
					t.Fatal("old account leaked", m)
				}
			} else if len(m.Repositories) != 1 {
				t.Fatal("partial list lost")
			}
		})
	}
}

func TestControllerDiscardsLateResultsFromOlderRefresh(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	firstStarted, release := make(chan struct{}), make(chan struct{})
	gh := githubStub{account: func(ctx context.Context) (string, error) {
		mu.Lock()
		calls++
		number := calls
		mu.Unlock()
		if number == 1 {
			close(firstStarted)
			<-release
			return "old", nil
		}
		return "new", nil
	}}
	c, changed := newTestController(t, &memoryStore{prefs: domain.DefaultPreferences()}, actionsStub{}, gh, nil)
	c.Refresh()
	<-firstStarted
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return m.Account == "new" && m.Complete })
	close(release)
	c.Close()
	if c.Snapshot().Account != "new" {
		t.Fatal("old account overwrote new account")
	}
}

func TestControllerPickerCancellationAndSaveFailure(t *testing.T) {
	store := &memoryStore{prefs: domain.DefaultPreferences(), fail: true}
	c, changed := newTestController(t, store, actionsStub{}, githubStub{}, nil)
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return m.Complete })
	c.BeginClone("https://github.com/you/project")
	await(t, c, changed, func(m Model) bool { return m.Task.Status == "cancelled" })
	c.SetTheme("dark")
	if c.Snapshot().Notice == "" || c.Snapshot().Preferences.Theme != "dark" {
		t.Fatal(c.Snapshot())
	}
}
