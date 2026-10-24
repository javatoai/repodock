//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsJobCancelsDescendants(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	child := make(chan uint32, 1)
	done := make(chan error, 1)
	go func() {
		_, err := (ExecRunner{}).Run(ctx, Command{Path: os.Args[0], Args: []string{"-test.run=^TestWindowsRunnerHelper$"}, Env: []string{"REPODOCK_PROCESS_HELPER=parent"}}, func(line string) {
			if pid, err := strconv.ParseUint(line, 10, 32); err == nil {
				child <- uint32(pid)
			}
		})
		done <- err
	}()
	var pid uint32
	select {
	case pid = <-child:
		cancel()
	case <-time.After(8 * time.Second):
		t.Fatal("child did not start")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("job cancellation timed out")
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return
	} // The terminated process has already been collected.
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 0)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("child still active: %v %v", status, err)
	}
}

func TestWindowsRunnerHelper(t *testing.T) {
	switch os.Getenv("REPODOCK_PROCESS_HELPER") {
	case "child":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "parent":
		cmd := exec.Command(os.Args[0], "-test.run=^TestWindowsRunnerHelper$")
		cmd.Env = append(os.Environ(), "REPODOCK_PROCESS_HELPER=child")
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, cmd.Process.Pid)
		_ = cmd.Wait()
		os.Exit(0)
	}
}
