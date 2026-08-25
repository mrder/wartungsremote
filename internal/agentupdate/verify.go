// Package agentupdate implements the verify/stage/swap/rollback mechanics
// for signed agent self-updates (docs/AGENT.md §15). Signing itself is
// deliberately NOT part of this package or of any online service — release
// artifacts are signed offline with cmd/wr-release-sign, and neither
// wr-core nor wr-agent ever holds a production private signing key.
package agentupdate

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrHashMismatch      = errors.New("agentupdate: artifact hash does not match expected sha256")
	ErrSignatureInvalid  = errors.New("agentupdate: signature verification failed")
	ErrNoTrustedKey      = errors.New("agentupdate: no trusted release public key configured")
)

// VerifyArtifact checks that data hashes to expectedSHA256Hex and that
// sigB64 is a valid Ed25519 signature (over the hash, not the raw bytes —
// matching cmd/wr-release-sign's signing scheme) made by the given trusted
// public key. Fails closed: an empty/short pubKey is always rejected rather
// than silently skipping verification.
func VerifyArtifact(pubKey ed25519.PublicKey, data []byte, expectedSHA256Hex, sigB64 string) error {
	if len(pubKey) != ed25519.PublicKeySize {
		return ErrNoTrustedKey
	}
	sum := sha256.Sum256(data)
	expected, err := hex.DecodeString(expectedSHA256Hex)
	if err != nil || len(expected) != len(sum) {
		return fmt.Errorf("%w: malformed expected hash", ErrHashMismatch)
	}
	if !bytes.Equal(sum[:], expected) {
		return ErrHashMismatch
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("%w: malformed signature encoding", ErrSignatureInvalid)
	}
	if !ed25519.Verify(pubKey, sum[:], sig) {
		return ErrSignatureInvalid
	}
	return nil
}

// VerifyHashAndSignature is like VerifyArtifact but takes an already
// hex-decoded hash and base64-decoded signature — used server-side, where
// the artifact bytes aren't available (only their claimed hash/signature),
// to check a release submission was signed by the trusted key before
// accepting it into the manifest.
func VerifyHashAndSignature(pubKey ed25519.PublicKey, sha256Sum []byte, sig []byte) error {
	if len(pubKey) != ed25519.PublicKeySize {
		return ErrNoTrustedKey
	}
	if len(sha256Sum) != sha256.Size {
		return fmt.Errorf("%w: expected hash must be %d bytes", ErrHashMismatch, sha256.Size)
	}
	if !ed25519.Verify(pubKey, sha256Sum, sig) {
		return ErrSignatureInvalid
	}
	return nil
}
