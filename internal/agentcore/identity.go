package agentcore

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Identity is the agent's local device identity: an Ed25519 keypair plus the
// device/install IDs assigned during enrollment. The private key never
// leaves the device (docs/SECURITY.md §4/§6).
type Identity struct {
	DeviceID   uuid.UUID
	InstallID  uuid.UUID
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

type storedIdentity struct {
	DeviceID   string `json:"device_id"`
	InstallID  string `json:"install_id"`
	PrivateKey []byte `json:"private_key"`
}

// NewInstallID generates a fresh random ID for a not-yet-enrolled
// installation, per docs/SPECIFICATION.md §7.
func NewInstallID() uuid.UUID {
	return uuid.New()
}

// GenerateKeypair creates a new local Ed25519 keypair; called once before
// enrollment.
func GenerateKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("agentcore: generate keypair: %w", err)
	}
	return pub, priv, nil
}

// Persist stores the completed identity (post-enrollment) via store.
func Persist(store CredentialStore, id Identity) error {
	body, err := json.Marshal(storedIdentity{
		DeviceID:   id.DeviceID.String(),
		InstallID:  id.InstallID.String(),
		PrivateKey: id.PrivateKey,
	})
	if err != nil {
		return fmt.Errorf("agentcore: marshal identity: %w", err)
	}
	if err := store.Save(body); err != nil {
		return err
	}
	return nil
}

// Load reads a previously persisted identity.
func Load(store CredentialStore) (Identity, error) {
	body, err := store.Load()
	if err != nil {
		return Identity{}, err
	}
	var s storedIdentity
	if err := json.Unmarshal(body, &s); err != nil {
		return Identity{}, fmt.Errorf("agentcore: unmarshal identity: %w", err)
	}
	deviceID, err := uuid.Parse(s.DeviceID)
	if err != nil {
		return Identity{}, fmt.Errorf("agentcore: invalid stored device_id: %w", err)
	}
	installID, err := uuid.Parse(s.InstallID)
	if err != nil {
		return Identity{}, fmt.Errorf("agentcore: invalid stored install_id: %w", err)
	}
	priv := ed25519.PrivateKey(s.PrivateKey)
	if len(priv) != ed25519.PrivateKeySize {
		return Identity{}, fmt.Errorf("agentcore: stored private key has unexpected size")
	}
	pub := priv.Public().(ed25519.PublicKey)
	return Identity{DeviceID: deviceID, InstallID: installID, PublicKey: pub, PrivateKey: priv}, nil
}

// Sign signs an arbitrary challenge (the control-channel nonce) with the
// device private key.
func (id Identity) Sign(message []byte) []byte {
	return ed25519.Sign(id.PrivateKey, message)
}
