package domain

import (
	"fmt"
	"net/url"
	"regexp"
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
}

type RecentProject struct {
	FullName     string    `json:"fullName"`
	Path         string    `json:"path"`
	LastOpenedAt time.Time `json:"lastOpenedAt"`
}

type Preferences struct {
	Version    int             `json:"version"`
	Theme      string          `json:"theme"`
	GitPath    string          `json:"gitPath,omitempty"`
	GHPath     string          `json:"ghPath,omitempty"`
	LastParent string          `json:"lastParent,omitempty"`
	Recent     []RecentProject `json:"recent"`
}

func DefaultPreferences() Preferences { return Preferences{Version: 1, Theme: "system"} }

func (p *Preferences) Remember(project RecentProject) {
	next := []RecentProject{project}
	for _, old := range p.Recent {
		if old.Path != project.Path && len(next) < 20 {
			next = append(next, old)
		}
	}
	p.Recent = next
}

type Progress struct {
	Stage   string
	Percent int // -1 means the stage does not report a percentage.
	Line    string
}
