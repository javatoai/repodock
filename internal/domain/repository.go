package domain

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

type RepositoryRef struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

func (r RepositoryRef) FullName() string { return r.Owner + "/" + r.Name }
func (r RepositoryRef) URL() string      { return "https://github.com/" + r.FullName() }

var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
var namePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

// ParseRepository accepts repository URLs, never arbitrary Git transports or
// command-line options. SSH input is intentionally normalized to HTTPS so gh
// supplies the same credentials as it uses for the repository picker.
func ParseRepository(raw string) (RepositoryRef, error) {
	invalid := fmt.Errorf("请输入 GitHub 仓库的 HTTPS 或 SSH 地址")
	raw = strings.TrimSpace(raw)
	var path string
	if strings.HasPrefix(strings.ToLower(raw), "git@github.com:") {
		path = raw[len("git@github.com:"):]
	} else {
		u, err := url.Parse(raw)
		if err != nil || u.Opaque != "" || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
			return RepositoryRef{}, invalid
		}
		if !strings.EqualFold(u.Hostname(), "github.com") || (u.Scheme != "https" && u.Scheme != "ssh") {
			return RepositoryRef{}, invalid
		}
		if u.User != nil && (u.Scheme != "ssh" || u.User.String() != "git") {
			return RepositoryRef{}, invalid
		}
		path = strings.TrimPrefix(u.Path, "/")
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || !ownerPattern.MatchString(parts[0]) || !namePattern.MatchString(parts[1]) || parts[1] == "." || parts[1] == ".." {
		return RepositoryRef{}, invalid
	}
	return RepositoryRef{Owner: parts[0], Name: parts[1]}, nil
}

type Repository struct {
	Ref       RepositoryRef
	UpdatedAt time.Time
	Private   bool
}

type RecentProject struct {
	FullName     string    `json:"fullName"`
	Path         string    `json:"path"`
	LastOpenedAt time.Time `json:"lastOpenedAt"`
	PreparedAt   time.Time `json:"preparedAt,omitempty"`
}

type Preferences struct {
	Version         int             `json:"version"`
	Theme           string          `json:"theme"`
	GitPath         string          `json:"gitPath,omitempty"`
	GHPath          string          `json:"ghPath,omitempty"`
	LastParent      string          `json:"lastParent,omitempty"`
	DefaultCloneDir string          `json:"defaultCloneDir,omitempty"`
	LocalProjects   []RecentProject `json:"localProjects,omitempty"`
	Publication     *PublishState   `json:"publication,omitempty"`
	Recent          []RecentProject `json:"recent"`
	Proxy           ProxyConfig     `json:"proxy"`
}

func DefaultPreferences() Preferences {
	return Preferences{Version: 3, Theme: "system", Proxy: DefaultProxy()}
}

func (p *Preferences) Remember(project RecentProject) {
	next := []RecentProject{project}
	for _, old := range p.Recent {
		if !SamePath(old.Path, project.Path) {
			next = append(next, old)
		}
	}
	p.Recent = next
	slices.SortStableFunc(p.Recent, func(a, b RecentProject) int { return b.LastOpenedAt.Compare(a.LastOpenedAt) })
	p.RememberLocal(project)
}

func SamePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (p *Preferences) RememberLocal(project RecentProject) {
	for i, old := range p.LocalProjects {
		if SamePath(old.Path, project.Path) {
			if project.LastOpenedAt.IsZero() {
				project.LastOpenedAt = old.LastOpenedAt
			}
			if project.PreparedAt.IsZero() {
				project.PreparedAt = old.PreparedAt
			}
			p.LocalProjects[i] = project
			p.SortLocalProjects()
			return
		}
	}
	p.LocalProjects = append(p.LocalProjects, project)
	p.SortLocalProjects()
}

func (p *Preferences) SortLocalProjects() {
	slices.SortStableFunc(p.LocalProjects, func(a, b RecentProject) int {
		left, right := a.PreparedAt, b.PreparedAt
		if left.IsZero() {
			left = a.LastOpenedAt
		}
		if right.IsZero() {
			right = b.LastOpenedAt
		}
		return right.Compare(left)
	})
}

// 兼容旧配置：历史记录保留，默认目录不再随单次克隆改变。
func (p *Preferences) Normalize() {
	if p.Proxy.URL == "" {
		p.Proxy.URL = DefaultProxyURL
	}
	if p.DefaultCloneDir == "" {
		p.DefaultCloneDir = p.LastParent
		if p.DefaultCloneDir == "" {
			if home, err := os.UserHomeDir(); err == nil {
				p.DefaultCloneDir = filepath.Join(home, "Projects")
			}
		}
	}
	for _, project := range p.Recent {
		p.RememberLocal(project)
	}
	for i := range p.LocalProjects {
		if p.LocalProjects[i].PreparedAt.IsZero() {
			p.LocalProjects[i].PreparedAt = p.LocalProjects[i].LastOpenedAt
		}
	}
	p.SortLocalProjects()
	slices.SortStableFunc(p.Recent, func(a, b RecentProject) int { return b.LastOpenedAt.Compare(a.LastOpenedAt) })
}

type Progress struct {
	Stage   string
	Percent int // -1 means the stage does not report a percentage.
	Line    string
}
