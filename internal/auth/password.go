package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params controls the cost factors used for password hashing.
// See docs/SECURITY.md §7: must be benchmarkable/configurable.
type Argon2Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	SaltLen     uint32
	KeyLen      uint32
}

func DefaultArgon2Params() Argon2Params {
	return Argon2Params{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1, SaltLen: 16, KeyLen: 32}
}

// HashPassword returns a PHC-like encoded Argon2id hash string containing
// the parameters used, so verification remains correct even after
// parameters change for future users.
func HashPassword(password string, p Argon2Params) (string, error) {
	if len(password) == 0 {
		return "", fmt.Errorf("auth: empty password")
	}
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, p.Iterations, p.MemoryKiB, p.Parallelism, p.KeyLen)

	encoded := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		p.MemoryKiB, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// VerifyPassword checks password against an encoded Argon2id hash produced
// by HashPassword, using constant-time comparison.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("auth: unrecognized password hash format")
	}
	var memoryKiB uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memoryKiB, &iterations, &parallelism); err != nil {
		return false, fmt.Errorf("auth: parse hash params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("auth: decode salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("auth: decode hash: %w", err)
	}

	got := argon2.IDKey([]byte(password), salt, iterations, memoryKiB, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
