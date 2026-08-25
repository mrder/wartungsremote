package db

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate applies all *.sql files from migrationsFS in lexical filename order
// that have not yet been recorded in schema_migrations. Each migration runs
// inside its own transaction. This intentionally never drops or rewrites
// existing schema automatically (no destructive auto-sync in production, per
// docs/AI_IMPLEMENTATION_GUIDE.md §7).
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsFS fs.FS) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename    text PRIMARY KEY,
			applied_at  timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("db: ensure schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, ".")
	if err != nil {
		return fmt.Errorf("db: read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	for _, name := range files {
		var already bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, name).Scan(&already)
		if err != nil {
			return fmt.Errorf("db: check migration %s: %w", name, err)
		}
		if already {
			continue
		}

		content, err := fs.ReadFile(migrationsFS, name)
		if err != nil {
			return fmt.Errorf("db: read migration %s: %w", name, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("db: begin migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(content)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("db: apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(filename) VALUES ($1)`, name); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("db: record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("db: commit migration %s: %w", name, err)
		}
	}
	return nil
}
