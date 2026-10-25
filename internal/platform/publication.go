package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

type Publisher struct {
	Runner          Runner
	GHPath, GitPath string
}

type remoteRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
	Private  bool   `json:"private"`
}

func (p *Publisher) git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return p.Runner.Run(ctx, Command{Path: p.GitPath, Dir: dir, Args: args, Env: commandEnvironment(p.GitPath)}, nil)
}

func publicationPath(request domain.PublishRequest, owner string) (string, error) {
	ref, err := domain.ParseRepository("https://github.com/" + owner + "/" + request.Name)
	if err != nil || ref.Name != request.Name || (runtime.GOOS == "windows" && !validWindowsName(request.Name)) {
		return "", fmt.Errorf("请输入有效的 GitHub 仓库名")
	}
	path := request.Path
	if !request.Existing {
		path = filepath.Join(request.Parent, request.Name)
	}
	if (!request.Existing && !filepath.IsAbs(request.Parent)) || !filepath.IsAbs(path) {
		return "", fmt.Errorf("请选择绝对路径的项目目录")
	}
	return filepath.Clean(path), nil
}

func canonicalPath(path string) string {
	if actual, err := filepath.EvalSymlinks(path); err == nil {
		return actual
	}
	return filepath.Clean(path)
}

func (p *Publisher) localRepository(ctx context.Context, path string) (bool, error) {
	out, err := p.git(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
			return false, nil
		}
		return false, fmt.Errorf("无法检查本地 Git 仓库：%w", err)
	}
	root := filepath.FromSlash(strings.TrimSpace(string(out)))
	if !domain.SamePath(canonicalPath(root), canonicalPath(path)) {
		return false, fmt.Errorf("该目录属于其他 Git 仓库，请选择仓库根目录")
	}
	return true, nil
}

func (p *Publisher) checkRemotes(ctx context.Context, path, allowed string) error {
	out, err := p.git(ctx, path, "remote")
	if err != nil {
		return err
	}
	names := strings.Fields(string(out))
	if len(names) == 0 {
		return nil
	}
	if allowed != "" && len(names) == 1 && names[0] == "origin" {
		config, err := p.git(ctx, path, "config", "--get-regexp", "^remote[.]origin[.](url|pushurl)$")
		if err != nil {
			return err
		}
		urls, pushes := 0, 0
		for _, line := range strings.Split(strings.TrimSpace(string(config)), "\n") {
			key, value, ok := strings.Cut(line, " ")
			if !ok || strings.TrimSpace(value) != allowed {
				return fmt.Errorf("远程地址已改变，未继续推送")
			}
			if key == "remote.origin.url" {
				urls++
			} else if key == "remote.origin.pushurl" {
				pushes++
			}
		}
		if urls == 1 && pushes <= 1 {
			return nil
		}
	}
	return fmt.Errorf("该目录已配置远程仓库，不能创建或发布")
}

func (p *Publisher) Inspect(ctx context.Context, request domain.PublishRequest, owner string) ([]string, error) {
	path, err := publicationPath(request, owner)
	if err != nil {
		return nil, err
	}
	if !request.Existing {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return nil, fmt.Errorf("同名目录已存在或无法访问")
		}
		return []string{"README.md"}, nil
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("项目目录不存在")
	}
	isRepo, err := p.localRepository(ctx, path)
	if err != nil {
		return nil, err
	}
	var data []byte
	if isRepo {
		if err = p.checkRemotes(ctx, path, ""); err != nil {
			return nil, err
		}
		data, err = p.git(ctx, path, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	} else {
		// 用临时 Git 元数据解释 .gitignore，预览不初始化用户目录。
		temp, tempErr := os.MkdirTemp("", "repodock-inspect-*")
		if tempErr != nil {
			return nil, tempErr
		}
		defer os.RemoveAll(temp)
		if _, err = p.git(ctx, "", "init", "--bare", temp); err != nil {
			return nil, err
		}
		data, err = p.git(ctx, path, "--git-dir="+temp, "--work-tree="+path, "ls-files", "--others", "--exclude-standard", "-z")
	}
	if err != nil {
		return nil, err
	}
	files := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
	if len(files) == 1 && files[0] == "" {
		return nil, nil
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

func (p *Publisher) lookup(ctx context.Context, owner, name string) (remoteRepository, bool, error) {
	out, runErr := p.Runner.Run(ctx, Command{Path: p.GHPath, Args: []string{"api", "--hostname", "github.com", "--include", "repos/" + owner + "/" + name}, Env: commandEnvironment(p.GitPath)}, nil)
	text := strings.ReplaceAll(string(out), "\r\n", "\n")
	status := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "HTTP/") {
			fields := strings.Fields(line)
			if len(fields) > 1 {
				status, _ = strconv.Atoi(fields[1])
			}
		}
	}
	if status == 404 {
		return remoteRepository{}, false, nil
	}
	if runErr != nil {
		return remoteRepository{}, false, fmt.Errorf("无法检查 GitHub 仓库：%s", Redact(runErr.Error()))
	}
	if status != 200 {
		return remoteRepository{}, false, fmt.Errorf("GitHub 仓库检查返回无效状态")
	}
	_, body, found := strings.Cut(text, "\n\n")
	var repo remoteRepository
	if !found || json.Unmarshal([]byte(body), &repo) != nil || repo.ID <= 0 || !strings.EqualFold(repo.FullName, owner+"/"+name) {
		return repo, false, fmt.Errorf("GitHub 仓库响应无效")
	}
	return repo, true, nil
}

