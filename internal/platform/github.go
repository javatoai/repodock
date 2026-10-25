package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"repodock/internal/application"
	"repodock/internal/domain"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ErrAccountChanged = application.ErrAccountChanged

type GitHub struct {
	Runner  Runner
	GHPath  string
	GitPath string
}

func commandEnvironment(gitPath string) []string {
	env := []string{"GH_HOST=github.com", "GH_PROMPT_DISABLED=1", "GH_PAGER=", "GH_DEBUG=", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never", "GIT_TRACE=0", "GIT_TRACE_CURL=0", "GIT_CURL_VERBOSE=0", "GIT_TRACE_REDACT=1", "LC_ALL=C", "LANG=C"}
	if gitPath != "" {
		env = append(env, "PATH="+filepath.Dir(gitPath)+string(filepath.ListSeparator)+getPath())
	}
	return env
}

func (g *GitHub) command(args ...string) Command {
	return Command{Path: g.GHPath, Args: args, Env: commandEnvironment(g.GitPath)}
}

func (g *GitHub) Account(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := g.Runner.Run(ctx, g.command("api", "--hostname", "github.com", "user"), nil)
	if err != nil {
		return "", fmt.Errorf("无法读取 GitHub 账户，请检查 gh 登录和网络：%w", err)
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(data, &user); err != nil || !ownerPatternForAccount(user.Login) {
		return "", fmt.Errorf("GitHub 账户响应无效")
	}
	return user.Login, nil
}

func ownerPatternForAccount(login string) bool {
	_, err := domain.ParseRepository("https://github.com/" + login + "/check")
	return err == nil
}

func (g *GitHub) ListRepositories(ctx context.Context, account string, page func([]domain.Repository)) error {
	for number := 1; ; number++ {
		pageCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		data, err := g.Runner.Run(pageCtx, g.command("api", "--hostname", "github.com", "--method", "GET", "user/repos",
			"-f", "affiliation=owner", "-f", "visibility=all", "-f", "sort=updated", "-f", "direction=desc",
			"-f", "per_page=100", "-f", "page="+strconv.Itoa(number)), nil)
		cancel()
		if err != nil {
			return fmt.Errorf("仓库尚未加载完整：%w", err)
		}
		var rows []struct {
			Name      string    `json:"name"`
			Private   bool      `json:"private"`
			UpdatedAt time.Time `json:"updated_at"`
			Owner     struct {
				Login string `json:"login"`
			} `json:"owner"`
		}
		if err := json.Unmarshal(data, &rows); err != nil {
			return fmt.Errorf("仓库列表响应无效：%w", err)
		}
		repositories := make([]domain.Repository, 0, len(rows))
		for _, row := range rows {
			if !strings.EqualFold(row.Owner.Login, account) {
				return ErrAccountChanged
			}
			ref, err := domain.ParseRepository("https://github.com/" + row.Owner.Login + "/" + row.Name)
			if err != nil {
				return fmt.Errorf("GitHub 返回了无效仓库名称")
			}
			repositories = append(repositories, domain.Repository{Ref: ref, UpdatedAt: row.UpdatedAt, Private: row.Private})
		}
		slices.SortStableFunc(repositories, func(a, b domain.Repository) int { return b.UpdatedAt.Compare(a.UpdatedAt) })
		page(repositories)
		if len(rows) < 100 {
			break
		}
	}
	current, err := g.Account(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(current, account) {
		return ErrAccountChanged
	}
	return nil
}
