//go:build windows

package windows

import (
	"context"
	"fmt"

	"github.com/UserExistsError/conpty"

	"wartungsremote/internal/platform"
)

type conptySession struct {
	cpty *conpty.ConPty
}

// OpenTerminal starts a ConPTY-backed PowerShell session, per docs/AGENT.md
// §11 (PowerShell default, ConPTY where available).
func (p *Provider) OpenTerminal(ctx context.Context) (platform.TerminalSession, error) {
	cpty, err := conpty.Start("powershell.exe -NoLogo", conpty.ConPtyDimensions(80, 24))
	if err != nil {
		return nil, fmt.Errorf("windows: start conpty: %w", err)
	}
	return &conptySession{cpty: cpty}, nil
}

func (s *conptySession) Read(p []byte) (int, error)  { return s.cpty.Read(p) }
func (s *conptySession) Write(p []byte) (int, error) { return s.cpty.Write(p) }

func (s *conptySession) Resize(cols, rows int) error {
	return s.cpty.Resize(cols, rows)
}

func (s *conptySession) Close() error {
	return s.cpty.Close()
}
