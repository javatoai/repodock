//go:build !windows

package platform

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestRunnerCancelsProcessGroup(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan int, 1)
	done := make(chan error, 1)
	go func() {
		_, err := (ExecRunner{}).Run(ctx, Command{Path: shell, Args: []string{"-c", "sleep 30 & echo $! >&2; wait"}}, func(line string) {
			if pid, err := strconv.Atoi(line); err == nil {
				started <- pid
			}
		})
		done <- err
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("child did not start")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation left a child holding the output pipe")
	}
}
