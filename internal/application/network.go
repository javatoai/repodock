package application

import (
	"context"
	"fmt"
	"repodock/internal/domain"
)

// Each inspection owns a generation, including a repeat of the same path after
// a focus event. Slow results can never replace the latest selection.
func (c *Controller) CheckCloneTarget(raw, parent string) {
	ref, err := domain.ParseRepository(raw)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if c.targetCancel != nil {
		c.targetCancel()
	}
	c.targetGeneration++
	id := c.targetGeneration
	ctx, cancel := context.WithCancel(c.ctx)
	c.targetCancel = cancel
	c.model.CloneTarget = domain.CloneTarget{Repository: ref, Parent: parent, State: "checking"}
	deps := c.model.Dependencies
	deps.Proxy = c.model.Preferences.Proxy
	c.workers.Add(1)
	c.mu.Unlock()
	c.notify()
	go func() {
		defer c.workers.Done()
		defer cancel()
		target := domain.CloneTarget{Repository: ref, Parent: parent, State: "error"}
		if err != nil {
			target.Message = err.Error()
		} else if inspector := c.factory(deps).Targets; inspector != nil {
			target = inspector.Inspect(ctx, ref, parent)
		} else {
			target.Message = "目录检查服务不可用"
		}
		c.mu.Lock()
		if c.closed || id != c.targetGeneration {
			c.mu.Unlock()
			return
		}
		c.model.CloneTarget = target
		c.targetCancel = nil
		c.mu.Unlock()
		c.notify()
	}()
}

func (c *Controller) RecheckCloneTarget() {
	c.mu.Lock()
	target := c.model.CloneTarget
	c.mu.Unlock()
	if target.Repository.Name != "" {
		c.CheckCloneTarget(target.Repository.URL(), target.Parent)
	}
}

func (c *Controller) TestProxy(config domain.ProxyConfig) {
	// Testing an off draft still tests its explicit endpoint; it does not save
	// the switch or change the network behavior of other commands.
	config, err := domain.NormalizeProxy(config)
	c.mu.Lock()
	if c.closed || c.model.Task.Busy() {
		c.mu.Unlock()
		return
	}
	if c.proxyCancel != nil {
		c.proxyCancel()
	}
	c.proxyGeneration++
	id := c.proxyGeneration
	ctx, cancel := context.WithCancel(c.ctx)
	c.proxyCancel = cancel
	c.model.ProxyTest = ProxyTest{Config: config, Loading: true}
	deps := c.model.Dependencies
	c.workers.Add(1)
	c.mu.Unlock()
	c.notify()
	go func() {
		defer c.workers.Done()
		defer cancel()
		testErr := err
		if testErr == nil {
			if tester := c.factory(deps).ProxyTester; tester != nil {
				testErr = tester.Test(ctx, config)
			} else {
				testErr = fmt.Errorf("代理测试服务不可用")
			}
		}
		c.mu.Lock()
		if c.closed || id != c.proxyGeneration {
			c.mu.Unlock()
			return
		}
		c.model.ProxyTest.Loading = false
		if testErr != nil {
			c.model.ProxyTest.Error = testErr.Error()
		} else {
			c.model.ProxyTest.Message = "GitHub API 连接成功"
		}
		c.proxyCancel = nil
		c.mu.Unlock()
		c.notify()
	}()
}
