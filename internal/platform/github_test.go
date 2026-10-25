package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"repodock/internal/domain"
	"slices"
	"strings"
	"testing"
)

type runnerFunc func(context.Context, Command, func(string)) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, cmd Command, output func(string)) ([]byte, error) {
	return f(ctx, cmd, output)
}

func repositoryPage(count, offset int) []byte {
	rows := []map[string]any{}
	for i := range count {
		rows = append(rows, map[string]any{"name": fmt.Sprintf("repo-%03d", i+offset), "private": (i+offset)%2 == 0, "owner": map[string]string{"login": "you"}, "updated_at": "2026-10-23T10:00:00Z"})
	}
	data, _ := json.Marshal(rows)
	return data
}

func TestGitHubPagesAllOwnedRepositories(t *testing.T) {
	pages := 0
	g := GitHub{GHPath: "gh", Runner: runnerFunc(func(ctx context.Context, cmd Command, emit func(string)) ([]byte, error) {
		if slices.Contains(cmd.Args, "user") {
			return []byte(`{"login":"you"}`), nil
		}
		if !slices.Contains(cmd.Args, "affiliation=owner") || !slices.Contains(cmd.Args, "visibility=all") || !slices.Contains(cmd.Args, "per_page=100") {
			t.Fatal(cmd.Args)
		}
		pages++
		if pages == 1 {
			return repositoryPage(100, 0), nil
		}
		if !slices.Contains(cmd.Args, "page=2") {
			t.Fatal(cmd.Args)
		}
		return repositoryPage(1, 100), nil
	})}
	var repos []domain.Repository
	err := g.ListRepositories(context.Background(), "you", func(page []domain.Repository) { repos = append(repos, page...) })
	if err != nil || len(repos) != 101 || pages != 2 {
		t.Fatal(len(repos), pages, err)
	}
	if !repos[0].Private || repos[1].Private {
		t.Fatal("public/private metadata lost")
	}
}

func TestGitHubFailureKeepsPartialListIncomplete(t *testing.T) {
	calls, delivered := 0, 0
	g := GitHub{Runner: runnerFunc(func(context.Context, Command, func(string)) ([]byte, error) {
		calls++
		if calls == 1 {
			return repositoryPage(100, 0), nil
		}
		return nil, errors.New("HTTP 403")
	})}
	err := g.ListRepositories(context.Background(), "you", func(p []domain.Repository) { delivered += len(p) })
	if err == nil || delivered != 100 || !strings.Contains(err.Error(), "未加载完整") {
		t.Fatal(delivered, err)
	}
}

func TestGitHubDetectsAccountSwitch(t *testing.T) {
	g := GitHub{Runner: runnerFunc(func(ctx context.Context, cmd Command, emit func(string)) ([]byte, error) {
		if slices.Contains(cmd.Args, "user") {
			return []byte(`{"login":"someone-else"}`), nil
		}
		return repositoryPage(1, 0), nil
	})}
	if err := g.ListRepositories(context.Background(), "you", func([]domain.Repository) {}); !errors.Is(err, ErrAccountChanged) {
		t.Fatal(err)
	}
}

func TestGitHubAccountFailures(t *testing.T) {
	for _, detail := range []string{"not logged in", "HTTP 403: resource not accessible", "network unreachable"} {
		t.Run(detail, func(t *testing.T) {
			g := GitHub{Runner: runnerFunc(func(context.Context, Command, func(string)) ([]byte, error) { return nil, errors.New(detail) })}
			if account, err := g.Account(context.Background()); err == nil || account != "" {
				t.Fatal(account, err)
			}
		})
	}
}
