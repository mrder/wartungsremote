//go:build windows

package windows

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// SupportAccountUsername is the dedicated local account created for the
// SSH/RDP tunnel feature (docs/AGENT.md "Remote-support account") — a
// local Administrator (RDP always permits admins in, no separate "Remote
// Desktop Users" group membership needed), never the customer's own
// account.
const SupportAccountUsername = "remotewartung"

// EnsureSupportAccount creates the account (idempotent — a no-op if it
// already exists) and always sets a freshly generated password, since this
// is called both on first provisioning and on every explicit rotation.
func (p *Provider) EnsureSupportAccount(ctx context.Context) (username, password string, err error) {
	password, err = generateSupportPassword()
	if err != nil {
		return "", "", fmt.Errorf("windows: generate support password: %w", err)
	}

	exists := exec.CommandContext(ctx, "net", "user", SupportAccountUsername).Run() == nil
	if !exists {
		addCmd := exec.CommandContext(ctx, "net", "user", SupportAccountUsername, password, "/add", "/expires:never")
		if out, err := addCmd.CombinedOutput(); err != nil {
			return "", "", fmt.Errorf("windows: create support account: %w: %s", err, strings.TrimSpace(string(out)))
		}
		groupCmd := exec.CommandContext(ctx, "net", "localgroup", "Administrators", SupportAccountUsername, "/add")
		if out, err := groupCmd.CombinedOutput(); err != nil {
			return "", "", fmt.Errorf("windows: grant admin to support account: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return SupportAccountUsername, password, nil
	}

	setCmd := exec.CommandContext(ctx, "net", "user", SupportAccountUsername, password)
	if out, err := setCmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("windows: set support account password: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return SupportAccountUsername, password, nil
}

func generateSupportPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// base64url already reliably mixes upper/lower/digits; append a fixed
	// symbol so default Windows password-complexity policy (3 of 4
	// character classes) is satisfied even in the unlikely case a given
	// random draw lacks one.
	return base64.RawURLEncoding.EncodeToString(buf) + "!1", nil
}
