package platform

import (
	"os/exec"
	"slices"
	"testing"
)

func TestInheritedRepositorySelectorsAreRemoved(t *testing.T) {
	input := []string{"GIT_DIR=/other/repository", "GIT_WORK_TREE=/other/tree", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.worktree", "GIT_CONFIG_VALUE_0=/other/tree", "HTTPS_PROXY=http://localhost:1234", "GH_TOKEN=kept-in-environment", "PATH=/bin"}
	got := cleanEnvironment(input)
	if len(got) != 3 || !slices.Contains(got, "HTTPS_PROXY=http://localhost:1234") || !slices.Contains(got, "GH_TOKEN=kept-in-environment") {
		t.Fatal("expected proxy and authentication environment to be preserved")
	}
}

func TestToolResolutionValidatesExecutable(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git not installed")
	}
	if err := validateTool(git, "git version"); err != nil {
		t.Fatal(err)
	}
	if err := validateTool(git, "gh version"); err == nil {
		t.Fatal("wrong executable accepted")
	}
	if _, err := resolveTool("git", "relative/git"); err == nil {
		t.Fatal("relative executable accepted")
	}
}
