package tests

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"

	"wartungsremote/internal/device"
	"wartungsremote/internal/enrollment"
)

func TestDeviceDeleteRemovesNeverConnectedDevice(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	adminID := createTestUser(t, pool, "device-delete-admin")
	enroll := enrollment.New(pool)
	created, err := enroll.Create(ctx, enrollment.CreateParams{DisplayName: "never-connected", ExpiresIn: time.Hour, CreatedBy: adminID})
	if err != nil {
		t.Fatalf("Create enrollment: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	enrolled, err := enroll.Consume(ctx, enrollment.AgentEnrollRequest{
		Token: created.Token, InstallID: uuid.New(), PublicKey: pub,
		AgentVersion: "0.1.0-test", OS: "linux", Arch: "amd64", Hostname: "never-connected-host",
	})
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}

	devices := device.NewRepo(pool)
	// This device has never called UpdateConnectivity — last_seen_at is
	// NULL, exactly the "enrollment token consumed but agent never came
	// online" cleanup case Delete exists for.
	if err := devices.Delete(ctx, enrolled.DeviceID); err != nil {
		t.Fatalf("expected Delete to succeed for a never-connected device, got: %v", err)
	}
	if _, err := devices.GetByID(ctx, enrolled.DeviceID); err != device.ErrNotFound {
		t.Fatalf("expected device to be gone after Delete, GetByID returned: %v", err)
	}
}

func TestDeviceDeleteRefusesDeviceWithHistory(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	adminID := createTestUser(t, pool, "device-delete-history-admin")
	enroll := enrollment.New(pool)
	created, err := enroll.Create(ctx, enrollment.CreateParams{DisplayName: "has-connected", ExpiresIn: time.Hour, CreatedBy: adminID})
	if err != nil {
		t.Fatalf("Create enrollment: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	enrolled, err := enroll.Consume(ctx, enrollment.AgentEnrollRequest{
		Token: created.Token, InstallID: uuid.New(), PublicKey: pub,
		AgentVersion: "0.1.0-test", OS: "linux", Arch: "amd64", Hostname: "has-connected-host",
	})
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}

	devices := device.NewRepo(pool)
	// Simulate the device actually connecting at least once — this is
	// the exact condition Delete must refuse to cross, since it's the
	// one thing standing between "safe cleanup" and "silently destroying
	// a real device's audit/metrics trail."
	if err := devices.UpdateConnectivity(ctx, enrolled.DeviceID, device.StatusOnline, "203.0.113.5"); err != nil {
		t.Fatalf("UpdateConnectivity: %v", err)
	}

	if err := devices.Delete(ctx, enrolled.DeviceID); err != device.ErrHasHistory {
		t.Fatalf("expected Delete to refuse a device with connection history (ErrHasHistory), got: %v", err)
	}

	// And it must still actually be there afterward — a refused delete
	// must not have partially applied.
	if _, err := devices.GetByID(ctx, enrolled.DeviceID); err != nil {
		t.Fatalf("expected device to still exist after refused Delete, got: %v", err)
	}
}

func TestDeviceDeleteOfUnknownIDReturnsNotFound(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	devices := device.NewRepo(pool)

	if err := devices.Delete(ctx, uuid.New()); err != device.ErrNotFound {
		t.Fatalf("expected ErrNotFound for a nonexistent device id, got: %v", err)
	}
}
