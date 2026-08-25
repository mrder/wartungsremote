package agentupdate

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyArtifactAcceptsValidSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("fake release artifact bytes")
	sum := sha256.Sum256(data)
	sig := ed25519.Sign(priv, sum[:])

	err = VerifyArtifact(pub, data, hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sig))
	if err != nil {
		t.Fatalf("expected valid signature to verify, got %v", err)
	}
}

func TestVerifyArtifactRejectsTamperedData(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("original bytes")
	sum := sha256.Sum256(data)
	sig := ed25519.Sign(priv, sum[:])

	tampered := []byte("tampered bytes!")
	err = VerifyArtifact(pub, tampered, hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sig))
	if err == nil {
		t.Fatal("expected tampered artifact to fail verification")
	}
}

func TestVerifyArtifactRejectsWrongKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	data := []byte("some bytes")
	sum := sha256.Sum256(data)
	sig := ed25519.Sign(priv, sum[:])

	err := VerifyArtifact(otherPub, data, hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sig))
	if err == nil {
		t.Fatal("expected signature from an untrusted key to fail verification")
	}
}

func TestVerifyArtifactFailsClosedOnEmptyKey(t *testing.T) {
	data := []byte("data")
	sum := sha256.Sum256(data)
	err := VerifyArtifact(nil, data, hex.EncodeToString(sum[:]), "")
	if err != ErrNoTrustedKey {
		t.Fatalf("expected ErrNoTrustedKey, got %v", err)
	}
}

func TestStageAndSwapAndRestoreBackup(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "wr-agent.exe")
	if err := os.WriteFile(current, []byte("old binary contents"), 0o755); err != nil {
		t.Fatal(err)
	}

	backup, err := StageAndSwap(current, []byte("new binary contents"))
	if err != nil {
		t.Fatalf("StageAndSwap failed: %v", err)
	}
	got, err := os.ReadFile(current)
	if err != nil || string(got) != "new binary contents" {
		t.Fatalf("expected current path to hold new contents, got %q, err %v", got, err)
	}
	backupContents, err := os.ReadFile(backup)
	if err != nil || string(backupContents) != "old binary contents" {
		t.Fatalf("expected backup to hold old contents, got %q, err %v", backupContents, err)
	}

	if err := RestoreBackup(current, backup); err != nil {
		t.Fatalf("RestoreBackup failed: %v", err)
	}
	restored, err := os.ReadFile(current)
	if err != nil || string(restored) != "old binary contents" {
		t.Fatalf("expected current path restored to old contents, got %q, err %v", restored, err)
	}
}

func TestMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "update.marker")

	if _, ok, err := LoadMarker(path); err != nil || ok {
		t.Fatalf("expected no marker initially, got ok=%v err=%v", ok, err)
	}

	m := Marker{Version: "1.2.3", BackupPath: "/tmp/backup", BootAttempts: 1}
	if err := SaveMarker(path, m); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := LoadMarker(path)
	if err != nil || !ok {
		t.Fatalf("expected marker to load, ok=%v err=%v", ok, err)
	}
	if loaded != m {
		t.Fatalf("loaded marker %+v does not match saved %+v", loaded, m)
	}

	if err := DeleteMarker(path); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := LoadMarker(path); err != nil || ok {
		t.Fatalf("expected marker gone after delete, ok=%v err=%v", ok, err)
	}
	// deleting again must be a no-op, not an error
	if err := DeleteMarker(path); err != nil {
		t.Fatalf("expected deleting an already-absent marker to be a no-op, got %v", err)
	}
}
