// Command wr-core is the WartungsRemote core server: authentication, device
// registry, control channel and admin/agent HTTP API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kardianos/service"

	"wartungsremote/internal/audit"
	"wartungsremote/internal/config"
	"wartungsremote/internal/db"
	"wartungsremote/internal/httpapi"
	"wartungsremote/migrations"
)

const version = "0.1.0-dev"

type program struct {
	cancel context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go func() {
		if err := run(ctx); err != nil {
			slog.Error("wr-core exited with error", "error", err)
		}
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "createadmin" {
		if err := runCreateAdmin(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "wr-core createadmin:", err)
			os.Exit(1)
		}
		return
	}

	action := flag.String("service", "", "service control action: install, uninstall, start, stop, restart (default: run in foreground)")
	configPath := flag.String("config", os.Getenv("WR_CONFIG_FILE"), "path to server.yaml")
	dbURLFile := flag.String("database-url-file", os.Getenv("WR_DATABASE_URL_FILE"), "path to a file containing the database DSN")
	migrationDBURLFile := flag.String("migration-database-url-file", os.Getenv("WR_MIGRATION_DATABASE_URL_FILE"), "path to a file containing a DDL-capable database DSN for migrations only (optional; defaults to --database-url-file)")
	sessionPepperFile := flag.String("session-pepper-file", os.Getenv("WR_SESSION_PEPPER_FILE"), "path to the session pepper secret file")
	totpKeyFile := flag.String("totp-key-file", os.Getenv("WR_TOTP_ENCRYPTION_KEY_FILE"), "path to the TOTP encryption key secret file (32 bytes)")
	internalKeyFile := flag.String("internal-service-key-file", os.Getenv("WR_INTERNAL_SERVICE_KEY_FILE"), "path to the internal service key secret file")
	flag.Parse()

	// These flags exist so a Windows Service (which cannot reliably observe
	// machine environment variable changes made after the SCM started
	// without a reboot) can be given the same configuration via its fixed
	// service command line instead. Re-exporting them as env vars here keeps
	// internal/config's single env-var-based secret loading path authoritative.
	setEnvIfSet("WR_CONFIG_FILE", *configPath)
	setEnvIfSet("WR_DATABASE_URL_FILE", *dbURLFile)
	setEnvIfSet("WR_MIGRATION_DATABASE_URL_FILE", *migrationDBURLFile)
	setEnvIfSet("WR_SESSION_PEPPER_FILE", *sessionPepperFile)
	setEnvIfSet("WR_TOTP_ENCRYPTION_KEY_FILE", *totpKeyFile)
	setEnvIfSet("WR_INTERNAL_SERVICE_KEY_FILE", *internalKeyFile)

	var svcArgs []string
	if *configPath != "" {
		svcArgs = append(svcArgs, "--config", *configPath)
	}
	if *dbURLFile != "" {
		svcArgs = append(svcArgs, "--database-url-file", *dbURLFile)
	}
	if *migrationDBURLFile != "" {
		svcArgs = append(svcArgs, "--migration-database-url-file", *migrationDBURLFile)
	}
	if *sessionPepperFile != "" {
		svcArgs = append(svcArgs, "--session-pepper-file", *sessionPepperFile)
	}
	if *totpKeyFile != "" {
		svcArgs = append(svcArgs, "--totp-key-file", *totpKeyFile)
	}
	if *internalKeyFile != "" {
		svcArgs = append(svcArgs, "--internal-service-key-file", *internalKeyFile)
	}

	svcConfig := &service.Config{
		Name:        "wartungsremote-core",
		DisplayName: "WartungsRemote Core Server",
		Description: "WartungsRemote core server: auth, device registry, control channel, admin/agent API. See docs/ARCHITECTURE.md.",
		Arguments:   svcArgs,
	}
	prg := &program{}
	svc, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wr-core: create service:", err)
		os.Exit(1)
	}

	switch *action {
	case "install", "uninstall", "start", "stop", "restart":
		if err := service.Control(svc, *action); err != nil {
			fmt.Fprintln(os.Stderr, "wr-core:", *action, "failed:", err)
			os.Exit(1)
		}
		fmt.Printf("wr-core: %s completed\n", *action)
		return
	case "run", "":
		if service.Interactive() {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := run(ctx); err != nil {
				slog.Error("wr-core exited with error", "error", err)
				os.Exit(1)
			}
			return
		}
		if err := svc.Run(); err != nil {
			slog.Error("wr-core service run failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "wr-core: unknown --service action %q\n", *action)
		os.Exit(1)
	}
}

func setEnvIfSet(key, value string) {
	if value != "" {
		_ = os.Setenv(key, value)
	}
}

func run(ctx context.Context) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	configPath := os.Getenv("WR_CONFIG_FILE")
	if configPath == "" {
		configPath = "server.yaml"
	}
	cfg, err := config.LoadServer(configPath)
	if err != nil {
		return err
	}
	slog.Info("configuration loaded", "mode", cfg.Mode, "config_file", configPath)

	dsn := cfg.Secrets.DatabaseURL
	if dsn == "" {
		dsn = os.Getenv("WR_DATABASE_URL_DEV")
	}
	if dsn == "" {
		return errors.New("no database DSN configured (set WR_DATABASE_URL_FILE or, for local dev only, WR_DATABASE_URL_DEV)")
	}

	// Migrations need DDL rights; the runtime DSN above is allowed to be a
	// least-privilege role that doesn't have them (docs/DEPLOYMENT.md
	// §5a) — in which case a separate migration DSN is required. Falls
	// back to the same DSN when unset, which is the only DSN most
	// installs (anything not using the restricted role) ever configure.
	migrationDSN := cfg.Secrets.MigrationDatabaseURL
	if migrationDSN == "" {
		migrationDSN = dsn
	}
	migrationPool, err := db.Connect(ctx, migrationDSN)
	if err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	migrateErr := db.Migrate(ctx, migrationPool, migrations.FS)
	migrationPool.Close()
	if migrateErr != nil {
		return migrateErr
	}
	slog.Info("database migrations applied")

	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	slog.Info("database connected")

	auditLogger := audit.New(pool)

	deps := httpapi.Dependencies{
		Pool:    pool,
		Config:  cfg,
		Audit:   auditLogger,
		Version: version,
	}
	router, err := httpapi.NewRouter(deps)
	if err != nil {
		return err
	}

	publicSrv := &http.Server{
		Addr:              cfg.Public.Listen,
		Handler:           router.Public,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	adminSrv := &http.Server{
		Addr:              cfg.Admin.Listen,
		Handler:           router.Admin,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		slog.Info("public gateway listening", "addr", cfg.Public.Listen)
		if err := publicSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	go func() {
		slog.Info("admin web listening", "addr", cfg.Admin.Listen)
		if err := adminSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		slog.Error("server error", "error", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var shutdownErr error
	if err := publicSrv.Shutdown(shutdownCtx); err != nil {
		shutdownErr = errors.Join(shutdownErr, err)
	}
	if err := adminSrv.Shutdown(shutdownCtx); err != nil {
		shutdownErr = errors.Join(shutdownErr, err)
	}
	router.Close()
	slog.Info("graceful shutdown complete")
	return shutdownErr
}
