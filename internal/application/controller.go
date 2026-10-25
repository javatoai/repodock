package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"slices"
	"strings"
	"sync"
	"time"
)

type Task struct {
	Kind       string
	Status     string // idle, picking, cloning, cancelling, success, error, cancelled
	Repository domain.RepositoryRef
	Path       string
	Progress   domain.Progress
	Logs       []string
	Error      string
}

func (t Task) Busy() bool {
	return t.Status == "picking" || t.Status == "cloning" || t.Status == "publishing" || t.Status == "opening" || t.Status == "cancelling"
}

type Model struct {
	Preferences       domain.Preferences
	Dependencies      Dependencies
	DependenciesReady bool
	CodexChecked      bool
	CodexError        string
	Account           string
	Loading           bool
	Complete          bool
	AccountError      string
	Repositories      []domain.Repository
	Available         map[string]bool
	Task              Task
	Notice            string
	OpenedPath        string
	OpenSequence      uint64
	PreviewRequest    domain.PublishRequest
	PreviewFiles      []string
	PreviewError      string
	PreviewLoading    bool
	CloneTarget       domain.CloneTarget
	ProxyTest         ProxyTest
}

type ProxyTest struct {
	Config  domain.ProxyConfig
	Loading bool
	Error   string
	Message string
}

type Controller struct {
	mu                sync.Mutex
	model             Model
	store             PreferenceStore
	actions           DesktopActions
	resolver          ToolResolver
	factory           ServiceFactory
	onChange          func()
	ctx               context.Context
	cancel            context.CancelFunc
	refreshCancel     context.CancelFunc
	cloneCancel       context.CancelFunc
	generation        uint64
	workers           sync.WaitGroup
	closed            bool
	previewGeneration uint64
	targetGeneration  uint64
	targetCancel      context.CancelFunc
	proxyGeneration   uint64
	proxyCancel       context.CancelFunc
}

func NewController(store PreferenceStore, actions DesktopActions, resolver ToolResolver, factory ServiceFactory, onChange func()) *Controller {
	prefs, err := store.Load()
	prefs.Normalize()
	ctx, cancel := context.WithCancel(context.Background())
	c := &Controller{store: store, actions: actions, resolver: resolver, factory: factory, onChange: onChange, ctx: ctx, cancel: cancel}
	c.model = Model{Preferences: prefs, Task: Task{Status: "idle"}, Available: map[string]bool{}}
	if err != nil {
		c.model.Notice = err.Error()
	}
	return c
}

func (c *Controller) Snapshot() Model {
	c.mu.Lock()
	defer c.mu.Unlock()
	m := c.model
	m.Preferences.Recent = slices.Clone(m.Preferences.Recent)
	m.Preferences.LocalProjects = slices.Clone(m.Preferences.LocalProjects)
	if m.Preferences.Publication != nil {
		state := *m.Preferences.Publication
		m.Preferences.Publication = &state
	}
	m.PreviewFiles = slices.Clone(m.PreviewFiles)
	m.Repositories = slices.Clone(m.Repositories)
	m.Task.Logs = slices.Clone(m.Task.Logs)
	m.Available = make(map[string]bool, len(c.model.Available))
	for path, available := range c.model.Available {
		m.Available[path] = available
	}
	return m
}

func (c *Controller) notify() {
	if c.onChange != nil {
		c.onChange()
	}
}

