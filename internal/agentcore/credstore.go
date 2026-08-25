package agentcore

// CredentialStore persists the device's private key material at rest,
// protected by an OS-appropriate mechanism, per docs/SECURITY.md §6:
//   - Linux: root-owned file, mode 0600.
//   - Windows: DPAPI, CRYPTPROTECT_LOCAL_MACHINE scope.
// The plaintext private key never leaves the device (it is encrypted for
// at-rest storage only; it is used in-process for signing).
type CredentialStore interface {
	Save(data []byte) error
	Load() ([]byte, error)
	Exists() bool
}
