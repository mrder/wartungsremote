//go:build linux

package linux

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"wartungsremote/internal/platform"
)

// SupportAccountUsername is the dedicated local account created for the
// SSH/RDP tunnel feature (docs/AGENT.md "Remote-support account") — never
// the existing root account, so a customer's own root credential is never
// silently changed.
const SupportAccountUsername = "remotewartung"

// EnsureSupportAccount creates the account (idempotent — a no-op if it
// already exists) and always sets a freshly generated password, since this
// is called both on first provisioning and on every explicit rotation.
// Granting sudo is best-effort: some minimal/appliance distros (found live
// on a real ZimaOS install) don't ship sudo or sudoers.d at all — this
// still creates a working, password-authenticated SSH account even then,
// just without elevation from that shell (Terminal already provides
// unconditional root-equivalent access regardless of this).
func (p *Provider) EnsureSupportAccount(ctx context.Context) (username, password string, err error) {
	// Same length as Windows (see platform.GenerateSupportPassword) purely
	// for one consistent format across both platforms — Linux itself has
	// no equivalent length constraint.
	password, err = platform.GenerateSupportPassword(14)
	if err != nil {
		return "", "", fmt.Errorf("linux: generate support password: %w", err)
	}

	if exec.CommandContext(ctx, "id", "-u", SupportAccountUsername).Run() != nil {
		shell := "/bin/sh"
		if _, err := exec.LookPath("bash"); err == nil {
			shell = "/bin/bash"
		}
		cmd := exec.CommandContext(ctx, "useradd", "-m", "-s", shell, SupportAccountUsername)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", "", fmt.Errorf("linux: create support account: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}

	if err := grantSudo(ctx); err != nil {
		slog.Warn("could not grant sudo to remote-support account (sudo may not be installed on this distro); SSH login will still work, just without elevation from that shell", "error", err)
	}

	setPw := exec.CommandContext(ctx, "chpasswd")
	setPw.Stdin = strings.NewReader(fmt.Sprintf("%s:%s\n", SupportAccountUsername, password))
	if out, err := setPw.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("linux: set support account password: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return SupportAccountUsername, password, nil
}

// grantSudo writes a dedicated sudoers.d drop-in (never touches the main
// sudoers file) rather than guessing a distro's admin group name (sudo,
// wheel, and adm are all used by different distros).
func grantSudo(ctx context.Context) error {
	if _, err := exec.LookPath("sudo"); err != nil {
		return fmt.Errorf("sudo not installed: %w", err)
	}
	const dir = "/etc/sudoers.d"
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	line := fmt.Sprintf("%s ALL=(ALL) NOPASSWD:ALL\n", SupportAccountUsername)
	return os.WriteFile(dir+"/wartungsremote-support", []byte(line), 0440)
}