func (c *Controller) Refresh() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if c.refreshCancel != nil {
		c.refreshCancel()
	}
	ctx, cancel := context.WithCancel(c.ctx)
	c.refreshCancel = cancel
	c.generation++
	id := c.generation
	prefs := c.model.Preferences
	prefs.Recent = slices.Clone(prefs.Recent)
	prefs.LocalProjects = slices.Clone(prefs.LocalProjects)
	c.model.Loading, c.model.Complete = true, false
	c.model.Account, c.model.AccountError = "", ""
	c.model.Repositories = nil
	c.workers.Add(1)
	c.mu.Unlock()
	c.notify()
	go func() {
		defer c.workers.Done()
		defer cancel()
		deps := c.resolver.Resolve(prefs.GitPath, prefs.GHPath)
		deps.Proxy = prefs.Proxy
		available := make(map[string]bool, len(prefs.LocalProjects))
		for _, project := range prefs.LocalProjects {
			available[project.Path] = directoryExists(project.Path)
		}
		if !c.refreshUpdate(id, func(m *Model) {
			m.Dependencies, m.DependenciesReady = deps, true
			// Merge availability only for records still present. A clone that
			// finished during a refresh keeps its newly verified path.
			for _, saved := range prefs.LocalProjects {
				if slices.ContainsFunc(m.Preferences.LocalProjects, func(current domain.RecentProject) bool {
					return domain.SamePath(current.Path, saved.Path) && current.LastOpenedAt.Equal(saved.LastOpenedAt) && current.PreparedAt.Equal(saved.PreparedAt)
				}) {
					m.Available[saved.Path] = available[saved.Path]
				}
			}
		}) {
			return
		}
		if detector, ok := c.actions.(CodexDetector); ok {
			detectionErr := detector.DetectCodex()
			if !c.refreshUpdate(id, func(m *Model) {
				m.CodexChecked, m.CodexError = true, ""
				if detectionErr != nil {
					m.CodexError = detectionErr.Error()
				}
			}) {
				return
			}
		}
		if deps.GHErr != nil {
			c.finishRefresh(id, deps.GHErr)
			return
		}
		service := c.factory(deps).GitHub
		account, err := service.Account(ctx)
		if err != nil {
			c.finishRefresh(id, err)
			return
		}
		if !c.refreshUpdate(id, func(m *Model) { m.Account = account }) {
			return
		}
		err = service.ListRepositories(ctx, account, func(page []domain.Repository) {
			c.refreshUpdate(id, func(m *Model) {
				for _, repo := range page {
					if !slices.ContainsFunc(m.Repositories, func(old domain.Repository) bool { return strings.EqualFold(old.Ref.FullName(), repo.Ref.FullName()) }) {
						m.Repositories = append(m.Repositories, repo)
					}
				}
				slices.SortStableFunc(m.Repositories, func(a, b domain.Repository) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
			})
		})
		c.finishRefresh(id, err)
	}()
}

func (c *Controller) refreshUpdate(id uint64, update func(*Model)) bool {
	c.mu.Lock()
	if c.closed || id != c.generation {
		c.mu.Unlock()
		return false
	}
	update(&c.model)
	c.mu.Unlock()
	c.notify()
	return true
}

func (c *Controller) finishRefresh(id uint64, err error) {
	c.refreshUpdate(id, func(m *Model) {
		m.Loading, m.Complete = false, err == nil
		if err != nil && !errors.Is(err, context.Canceled) {
			m.AccountError = err.Error()
		}
		if errors.Is(err, ErrAccountChanged) {
			m.Account, m.Repositories = "", nil
		}
	})
}

func (c *Controller) BeginClone(raw string, parents ...string) {
	ref, err := domain.ParseRepository(raw)
	c.mu.Lock()
	if c.closed || c.model.Task.Busy() {
		c.mu.Unlock()
		return
	}
	if err == nil && (!c.model.DependenciesReady || c.model.Dependencies.GitErr != nil || c.model.Dependencies.GHErr != nil) {
		err = fmt.Errorf("请先在设置中确认 Git 和 GitHub CLI 可用")
	}
	if err != nil {
		c.model.Task = Task{Status: "error", Error: err.Error()}
		c.mu.Unlock()
		c.notify()
		return
	}
	c.model.Task = Task{Kind: "clone", Status: "picking", Repository: ref}
	ctx, cancel := context.WithCancel(c.ctx)
	c.cloneCancel = cancel
	parent := c.model.Preferences.DefaultCloneDir
	deps := c.model.Dependencies
	deps.Proxy = c.model.Preferences.Proxy
	c.workers.Add(1)
	c.mu.Unlock()
	c.notify()
	go func() {
		defer c.workers.Done()
		defer cancel()
		chosen := parent
		var err error
		if len(parents) > 0 {
			chosen = parents[0]
		} else {
			chosen, err = c.actions.PickDirectory(parent)
		}
		if err != nil {
			c.finishClone("", err)
			return
		}
		if chosen == "" || ctx.Err() != nil {
			c.finishClone("", context.Canceled)
			return
		}
		if !filepath.IsAbs(chosen) {
			c.finishClone("", fmt.Errorf("请选择绝对路径的保存目录"))
			return
		}
		services := c.factory(deps)
		if services.Targets != nil {
			target := services.Targets.Inspect(ctx, ref, chosen)
			if target.State != "ready" {
				c.finishClone("", fmt.Errorf("%s", target.Message))
				return
			}
		}
		if err := os.MkdirAll(chosen, 0755); err != nil {
			c.finishClone("", fmt.Errorf("无法创建保存目录：%w", err))
			return
		}
		c.mu.Lock()
		c.model.Task.Status = "cloning"
		c.mu.Unlock()
		c.notify()
		path, err := services.Cloner.Clone(ctx, ref, chosen, c.reportProgress)
		c.finishClone(path, err)
	}()
}

