package agentcore

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

type enrollRequest struct {
	Token        string `json:"token"`
	InstallID    string `json:"install_id"`
	PublicKey    string `json:"public_key"`
	AgentVersion string `json:"agent_version"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Hostname     string `json:"hostname"`
}

type enrollResponse struct {
	Data struct {
		DeviceID string `json:"device_id"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Enroll performs the one-time enrollment flow of docs/SPECIFICATION.md §8:
// generate a local keypair, submit the token + public key, and persist the
// resulting device identity. tokenFilePath is deleted on success so the
// one-time token cannot be reused or left readable on disk.
func Enroll(ctx context.Context, serverURL, tokenFilePath string, store CredentialStore, agentVersion, osName, arch, hostname string) (Identity, error) {
	tokenBytes, err := os.ReadFile(tokenFilePath)
	if err != nil {
		return Identity{}, fmt.Errorf("agentcore: read enrollment token file: %w", err)
	}
	token := strings.TrimSpace(string(tokenBytes))

	pub, priv, err := GenerateKeypair()
	if err != nil {
		return Identity{}, err
	}
	installID := NewInstallID()

	reqBody, err := json.Marshal(enrollRequest{
		Token:        token,
		InstallID:    installID.String(),
		PublicKey:    base64.StdEncoding.EncodeToString(pub),
		AgentVersion: agentVersion,
		OS:           osName,
		Arch:         arch,
		Hostname:     hostname,
	})
	if err != nil {
		return Identity{}, fmt.Errorf("agentcore: marshal enroll request: %w", err)
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+"/api/v1/agent/enroll", bytes.NewReader(reqBody))
	if err != nil {
		return Identity{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return Identity{}, fmt.Errorf("agentcore: enroll request: %w", err)
	}
	defer resp.Body.Close()

	var out enrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Identity{}, fmt.Errorf("agentcore: decode enroll response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated || out.Error != nil {
		msg := "unknown error"
		if out.Error != nil {
			msg = out.Error.Message
		}
		return Identity{}, fmt.Errorf("agentcore: enrollment rejected (status %d): %s", resp.StatusCode, msg)
	}

	deviceID, err := uuid.Parse(out.Data.DeviceID)
	if err != nil {
		return Identity{}, fmt.Errorf("agentcore: invalid device_id in response: %w", err)
	}

	identity := Identity{DeviceID: deviceID, InstallID: installID, PublicKey: pub, PrivateKey: priv}
	if err := Persist(store, identity); err != nil {
		return Identity{}, err
	}

	// Best-effort: the token must not remain readable on disk after use.
	_ = os.Remove(tokenFilePath)

	return identity, nil
}
