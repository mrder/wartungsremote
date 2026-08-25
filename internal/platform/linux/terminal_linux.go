//go:build linux

package linux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"

	"wartungsremote/internal/platform"
)

type ptySession struct {
	cmd *exec.Cmd
	f   *os.File
}

// OpenTerminal starts a PTY-backed shell, per docs/AGENT.md §10: explicit
// default shell, no command-string embedding in `sh -c`.
func (p *Provider) OpenTerminal(ctx context.Context) (platform.TerminalSession, error) {
	shell := defaultShell()
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("linux: start pty: %w", err)
	}
	return &ptySession{cmd: cmd, f: f}, nil
}

func defaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		if _, err := os.Stat(shell); err == nil {
			return shell
		}
	}
	for _, candidate := range []string{"/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "/bin/sh"
}

func (s *ptySession) Read(p []byte) (int, error)  { return s.f.Read(p) }
func (s *ptySession) Write(p []byte) (int, error) { return s.f.Write(p) }

func (s *ptySession) Resize(cols, rows int) error {
	return pty.Setsize(s.f, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

// Close terminates the whole process group so children spawned inside the
// shell don't linger after the remote session ends (docs/AGENT.md §10).
func (s *ptySession) Close() error {
	_ = s.f.Close()
	if s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
	}
	_, _ = s.cmd.Process.Wait()
	return nil
}
