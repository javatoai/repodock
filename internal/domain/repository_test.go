package domain

import (
	"fmt"
	"testing"
)

func TestRepositoryURLNormalization(t *testing.T) {
	for _, input := range []string{"https://github.com/you/project", " https://github.com/you/project.git ", "https://github.com/you/project.git/", "git@github.com:you/project.git", "ssh://git@github.com/you/project.git"} {
		ref, err := ParseRepository(input)
		if err != nil || ref.FullName() != "you/project" || ref.URL() != "https://github.com/you/project" {
			t.Errorf("%q: %v, %v", input, ref, err)
		}
	}
	for _, input := range []string{"", "you/project", "file:///tmp/repo", "https://gitlab.com/you/project", "https://github.com.evil/you/project", "https://token@github.com/you/project", "https://github.com/you/..", "https://github.com/you/%2e%2e", "https://github.com/you/project/tree/main", "https://github.com/you/project?token=secret", "git@github.com:you/project;echo-bad", "--upload-pack=bad", "ssh://git:password@github.com/you/project", "https://github.com:444/you/project"} {
		if _, err := ParseRepository(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestOpenedProjectsDeduplicateWithoutTruncation(t *testing.T) {
	p := DefaultPreferences()
	for i := range 25 {
		p.Remember(RecentProject{Path: fmt.Sprint(i)})
	}
	p.Remember(RecentProject{Path: "10", FullName: "you/reopened"})
	if len(p.Recent) != 25 || p.Recent[0].FullName != "you/reopened" || len(p.LocalProjects) != 25 {
		t.Fatal(p.Recent)
	}
	count := 0
	for _, project := range p.Recent {
		if project.Path == "10" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate recent entry: %d", count)
	}
}
