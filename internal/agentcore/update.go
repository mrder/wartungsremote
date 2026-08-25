package agentcore

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"wartungsremote/internal/agentupdate"
	"wartungsremote/internal/protocol"
)

// maxArtifactBytes bounds the self-update download so a compromised or
// misconfigured manifest entry can't exhaust agent disk/memory.
const maxArtifactBytes = 256 << 20 // 256MiB

// ReleasePublicKeyHex is the agent's build-embedded trusted release
// signing key (hex-encoded Ed25519 public key), set at build time via:
//
//	go build -ldflags "-X wartungsremote/internal/agentcore.ReleasePublicKeyHex=<hex>"
//
// An empty value means this build has no way to verify a release's
// authenticity, so self-update fails closed rather than trusting whatever
// the server (or a network attacker) claims. This is intentionally
// independent of the server's own signature check on POST
// /agent/releases — a compromised server must not be able to push
// unverifiable binaries to agents.
var ReleasePublicKeyHex string

func releasePublicKey() (ed25519.PublicKey, error) {
	if ReleasePublicKeyHex == "" {
		return nil, fmt.Errorf("no release public key embedded in this build; self-update disabled")
	}
	raw, err := hex.DecodeString(ReleasePublicKeyHex)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("malformed embedded release public key")
	}
	return ed25519.PublicKey(raw), nil
}

// beginUpdate performs docs/AGENT.md §15 steps 4-8 synchronously: download,
// hash+signature verification, staging, backup, and the atomic swap. It
// does not restart the process itself — the caller does that once it has
// had a chance to acknowledge the command over the still-open connection.
// Every failure path here runs before the swap, so a failure never leaves
// the install without a working binary.
func (a *agentSession) beginUpdate(p protocol.AgentUpdateParams) error {
	pubKey, err := releasePublicKey()
	if err != nil {
		return fmt.Errorf("agent update rejected: %w", err)
	}

	slog.Info("agent update starting", "version", p.Version, "url", p.ArtifactURL)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(p.ArtifactURL)
	if err != nil {
		return fmt.Errorf("download artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download artifact: server returned %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArtifactBytes+1))
	if err != nil {
		return fmt.Errorf("read artifact: %w", err)
	}
	if len(data) > maxArtifactBytes {
		return fmt.Errorf("artifact exceeds %d byte limit", maxArtifactBytes)
	}

	if err := agentupdate.VerifyArtifact(pubKey, data, p.ArtifactSHA256Hex, p.SignatureBase64); err != nil {
		return fmt.Errorf("artifact verification failed: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("determine own executable path: %w", err)
	}
	backupPath, err := agentupdate.StageAndSwap(exePath, data)
	if err != nil {
		return fmt.Errorf("stage new binary: %w", err)
	}

	if a.dataDir != "" {
		markerPath := filepath.Join(a.dataDir, "update.marker")
		if err := agentupdate.SaveMarker(markerPath, agentupdate.Marker{Version: p.Version, BackupPath: backupPath}); err != nil {
			slog.Warn("agent update: failed to write rollback marker; update proceeds without a rollback safety net", "error", err)
		}
	}

	slog.Info("agent update staged", "version", p.Version, "backup", backupPath)
	return nil
}
