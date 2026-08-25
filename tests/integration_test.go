// Package tests contains database-backed integration tests. They exercise
// the exact security invariants docs/SPECIFICATION.md and docs/SECURITY.md
// call out as non-negotiable: enrollment tokens are single-use, and a
// session token issued at login must actually validate afterwards (this
// specific round trip previously had a bug where the stored hash was
// computed from different bytes than the lookup hash, silently breaking
// every login — see internal/auth/session.go newOpaqueToken).
//
// These tests require a real PostgreSQL instance and are skipped unless
// WR_TEST_DATABASE_URL is set, e.g.:
//
//	WR_TEST_DATABASE_URL="postgres://user:pass@127.0.0.1:5433/wartungsremote_test?sslmode=disable" go test ./tests/...
package tests

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"wartungsremote/internal/auth"
	"wartungsremote/internal/db"
	"wartungsremote/internal/enrollment"
	"wartungsremote/migrations"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("WR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WR_TEST_DATABASE_URL not set; skipping database integration test")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestEnrollmentTokenIsSingleUse(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	svc := enrollment.New(pool)

	adminID := createTestUser(t, pool, "enrollment-test-admin")
	created, err := svc.Create(ctx, enrollment.CreateParams{DisplayName: "test-device", ExpiresIn: time.Hour, CreatedBy: adminID})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	req := enrollment.AgentEnrollRequest{
		Token:        created.Token,
		InstallID:    uuid.New(),
		PublicKey:    pub,
		AgentVersion: "0.1.0-test",
		OS:           "linux",
		Arch:         "amd64",
		Hostname:     "test-host",
	}

	if _, err := svc.Consume(ctx, req); err != nil {
		t.Fatalf("expected first Consume to succeed, got: %v", err)
	}

	// Second consumption of the SAME token (even with a different install)
	// MUST fail — this is the core anti-replay guarantee from
	// docs/SPECIFICATION.md §8: "Ein Enrollment darf nicht gleichzeitig
	// zweimal erfolgreich abgeschlossen werden."
	req.InstallID = uuid.New()
	if _, err := svc.Consume(ctx, req); err == nil {
		t.Fatal("expected second Consume of the same token to fail (single-use invariant violated)")
	}
}

func createTestUser(t *testing.T, pool *pgxpool.Pool, username string) uuid.UUID {
	t.Helper()
	repo := auth.NewRepo(pool)
	hash, err := auth.HashPassword("integration-test-password-1234", auth.DefaultArgon2Params())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// Usernames are citext-unique; suffix with a fresh UUID so repeated test
	// runs against a non-reset database don't collide.
	id, err := repo.CreateUser(context.Background(), username+"-"+uuid.New().String(), "Integration Test", hash)
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return id
}

func TestEnrollmentTokenRejectsUnknownToken(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	svc := enrollment.New(pool)

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	_, err = svc.Consume(ctx, enrollment.AgentEnrollRequest{
		Token:        "wr_enroll_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
		InstallID:    uuid.New(),
		PublicKey:    pub,
		AgentVersion: "0.1.0-test",
		OS:           "linux",
		Arch:         "amd64",
	})
	if err == nil {
		t.Fatal("expected Consume with an unknown token to fail")
	}
}

func TestSessionCreateThenValidateRoundTrip(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	userID := createTestUser(t, pool, "session-test-user")

	sessions := auth.NewSessionStore(pool, "__Host-wr_session", time.Hour, 30*time.Minute)
	token, sess, err := sessions.Create(ctx, userID, "127.0.0.1", "integration-test")
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	// This is exactly the round trip that was broken: the token handed back
	// to the caller must validate successfully afterwards.
	got, err := sessions.Validate(ctx, token)
	if err != nil {
		t.Fatalf("expected freshly created session token to validate, got: %v", err)
	}
	if got.ID != sess.ID {
		t.Fatalf("expected validated session id %s, got %s", sess.ID, got.ID)
	}

	if err := sessions.Revoke(ctx, sess.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := sessions.Validate(ctx, token); err == nil {
		t.Fatal("expected revoked session token to no longer validate")
	}
}

func TestSessionValidateRejectsUnknownToken(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	sessions := auth.NewSessionStore(pool, "__Host-wr_session", time.Hour, 30*time.Minute)

	if _, err := sessions.Validate(ctx, "not-a-real-token"); err == nil {
		t.Fatal("expected an unknown session token to fail validation")
	}
}
