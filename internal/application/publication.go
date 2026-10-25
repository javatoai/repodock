package application

import (
	"context"
	"errors"
	"fmt"
	"repodock/internal/domain"
	"slices"
	"strings"
	"time"
)

func (c *Controller) PreviewPublication(request domain.PublishRequest) {
	c.mu.Lock()
	if c.closed || c.model.Task.Busy() {
		c.mu.Unlock()
		return
	}
	c.previewGeneration++
	id := c.previewGeneration
	account, deps := c.model.Account, c.model.Dependencies
	deps.Proxy = c.model.Preferences.Proxy
	c.model.PreviewRequest, c.model.PreviewFiles, c.model.PreviewError, c.model.PreviewLoading = request, nil, "", true
	c.workers.Add(1)
	c.mu.Unlock()
	c.notify()
	go func() {
		defer c.workers.Done()
		var files []string
		var err error
		publisher := c.factory(deps).Publisher
		if publisher == nil {
			err = fmt.Errorf("发布服务不可用")
		} else {
			files, err = publisher.Inspect(c.ctx, request, account)
		}
		c.mu.Lock()
		if id != c.previewGeneration || c.closed {
			c.mu.Unlock()
			return
		}
		c.model.PreviewFiles, c.model.PreviewLoading = files, false
		if err != nil {
			c.model.PreviewError = err.Error()
		}
		c.mu.Unlock()
		c.notify()
	}()
}

func (c *Controller) BeginPublish(request domain.PublishRequest) {
	c.mu.Lock()
	if c.closed || c.model.Task.Busy() {
		c.mu.Unlock()
		return
	}
	state := domain.PublishState{Request: request, Owner: c.model.Account}
	if saved := c.model.Preferences.Publication; saved != nil {
		if saved.Request != request {
			c.model.Notice = "请先继续未完成的发布任务"
			c.mu.Unlock()
			c.notify()
			return
		}
		state = *saved
	}
	deps := c.model.Dependencies
	deps.Proxy = c.model.Preferences.Proxy
	if !state.Published && (!c.model.DependenciesReady || deps.GitErr != nil || deps.GHErr != nil || state.Owner == "") {
		c.model.Task = Task{Kind: "publish", Status: "error", Error: "请先在设置中连接 GitHub 并确认 Git、gh 可用"}
		c.mu.Unlock()
		c.notify()
		return
	}
	ref := domain.RepositoryRef{Owner: state.Owner, Name: request.Name}
	c.model.Task = Task{Kind: "publish", Status: "publishing", Repository: ref, Path: state.Path}
	ctx, cancel := context.WithCancel(c.ctx)
	c.cloneCancel = cancel
	c.workers.Add(1)
	c.mu.Unlock()
	c.notify()
	go func() {
		defer c.workers.Done()
		defer cancel()
		var err error
		if !state.Published {
			publisher := c.factory(deps).Publisher
			if publisher == nil {
				err = fmt.Errorf("发布服务不可用")
			} else {
				state, err = publisher.Publish(ctx, state, func(saved domain.PublishState) error {
					c.mu.Lock()
					defer c.mu.Unlock()
					copy := saved
					c.model.Preferences.Publication = &copy
					c.model.Task.Path = saved.Path
					if saved.Published && !slices.ContainsFunc(c.model.Preferences.LocalProjects, func(p domain.RecentProject) bool { return domain.SamePath(p.Path, saved.Path) }) {
						c.model.Preferences.RememberLocal(domain.RecentProject{FullName: ref.FullName(), Path: saved.Path, PreparedAt: time.Now()})
						c.model.Available[saved.Path] = directoryExists(saved.Path)
					}
					return c.store.Save(c.model.Preferences)
				}, c.reportProgress)
			}
		}
		if err == nil {
			c.mu.Lock()
			preparedAt := time.Now()
			for _, project := range c.model.Preferences.LocalProjects {
				if domain.SamePath(project.Path, state.Path) {
					preparedAt = project.PreparedAt
					break
				}
			}
			c.model.Preferences.RememberLocal(domain.RecentProject{FullName: ref.FullName(), Path: state.Path, PreparedAt: preparedAt})
			if strings.EqualFold(c.model.Account, state.Owner) && !slices.ContainsFunc(c.model.Repositories, func(repo domain.Repository) bool { return strings.EqualFold(repo.Ref.FullName(), ref.FullName()) }) {
				c.model.Repositories = append(c.model.Repositories, domain.Repository{Ref: ref, Private: request.Private, UpdatedAt: time.Now()})
				slices.SortStableFunc(c.model.Repositories, func(a, b domain.Repository) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
			}
			c.model.Available[state.Path] = directoryExists(state.Path)
			c.model.Task.Status, c.model.Task.Path = "opening", state.Path
			c.saveLocked()
			c.mu.Unlock()
			c.reportProgress(domain.Progress{Stage: "打开 Codex", Percent: -1})
			if ctx.Err() != nil {
				err = ctx.Err()
			} else if !directoryExists(state.Path) {
				err = fmt.Errorf("本地项目目录已不存在")
			} else {
				err = c.actions.OpenCodex(state.Path)
			}
		}
		c.mu.Lock()
		c.cloneCancel = nil
		c.model.Task.Path = state.Path
		switch {
		case err == nil:
			c.model.Preferences.Remember(domain.RecentProject{FullName: ref.FullName(), Path: state.Path, LastOpenedAt: time.Now()})
			c.model.Preferences.Publication = nil
			c.model.Task.Status = "success"
			c.model.OpenedPath, c.model.OpenSequence = state.Path, c.model.OpenSequence+1
			c.saveLocked()
		case errors.Is(err, context.Canceled):
			c.model.Task.Status, c.model.Task.Error = "cancelled", err.Error()
		default:
			c.model.Task.Status, c.model.Task.Error = "error", err.Error()
		}
		c.mu.Unlock()
		c.notify()
	}()
}

func (c *Controller) EndPublication() {
	c.mu.Lock()
	if !c.model.Task.Busy() {
		c.model.Preferences.Publication = nil
		c.model.Task = Task{Status: "idle"}
		c.model.Notice = "任务已结束，本地目录和已创建的远程仓库均已保留"
		c.saveLocked()
	}
	c.mu.Unlock()
	c.notify()
}
