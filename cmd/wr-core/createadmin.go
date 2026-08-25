package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"wartungsremote/internal/auth"
	"wartungsremote/internal/config"
	"wartungsremote/internal/db"
)

// runCreateAdmin implements `wr-core createadmin`, the explicit, non-interactive
// bootstrap path for the first super_admin required by docs/SPECIFICATION.md
// §12 ("erster Superadmin wird explizit per Setup/CLI erstellt"). The
// password is read from a file (never a CLI argument or interactive prompt
// echoed to a terminal) to avoid leaking it via shell history or process
// listings.
func runCreateAdmin(args []string) error {
	fs := flag.NewFlagSet("createadmin", flag.ExitOnError)
	username := fs.String("username", "", "admin username")
	displayName := fs.String("display-name", "", "display name")
	passwordFile := fs.String("password-file", "", "path to a file containing the initial password (single line)")
	configPath := fs.String("config", os.Getenv("WR_CONFIG_FILE"), "path to server.yaml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" || *passwordFile == "" {
		return fmt.Errorf("createadmin: --username and --password-file are required")
	}

	passwordBytes, err := os.ReadFile(*passwordFile)
	if err != nil {
		return fmt.Errorf("createadmin: read password file: %w", err)
	}
	password := strings.TrimRight(string(passwordBytes), "\r\n")
	if len(password) < 12 {
		return fmt.Errorf("createadmin: password must be at least 12 characters (docs/SECURITY.md §7)")
	}

	path := *configPath
	if path == "" {
		path = "server.yaml"
	}
	cfg, err := config.LoadServer(path)
	if err != nil {
		return err
	}

	dsn := cfg.Secrets.DatabaseURL
	if dsn == "" {
		dsn = os.Getenv("WR_DATABASE_URL_DEV")
	}
	if dsn == "" {
		return fmt.Errorf("createadmin: no database DSN configured")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	argon2Params := auth.Argon2Params{
		MemoryKiB:   cfg.Security.Argon2MemoryKiB,
		Iterations:  cfg.Security.Argon2Iterations,
		Parallelism: cfg.Security.Argon2Parallelism,
		SaltLen:     16,
		KeyLen:      32,
	}
	hash, err := auth.HashPassword(password, argon2Params)
	if err != nil {
		return err
	}

	repo := auth.NewRepo(pool)
	userID, err := repo.CreateUser(ctx, *username, *displayName, hash)
	if err != nil {
		return err
	}
	roleID, err := repo.RoleIDByName(ctx, "super_admin")
	if err != nil {
		return fmt.Errorf("createadmin: super_admin role missing; run migrations first: %w", err)
	}
	if err := repo.AssignRole(ctx, userID, roleID, auth.ScopeGlobal, nil); err != nil {
		return err
	}

	fmt.Printf("Created super_admin %q (id=%s).\n", *username, userID)
	fmt.Println("TOTP setup is required on first login (the login response will include a setup_uri).")
	return nil
}