func (p *Publisher) confirmAccount(ctx context.Context, owner string) error {
	account, err := (&GitHub{Runner: p.Runner, GHPath: p.GHPath, GitPath: p.GitPath}).Account(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(account, owner) {
		return fmt.Errorf("GitHub 账户已切换，请重新选择项目")
	}
	return nil
}

func (p *Publisher) Publish(ctx context.Context, state domain.PublishState, checkpoint func(domain.PublishState) error, report func(domain.Progress)) (domain.PublishState, error) {
	fail := func(err error) (domain.PublishState, error) { return state, err }
	save := func() error {
		if err := checkpoint(state); err != nil {
			return fmt.Errorf("无法保存发布进度，已停止后续操作：%w", err)
		}
		return ctx.Err()
	}
	if state.Published {
		return state, nil
	}
	path, err := publicationPath(state.Request, state.Owner)
	if err != nil {
		return fail(err)
	}
	if state.Path != "" && !domain.SamePath(state.Path, path) {
		return fail(fmt.Errorf("发布目录与原任务不一致"))
	}
	state.Path = path
	report(domain.Progress{Stage: "检查项目", Percent: -1})
	if ctx.Err() != nil {
		return fail(ctx.Err())
	}
	isRepo := false
	if state.Request.Existing || state.LocalReady {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			return fail(fmt.Errorf("项目目录不存在"))
		}
		isRepo, err = p.localRepository(ctx, path)
		if err != nil {
			return fail(err)
		}
		if isRepo {
			if err = p.checkRemotes(ctx, path, state.RemoteURL); err != nil {
				return fail(err)
			}
			if _, err = p.git(ctx, path, "symbolic-ref", "--short", "HEAD"); err != nil {
				return fail(fmt.Errorf("请先切换到一个本地分支再发布"))
			}
		}
	} else if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		return fail(fmt.Errorf("同名目录已存在或无法访问"))
	}
	identityDir := path
	if !isRepo {
		identityDir = os.TempDir()
	}
	if _, err = p.git(ctx, identityDir, "var", "GIT_AUTHOR_IDENT"); err != nil {
		return fail(fmt.Errorf("请先配置 Git 提交姓名和邮箱：%w", err))
	}
	if _, err = p.git(ctx, identityDir, "var", "GIT_COMMITTER_IDENT"); err != nil {
		return fail(fmt.Errorf("Git 提交身份不可用：%w", err))
	}
	if err = p.confirmAccount(ctx, state.Owner); err != nil {
		return fail(err)
	}
	remote, exists, err := p.lookup(ctx, state.Owner, state.Request.Name)
	if err != nil {
		return fail(err)
	}
	if state.RemoteID == 0 && exists {
		if state.RemoteAttempted {
			return fail(fmt.Errorf("上次创建请求的结果不确定，GitHub 已有同名仓库；请检查远端，未重复创建或推送"))
		}
		return fail(fmt.Errorf("GitHub 已有同名仓库，请更换项目名"))
	}
	if state.RemoteID != 0 && (!exists || remote.ID != state.RemoteID || remote.Private != state.Request.Private) {
		return fail(fmt.Errorf("原任务的 GitHub 仓库已删除、替换或更改可见性，已停止"))
	}
	if !state.LocalReady {
		if !state.Request.Existing {
			if err = os.MkdirAll(state.Request.Parent, 0755); err != nil {
				return fail(err)
			}
			if err = os.Mkdir(path, 0755); err != nil {
				return fail(err)
			}
		}
		state.LocalReady = true
		if err = save(); err != nil {
			return fail(err)
		}
	}
	if !isRepo {
		if _, err = p.git(ctx, path, "init", "-b", "main"); err != nil {
			return fail(err)
		}
	}
	if !state.Request.Existing && !state.Committed {
		readme := filepath.Join(path, "README.md")
		file, createErr := os.OpenFile(readme, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if createErr == nil {
			_, writeErr := file.WriteString("# " + state.Request.Name + "\n")
			closeErr := file.Close()
			if writeErr != nil {
				return fail(writeErr)
			}
			if closeErr != nil {
				return fail(closeErr)
			}
		} else if !os.IsExist(createErr) {
			return fail(createErr)
		}
	}
	if !state.Committed {
		report(domain.Progress{Stage: "提交项目", Percent: -1})
		if err = p.checkRemotes(ctx, path, ""); err != nil {
			return fail(err)
		}
		if _, err = p.git(ctx, path, "var", "GIT_AUTHOR_IDENT"); err != nil {
			return fail(err)
		}
		branch, branchErr := p.git(ctx, path, "symbolic-ref", "--short", "HEAD")
		if branchErr != nil {
			return fail(branchErr)
		}
		state.Branch = strings.TrimSpace(string(branch))
		if _, err = p.git(ctx, path, "check-ref-format", "--branch", state.Branch); err != nil {
			return fail(err)
		}
		if _, err = p.git(ctx, path, "add", "-A", "--", "."); err != nil {
			return fail(err)
		}
		status, statusErr := p.git(ctx, path, "status", "--porcelain")
		if statusErr != nil {
			return fail(statusErr)
		}
		_, headErr := p.git(ctx, path, "rev-parse", "--verify", "HEAD")
		if len(status) != 0 || headErr != nil {
			if _, err = p.git(ctx, path, "commit", "--allow-empty", "-m", "Publish project"); err != nil {
				return fail(err)
			}
		}
		head, headErr := p.git(ctx, path, "rev-parse", "HEAD")
		if headErr != nil {
			return fail(headErr)
		}
		state.Commit, state.Committed = strings.TrimSpace(string(head)), true
		if err = save(); err != nil {
			return fail(err)
		}
	}
	head, err := p.git(ctx, path, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) != state.Commit {
		return fail(fmt.Errorf("本地提交已变化，请先处理未完成的发布任务"))
	}
	branch, err := p.git(ctx, path, "symbolic-ref", "--short", "HEAD")
	if err != nil || strings.TrimSpace(string(branch)) != state.Branch {
		return fail(fmt.Errorf("本地分支已变化，未继续推送"))
	}
	if err = p.confirmAccount(ctx, state.Owner); err != nil {
		return fail(err)
	}
	if state.RemoteID == 0 {
		report(domain.Progress{Stage: "创建 GitHub 仓库", Percent: -1})
		state.RemoteAttempted = true
		if err = save(); err != nil {
			return fail(err)
		}
		data, createErr := p.Runner.Run(ctx, Command{Path: p.GHPath, Args: []string{"api", "--hostname", "github.com", "--method", "POST", "user/repos", "-f", "name=" + state.Request.Name, "-F", "private=" + strconv.FormatBool(state.Request.Private)}, Env: commandEnvironment(p.GitPath)}, nil)
		if createErr != nil {
			return fail(fmt.Errorf("创建仓库失败或结果未知，本地提交已保留：%s", Redact(createErr.Error())))
		}
		var created remoteRepository
		if json.Unmarshal(data, &created) != nil || created.ID <= 0 || !strings.EqualFold(created.FullName, state.Owner+"/"+state.Request.Name) || created.CloneURL != (domain.RepositoryRef{Owner: state.Owner, Name: state.Request.Name}).URL()+".git" || created.Private != state.Request.Private {
			return fail(fmt.Errorf("创建响应无效，请检查 GitHub，未继续推送"))
		}
		state.RemoteID, state.RemoteURL = created.ID, created.CloneURL
		if err = save(); err != nil {
			return fail(err)
		}
	}
	if err = p.checkRemotes(ctx, path, state.RemoteURL); err != nil {
		return fail(err)
	}
	remotes, err := p.git(ctx, path, "remote")
	if err != nil {
		return fail(err)
	}
	if len(strings.Fields(string(remotes))) == 0 {
		if _, err = p.git(ctx, path, "remote", "add", "origin", state.RemoteURL); err != nil {
			return fail(err)
		}
	}
	report(domain.Progress{Stage: "推送到 GitHub", Percent: -1})
	// 只为本次 HTTPS 命令提供 gh 凭据，不改写用户的全局 Git 配置。
	helper := "!" + "'" + strings.ReplaceAll(filepath.ToSlash(p.GHPath), "'", "'\"'\"'") + "' auth git-credential"
	prefix := []string{"-c", "credential.helper=", "-c", "credential.https://github.com.helper=" + helper}
	args := append(slices.Clone(prefix), "push", "--set-upstream", "origin", state.Branch)
	if _, err = p.Runner.Run(ctx, Command{Path: p.GitPath, Dir: path, Args: args, Env: commandEnvironment(p.GitPath)}, func(line string) {
		report(domain.Progress{Stage: "推送到 GitHub", Percent: -1, Line: Redact(line)})
	}); err != nil {
		return fail(fmt.Errorf("推送失败，已保留本地提交和新建仓库，可继续任务：%w", err))
	}
	report(domain.Progress{Stage: "核对远程提交", Percent: -1})
	args = append(slices.Clone(prefix), "ls-remote", "--refs", "origin", "refs/heads/"+state.Branch)
	data, err := p.git(ctx, path, args...)
	if err != nil {
		return fail(err)
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 || fields[0] != state.Commit || fields[1] != "refs/heads/"+state.Branch {
		return fail(fmt.Errorf("远程提交未与本地一致，请重试核对"))
	}
	state.Published = true
	if err = save(); err != nil {
		return fail(err)
	}
	return state, nil
}
