package application

import (
	"context"
	"errors"
	"repodock/internal/domain"
)

var ErrAccountChanged = errors.New("GitHub 账户已切换，请刷新仓库列表")

type Dependencies struct {
	GitPath string
	GHPath  string
	GitErr  error
	GHErr   error
	Proxy   domain.ProxyConfig
}

type TargetInspector interface {
	Inspect(context.Context, domain.RepositoryRef, string) domain.CloneTarget
}

type ProxyTester interface {
	Test(context.Context, domain.ProxyConfig) error
}

type GitHub interface {
	Account(context.Context) (string, error)
	// ListRepositories calls page only with repositories of the confirmed account.
	// A final error leaves already delivered pages available as an incomplete list.
	ListRepositories(ctx context.Context, account string, page func([]domain.Repository)) error
}

type Publisher interface {
	Inspect(context.Context, domain.PublishRequest, string) ([]string, error)
	Publish(context.Context, domain.PublishState, func(domain.PublishState) error, func(domain.Progress)) (domain.PublishState, error)
}

type Cloner interface {
	Clone(context.Context, domain.RepositoryRef, string, func(domain.Progress)) (string, error)
}

type PreferenceStore interface {
	Load() (domain.Preferences, error)
	Save(domain.Preferences) error
}

type DesktopActions interface {
	PickDirectory(initial string) (string, error)
	OpenCodex(path string) error
	OpenFolder(path string) error
	OpenURL(url string) error
}

type CodexDetector interface {
	DetectCodex() error
}

type ToolResolver interface {
	Resolve(gitPath, ghPath string) Dependencies
}

type Services struct {
	GitHub      GitHub
	Cloner      Cloner
	Publisher   Publisher
	Targets     TargetInspector
	ProxyTester ProxyTester
}

type ServiceFactory func(Dependencies) Services
