//go:build linux

package agentcore

import (
	"fmt"
	"os"
	"path/filepath"
)

// fileCredentialStore stores the credential in a single file with mode
// 0600. The containing directory is created with 0700. Ownership (root) is
// established by the installer/service user, not by this code, since a
// non-root process cannot chown to root anyway.
type fileCredentialStore struct {
	path string
}

func NewCredentialStore(dataDir string) CredentialStore {
	return &fileCredentialStore{path: filepath.Join(dataDir, "device_credential.bin")}
}

func (s *fileCredentialStore) Save(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("agentcore: create credential dir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("agentcore: write credential: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return fmt.Errorf("agentcore: chmod credential: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("agentcore: finalize credential: %w", err)
	}
	return nil
}

func (s *fileCredentialStore) Load() ([]byte, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("agentcore: read credential: %w", err)
	}
	return data, nil
}

func (s *fileCredentialStore) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}
