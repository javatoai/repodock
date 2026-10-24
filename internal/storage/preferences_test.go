package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"repodock/internal/domain"
	"testing"
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
		data, err := os.ReadFile(s.Path)
		if err != nil || !json.Valid(data) {
			t.Fatalf("partial settings: %q, %v", data, err)
		}
	}
}
