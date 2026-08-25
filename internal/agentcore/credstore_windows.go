//go:build windows

package agentcore

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dpapiCredentialStore encrypts the credential at rest using the Windows
// Data Protection API in CRYPTPROTECT_LOCAL_MACHINE scope, per
// docs/SECURITY.md §6, so any process running with sufficient privilege on
// this specific machine (i.e. the agent service) can decrypt it, but the
// file is meaningless if copied to another machine.
type dpapiCredentialStore struct {
	path string
}

func NewCredentialStore(dataDir string) CredentialStore {
	return &dpapiCredentialStore{path: filepath.Join(dataDir, "device_credential.dat")}
}

var (
	modcrypt32           = windows.NewLazySystemDLL("crypt32.dll")
	modkernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procCryptProtectData   = modcrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = modcrypt32.NewProc("CryptUnprotectData")
	procLocalFree          = modkernel32.NewProc("LocalFree")
)

const cryptProtectLocalMachine = 0x4

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(b []byte) dataBlob {
	if len(b) == 0 {
		return dataBlob{}
	}
	return dataBlob{cbData: uint32(len(b)), pbData: &b[0]}
}

func protect(plaintext []byte) ([]byte, error) {
	in := newBlob(plaintext)
	var out dataBlob
	descr, err := windows.UTF16PtrFromString("WartungsRemote device credential")
	if err != nil {
		return nil, err
	}
	ret, _, callErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		uintptr(unsafe.Pointer(descr)),
		0, 0, 0,
		uintptr(cryptProtectLocalMachine),
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("agentcore: CryptProtectData failed: %w", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return blobBytes(out), nil
}

func unprotect(ciphertext []byte) ([]byte, error) {
	in := newBlob(ciphertext)
	var out dataBlob
	ret, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("agentcore: CryptUnprotectData failed: %w", callErr)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return blobBytes(out), nil
}

func blobBytes(b dataBlob) []byte {
	if b.cbData == 0 || b.pbData == nil {
		return nil
	}
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

func (s *dpapiCredentialStore) Save(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("agentcore: create credential dir: %w", err)
	}
	encrypted, err := protect(data)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, encrypted, 0o600); err != nil {
		return fmt.Errorf("agentcore: write credential: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("agentcore: finalize credential: %w", err)
	}
	return nil
}

func (s *dpapiCredentialStore) Load() ([]byte, error) {
	encrypted, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("agentcore: read credential: %w", err)
	}
	return unprotect(encrypted)
}

func (s *dpapiCredentialStore) Exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}
