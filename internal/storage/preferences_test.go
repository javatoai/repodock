package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"testing"
	"time"
)

func TestPreferencesAtomicRoundtrip(t *testing.T) {
	s := &Preferences{Path: filepath.Join(t.TempDir(), "nested", "state.json")}
	p, err := s.Load()
	if err != nil || p.Theme != "system" {
		t.Fatal(p, err)
	}
	p.Theme, p.LastParent = "dark", filepath.Join(t.TempDir(), "中文 空格")
	p.Remember(domain.RecentProject{FullName: "you/project", Path: filepath.Join(p.LastParent, "project")})
	for range 2 {
		if err := s.Save(p); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := s.Load()
	if err != nil || loaded.LastParent != p.LastParent || len(loaded.Recent) != 1 || loaded.Theme != "dark" {
		t.Fatal(loaded, err)
	}
	files, _ := os.ReadDir(filepath.Dir(s.Path))
	if len(files) != 1 {
		t.Fatalf("temporary file leaked: %v", files)
	}
}

func TestVersionOneAndTwoMigrateToThreeAndPreservePublication(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			root := t.TempDir()
			p := domain.Preferences{Version: version, Theme: "dark", LastParent: root, Publication: &domain.PublishState{Path: filepath.Join(root, "pending"), Committed: true, Commit: "abc123"}, Recent: []domain.RecentProject{{FullName: "you/old", Path: filepath.Join(root, "old"), LastOpenedAt: time.Unix(10, 0)}}, LocalProjects: []domain.RecentProject{{FullName: "you/cloned", Path: filepath.Join(root, "cloned")}}}
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "state.json")
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			store := &Preferences{Path: path}
			got, err := store.Load()
			if err != nil || got.Version != 3 || got.Proxy.Enabled || got.Proxy.URL != domain.DefaultProxyURL || got.Publication == nil || got.Publication.Commit != "abc123" || len(got.LocalProjects) != 2 || got.DefaultCloneDir != root {
				t.Fatal(got, err)
			}
			got.Proxy = domain.ProxyConfig{Enabled: true, URL: "https://proxy.example:8443"}
			if err = store.Save(got); err != nil {
				t.Fatal(err)
			}
			restarted, err := (&Preferences{Path: path}).Load()
			if err != nil || restarted.Proxy != got.Proxy || restarted.Publication.Commit != "abc123" {
				t.Fatal(restarted, err)
			}
		})
	}
}

func TestCorruptPreferencesArePreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Preferences{Path: path}
	if _, err := s.Load(); err == nil {
		t.Fatal("corruption ignored")
	}
	if err := s.Save(domain.DefaultPreferences()); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(path + ".corrupt-*")
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	data, _ := os.ReadFile(backups[0])
	if string(data) != "broken" {
		t.Fatal(string(data))
	}
}

func TestFuturePreferencesAreNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	original := []byte(`{"version":9,"future":true}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	s := &Preferences{Path: path}
	if _, err := s.Load(); err == nil {
		t.Fatal("accepted future schema")
	}
	if err := s.Save(domain.DefaultPreferences()); err == nil {
		t.Fatal("overwrote future schema")
	}
	data, _ := os.ReadFile(path)
	if string(data) != string(original) {
		t.Fatal(string(data))
	}
}

func TestAtomicSaveHasNoPartialJSON(t *testing.T) {
	s := &Preferences{Path: filepath.Join(t.TempDir(), "state.json")}
	p := domain.DefaultPreferences()
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	reader := &Preferences{Path: s.Path}
	go func() {
		for range 50 {
			if err := s.Save(p); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			return
		default:
		}
		loaded, err := reader.Load()
		if err != nil || loaded.Version != 3 || loaded.Theme != "system" {
			t.Fatalf("partial settings: %+v, %v", loaded, err)
		}
	}
}

func TestLegacyHistoryMigratesWithoutTruncating(t *testing.T) {
	p := domain.DefaultPreferences()
	p.Version = 1
	p.LastParent = t.TempDir()
	for i := range 35 {
		p.Recent = append(p.Recent, domain.RecentProject{FullName: "you/project", Path: filepath.Join(p.LastParent, fmt.Sprint(i))})
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := (&Preferences{Path: path}).Load()
	if err != nil || loaded.DefaultCloneDir != p.LastParent || len(loaded.Recent) != 35 || len(loaded.LocalProjects) != 35 {
		t.Fatal(loaded, err)
	}
}
