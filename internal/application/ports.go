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
}

type GitHub interface {
	Account(context.Context) (string, error)
	// ListPrivate calls page only with repositories of the confirmed account.
	// A final error leaves already delivered pages available as an incomplete list.
	ListPrivate(ctx context.Context, account string, page func([]domain.Repository)) error
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
	GitHub GitHub
	Cloner Cloner
}

type ServiceFactory func(Dependencies) Services
