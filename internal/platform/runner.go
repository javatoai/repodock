package platform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Command struct {
	Path string
	Args []string
	Env  []string
	Dir  string
}

type Runner interface {
	Run(ctx context.Context, command Command, stderr func(string)) ([]byte, error)
}

type ExecRunner struct{}

// Run drains both pipes and waits for the entire child process tree to stop
// before returning on cancellation, so the caller can safely remove staging.
func (ExecRunner) Run(ctx context.Context, command Command, stderr func(string)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir = command.Dir
	cmd.Env = append(cleanEnvironment(os.Environ()), command.Env...)
	cmd.WaitDelay = 5 * time.Second
	started, cleanup, err := prepareProcess(cmd)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	var stdout bytes.Buffer
	output := &lineWriter{emit: stderr}
	cmd.Stdout = &limitedWriter{writer: &stdout, remaining: 16 << 20}
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("无法启动 %s：%w", command.Path, err)
	}
	if err := started(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	// Cancellation can race the short interval between Start and attaching a
	// suspended Windows child to its job. Re-apply it after attachment.
	if ctx.Err() != nil {
		_ = cmd.Cancel()
	}
	err = cmd.Wait()
	output.flush()
	if ctx.Err() != nil {
		return stdout.Bytes(), ctx.Err()
	}
	if err != nil {
		detail := strings.TrimSpace(output.tail())
		if detail == "" {
			detail = err.Error()
		}
		return stdout.Bytes(), fmt.Errorf("%s", detail)
	}
	return stdout.Bytes(), nil
}

type limitedWriter struct {
	writer    io.Writer
	remaining int
}

// Keep the user's proxy and credential configuration, while removing local
// repository selectors inherited from an IDE or a parent Git process.
func cleanEnvironment(env []string) []string {
	blocked := map[string]bool{
		"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_COMMON_DIR": true,
		"GIT_INDEX_FILE": true, "GIT_OBJECT_DIRECTORY": true,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_SHALLOW_FILE": true,
		"GIT_CONFIG": true, "GIT_CONFIG_PARAMETERS": true, "GIT_CONFIG_COUNT": true,
		"GIT_PREFIX": true, "GIT_INTERNAL_SUPER_PREFIX": true, "GIT_GRAFT_FILE": true,
	}
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		if !blocked[key] && !strings.HasPrefix(key, "GIT_CONFIG_KEY_") && !strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
			result = append(result, entry)
		}
	}
	return result
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, errors.New("命令返回的数据过大")
	}
	w.remaining -= len(p)
	return w.writer.Write(p)
}

var tokenPattern = regexp.MustCompile(`(?i)(?:gh[pousr]_[a-z0-9_]+|github_pat_[a-z0-9_]+|https://[^\s/@]+(?::[^\s/@]*)?@github\.com)`)
var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

func Redact(s string) string {
	s = ansiPattern.ReplaceAllString(s, "")
	return tokenPattern.ReplaceAllString(s, "[已隐藏凭据]")
}

type lineWriter struct {
	mu      sync.Mutex
	pending []byte
	lines   []string
	emit    func(string)
}

func (w *lineWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, b := range data {
		if b == '\r' || b == '\n' {
			w.deliver()
		} else if len(w.pending) < 8192 {
			w.pending = append(w.pending, b)
		}
	}
	return len(data), nil
}

func (w *lineWriter) deliver() {
	if len(w.pending) == 0 {
		return
	}
	line := Redact(string(w.pending))
	w.pending = w.pending[:0]
	w.lines = append(w.lines, line)
	if len(w.lines) > 8 {
		w.lines = w.lines[len(w.lines)-8:]
	}
	if w.emit != nil {
		w.emit(line)
	}
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.deliver()
}

func (w *lineWriter) tail() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.lines, "\n")
}
