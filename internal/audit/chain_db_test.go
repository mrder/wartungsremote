// Database-backed tests for the hash chain. Skipped unless
// WR_TEST_DATABASE_URL is set — see tests/integration_test.go for the
// convention this follows.
package audit

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"wartungsremote/internal/db"
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
	if _, err := pool.Exec(ctx, `TRUNCATE audit_log RESTART IDENTITY`); err != nil {
		t.Fatalf("truncate audit_log: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestVerifyChainTreatsLegacyRowsAsPreChain reproduces exactly the shape
// of an existing production database: rows inserted before this feature
// shipped have NULL prev_hash/entry_hash. VerifyChain must not report
// those as tampering — only entries from the point the chain actually
// started are checked.
func TestVerifyChainTreatsLegacyRowsAsPreChain(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	logger := New(pool)

	// Two legacy rows, exactly as they'd exist from before Record()
	// populated the hash columns.
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, `
			INSERT INTO audit_log (actor_type, event_type, result)
			VALUES ('system', 'legacy.event', 'success')
		`); err != nil {
			t.Fatalf("insert legacy row: %v", err)
		}
	}

	if err := logger.Record(ctx, Event{ActorType: ActorSystem, EventType: "chain.start", Result: ResultSuccess}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := logger.Record(ctx, Event{ActorType: ActorSystem, EventType: "chain.next", Result: ResultSuccess}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	result, err := logger.VerifyChain(ctx)
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected chain to be valid, got broken at id %v", result.BrokenAtID)
	}
	if result.EntriesPreChain != 2 {
		t.Fatalf("expected 2 pre-chain entries, got %d", result.EntriesPreChain)
	}
	if result.EntriesCheck != 2 {
		t.Fatalf("expected 2 checked (chained) entries, got %d", result.EntriesCheck)
	}
}

// TestVerifyChainDetectsTampering confirms the actual point of the
// feature: editing a stored row after the fact is detectable.
func TestVerifyChainDetectsTampering(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	logger := New(pool)

	for i := 0; i < 3; i++ {
		if err := logger.Record(ctx, Event{ActorType: ActorSystem, EventType: "test.event", Result: ResultSuccess}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	if result, err := logger.VerifyChain(ctx); err != nil || !result.Valid {
		t.Fatalf("expected valid chain before tampering, got valid=%v err=%v", result.Valid, err)
	}

	// Bypass the application entirely, as a compromised/careless direct-DB
	// edit would — including working around the append-only trigger
	// (migrations/0003_db_roles.sql), which a superuser connection can
	// disable but an ordinary attacker with only the app role's grants
	// could not. That trigger is the first line of defense; the hash
	// chain is what still catches an edit that gets past it.
	if _, err := pool.Exec(ctx, `ALTER TABLE audit_log DISABLE TRIGGER ALL`); err != nil {
		t.Fatalf("disable trigger: %v", err)
	}
	_, tamperErr := pool.Exec(ctx, `UPDATE audit_log SET result = 'failure' WHERE id = 2`)
	if _, err := pool.Exec(ctx, `ALTER TABLE audit_log ENABLE TRIGGER ALL`); err != nil {
		t.Fatalf("re-enable trigger: %v", err)
	}
	if tamperErr != nil {
		t.Fatalf("tamper: %v", tamperErr)
	}

	result, err := logger.VerifyChain(ctx)
	if err != nil {
		t.Fatalf("VerifyChain: %v", err)
	}
	if result.Valid {
		t.Fatal("expected tampering to be detected, but chain reported valid")
	}
	if result.BrokenAtID == nil || *result.BrokenAtID != 2 {
		t.Fatalf("expected break reported at id 2, got %v", result.BrokenAtID)
	}
}
