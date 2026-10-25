package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"sync"
	"time"
)

type Preferences struct {
	Path string
	mu   sync.Mutex
	// A corrupt file is preserved before a later explicit save replaces it.
	invalid bool
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "RepoDock", "state.json"), nil
}

func (s *Preferences) Load() (domain.Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := domain.DefaultPreferences()
	f, err := openPreferences(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, fmt.Errorf("读取设置：%w", err)
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	var saved domain.Preferences
	if err := d.Decode(&saved); err != nil {
		s.invalid = true
		return p, fmt.Errorf("设置文件损坏，将在下次保存前保留备份：%w", err)
	}
	if saved.Version < 1 || saved.Version > 3 {
		return p, fmt.Errorf("不支持设置文件版本 %d，请使用匹配版本的 RepoDock", saved.Version)
	}
	if saved.Theme != "system" && saved.Theme != "light" && saved.Theme != "dark" {
		saved.Theme = "system"
	}
	saved.Normalize()
	saved.Version = 3
	return saved, nil
}

func (s *Preferences) Save(p domain.Preferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return fmt.Errorf("创建设置目录：%w", err)
	}
	if s.invalid {
		backup := s.Path + ".corrupt-" + time.Now().Format("20060102-150405.000000000")
		if err := os.Rename(s.Path, backup); err != nil {
			return fmt.Errorf("备份损坏的设置：%w", err)
		}
		s.invalid = false
	}
	// Refuse to downgrade an unknown future schema.
	if raw, err := os.ReadFile(s.Path); err == nil {
		var version struct {
			Version int `json:"version"`
		}
		if json.Unmarshal(raw, &version) == nil && (version.Version < 1 || version.Version > 3) {
			return fmt.Errorf("设置版本不匹配，未覆盖原文件")
		}
	}
	p.Version = 3
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.Path), ".state-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(append(data, '\n')); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return fmt.Errorf("写入设置：%w", err)
	}
	if closeErr != nil {
		return closeErr
	}
	if err := replaceFile(tmp, s.Path); err != nil {
		return fmt.Errorf("保存设置：%w", err)
	}
	return nil
}
