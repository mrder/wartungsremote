package config

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"
)

// TestDevelopmentSecretsNeverLogged guards against a future change to
// fillDevelopmentSecrets accidentally logging the generated ephemeral
// session pepper / TOTP key bytes (only the fact that they were generated
// should ever be logged — docs/AGENT.md §16 "Loggt niemals: ... Private
// Keys").
func TestDevelopmentSecretsNeverLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	c := Default()
	c.Mode = "development"
	if err := c.fillDevelopmentSecrets(); err != nil {
		t.Fatal(err)
	}

	logOutput := buf.String()
	for _, secret := range [][]byte{c.Secrets.SessionPepper, c.Secrets.TOTPEncryptionKey} {
		if len(secret) == 0 {
			t.Fatal("expected a generated secret, got empty")
		}
		if strings.Contains(logOutput, hex.EncodeToString(secret)) {
			t.Fatalf("secret leaked into log output (hex): %s", logOutput)
		}
		if strings.Contains(logOutput, base64.StdEncoding.EncodeToString(secret)) {
			t.Fatalf("secret leaked into log output (base64): %s", logOutput)
		}
	}
	if !strings.Contains(logOutput, "ephemeral") {
		t.Fatalf("expected a warning about ephemeral secrets, got %q", logOutput)
	}
}
