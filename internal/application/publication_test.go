package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"testing"
)

type publisherStub struct {
	path  string
	calls int
}

func (p *publisherStub) Inspect(context.Context, domain.PublishRequest, string) ([]string, error) {
	return []string{"README.md"}, nil
}
func (p *publisherStub) Publish(_ context.Context, state domain.PublishState, checkpoint func(domain.PublishState) error, report func(domain.Progress)) (domain.PublishState, error) {
	p.calls++
	state.Path, state.Published, state.RemoteID = p.path, true, 42
	report(domain.Progress{Stage: "核对远程提交", Percent: -1})
	return state, checkpoint(state)
}

func TestPublishLaunchFailureRetriesOnlyCodex(t *testing.T) {
	path := t.TempDir()
	store := &memoryStore{prefs: domain.DefaultPreferences()}
	publisher := &publisherStub{path: path}
	c, changed := newTestController(t, store, actionsStub{openErr: errors.New("Codex missing")}, githubStub{}, nil)
	c.factory = func(Dependencies) Services { return Services{GitHub: githubStub{}, Publisher: publisher} }
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return m.Complete })
	request := domain.PublishRequest{Name: "project", Parent: filepath.Dir(path), Private: true}
	c.BeginPublish(request)
	m := await(t, c, changed, func(m Model) bool { return m.Task.Status == "error" })
	if publisher.calls != 1 || m.Preferences.Publication == nil || !m.Preferences.Publication.Published || len(m.Preferences.Recent) != 0 || len(m.Preferences.LocalProjects) != 1 {
		t.Fatal(m, publisher.calls)
	}
	preparedAt := m.Preferences.LocalProjects[0].PreparedAt
	if preparedAt.IsZero() {
		t.Fatal("publication missing preparation time after Codex failure")
	}
	c.mu.Lock()
	c.actions = actionsStub{}
	c.model.Account = ""
	c.model.Dependencies.GHErr = errors.New("gh missing")
	c.mu.Unlock()
	c.BeginPublish(request)
	m = await(t, c, changed, func(m Model) bool { return m.Task.Status == "success" })
	if publisher.calls != 1 || len(m.Preferences.Recent) != 1 || m.OpenedPath != path || m.OpenSequence != 1 || m.Preferences.Publication != nil {
		t.Fatal(m, publisher.calls)
	}
	if !m.Preferences.LocalProjects[0].PreparedAt.Equal(preparedAt) {
		t.Fatal("open retry changed preparation order")
	}
}

func TestCloneOverrideDoesNotChangeDefaultAndOpenRecordsSeparately(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "project")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	prefs := domain.DefaultPreferences()
	prefs.DefaultCloneDir = filepath.Join(t.TempDir(), "default")
	c, changed := newTestController(t, &memoryStore{prefs: prefs}, actionsStub{pick: func() (string, error) { t.Error("unexpected picker"); return "", nil }}, githubStub{}, cloneFunc(func(_ context.Context, _ domain.RepositoryRef, chosen string, _ func(domain.Progress)) (string, error) {
		if chosen != parent {
			t.Error(chosen)
		}
		return path, nil
	}))
	c.Refresh()
	await(t, c, changed, func(m Model) bool { return m.Complete })
	c.BeginClone("https://github.com/you/project", parent)
	m := await(t, c, changed, func(m Model) bool { return m.Task.Status == "success" })
	if m.Preferences.DefaultCloneDir != prefs.DefaultCloneDir || len(m.Preferences.Recent) != 0 || len(m.Preferences.LocalProjects) != 1 {
		t.Fatal(m)
	}
	c.OpenProject(path, false)
	m = await(t, c, changed, func(m Model) bool { return m.OpenSequence == 1 })
	if len(m.Preferences.Recent) != 1 {
		t.Fatal(m)
	}
	c.RemoveRecent(path)
	if len(c.Snapshot().Preferences.LocalProjects) != 1 {
		t.Fatal("removing history lost cloned path")
	}
}