func (c *Controller) finishClone(path string, err error) {
	c.mu.Lock()
	defer func() { c.mu.Unlock(); c.notify() }()
	c.cloneCancel = nil
	switch {
	case err == nil:
		c.model.Task.Status, c.model.Task.Path = "success", path
		c.model.Preferences.RememberLocal(domain.RecentProject{FullName: c.model.Task.Repository.FullName(), Path: path, PreparedAt: time.Now()})
		c.model.Available[path] = true
		c.saveLocked()
	case errors.Is(err, context.Canceled):
		c.model.Task.Status = "cancelled"
		// Cleanup errors remain visible even when cancellation was requested.
		if err != context.Canceled {
			c.model.Task.Error = err.Error()
		}
	default:
		c.model.Task.Status, c.model.Task.Error = "error", err.Error()
	}
}

func (c *Controller) CancelClone() {
	c.mu.Lock()
	if c.cloneCancel != nil {
		c.model.Task.Status = "cancelling"
		c.cloneCancel()
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Controller) ResetTask() {
	c.mu.Lock()
	if !c.model.Task.Busy() {
		c.model.Task = Task{Status: "idle"}
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Controller) SetTheme(theme string) {
	if theme != "system" && theme != "light" && theme != "dark" {
		return
	}
	c.mu.Lock()
	c.model.Preferences.Theme = theme
	c.saveLocked()
	c.mu.Unlock()
	c.notify()
}

func (c *Controller) ApplyTools(gitPath, ghPath string) {
	c.ApplySettings(gitPath, ghPath, c.Snapshot().Preferences.DefaultCloneDir)
}

func (c *Controller) ApplySettings(gitPath, ghPath, defaultDir string, proxies ...domain.ProxyConfig) {
	c.mu.Lock()
	if c.model.Task.Busy() {
		c.model.Notice = "任务结束后再修改设置"
		c.mu.Unlock()
		c.notify()
		return
	}
	defaultDir = strings.TrimSpace(defaultDir)
	if !filepath.IsAbs(defaultDir) {
		c.model.Notice = "默认项目目录必须是绝对路径"
		c.mu.Unlock()
		c.notify()
		return
	}
	proxy := c.model.Preferences.Proxy
	if len(proxies) > 0 {
		proxy = proxies[0]
	}
	proxy, err := domain.NormalizeProxy(proxy)
	if err != nil {
		c.model.Notice = err.Error()
		c.mu.Unlock()
		c.notify()
		return
	}
	next := c.model.Preferences
	next.GitPath, next.GHPath = strings.TrimSpace(gitPath), strings.TrimSpace(ghPath)
	next.DefaultCloneDir = filepath.Clean(defaultDir)
	next.Proxy = proxy
	if err := c.store.Save(next); err != nil {
		c.model.Notice = "设置未能保存：" + err.Error()
		c.mu.Unlock()
		c.notify()
		return
	}
	c.model.Preferences = next
	c.model.Dependencies.Proxy = proxy
	c.model.Notice = "设置已保存"
	c.mu.Unlock()
	c.Refresh()
}

func (c *Controller) OpenProject(path string, folder bool) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.workers.Add(1)
	c.mu.Unlock()
	go func() {
		defer c.workers.Done()
		var err error
		if !filepath.IsAbs(path) || !directoryExists(path) {
			err = fmt.Errorf("项目目录不存在，请确认是否已移动或删除")
		} else if folder {
			err = c.actions.OpenFolder(path)
		} else {
			err = c.actions.OpenCodex(path)
		}
		c.mu.Lock()
		c.model.Available[path] = directoryExists(path)
		if err != nil {
			c.model.Notice = err.Error()
		} else if !folder {
			for _, project := range c.model.Preferences.LocalProjects {
				if domain.SamePath(project.Path, path) {
					project.LastOpenedAt = time.Now()
					c.model.Preferences.Remember(project)
					c.saveLocked()
					break
				}
			}
			c.model.OpenedPath = path
			c.model.OpenSequence++
			if saved := c.model.Preferences.Publication; saved != nil && saved.Published && domain.SamePath(saved.Path, path) {
				c.model.Preferences.Publication = nil
				c.saveLocked()
			}
			if !c.model.Task.Busy() && domain.SamePath(c.model.Task.Path, path) {
				c.model.Task = Task{Status: "idle"}
			}
		}
		c.mu.Unlock()
		c.notify()
	}()
}

func (c *Controller) RemoveRecent(path string) {
	c.mu.Lock()
	c.model.Preferences.Recent = slices.DeleteFunc(c.model.Preferences.Recent, func(p domain.RecentProject) bool { return domain.SamePath(p.Path, path) })
	c.saveLocked()
	c.mu.Unlock()
	c.notify()
}

func (c *Controller) RemoveLocalProject(path string) {
	c.mu.Lock()
	if c.model.Task.Busy() && domain.SamePath(c.model.Task.Path, path) {
		c.mu.Unlock()
		return
	}
	next := c.model.Preferences
	next.LocalProjects = slices.DeleteFunc(slices.Clone(next.LocalProjects), func(p domain.RecentProject) bool { return domain.SamePath(p.Path, path) })
	next.Recent = slices.DeleteFunc(slices.Clone(next.Recent), func(p domain.RecentProject) bool { return domain.SamePath(p.Path, path) })
	if err := c.store.Save(next); err != nil {
		c.model.Notice = "移除记录未能保存：" + err.Error()
		c.mu.Unlock()
		c.notify()
		return
	}
	c.model.Preferences = next
	for saved := range c.model.Available {
		if domain.SamePath(saved, path) {
			delete(c.model.Available, saved)
		}
	}
	if !c.model.Task.Busy() && domain.SamePath(c.model.Task.Path, path) {
		c.model.Task = Task{Status: "idle"}
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Controller) reportProgress(progress domain.Progress) {
	c.mu.Lock()
	c.model.Task.Progress = progress
	if progress.Line != "" {
		c.model.Task.Logs = append(c.model.Task.Logs, progress.Line)
		if len(c.model.Task.Logs) > 100 {
			c.model.Task.Logs = c.model.Task.Logs[len(c.model.Task.Logs)-100:]
		}
	}
	c.mu.Unlock()
	c.notify()
}

func (c *Controller) DismissNotice() {
	c.mu.Lock()
	c.model.Notice = ""
	c.mu.Unlock()
	c.notify()
}

func (c *Controller) ReportError(err error) {
	if err != nil {
		c.mu.Lock()
		c.model.Notice = err.Error()
		c.mu.Unlock()
		c.notify()
	}
}

func (c *Controller) saveLocked() {
	if err := c.store.Save(c.model.Preferences); err != nil {
		c.model.Notice = "设置未能保存：" + err.Error()
	}
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// Close is called before quitting the native event loop, leaving it alive for
// the last process cancellation and temporary-directory cleanup.
func (c *Controller) Close() {
	c.mu.Lock()
	c.closed = true
	c.cancel()
	c.mu.Unlock()
	c.workers.Wait()
}
