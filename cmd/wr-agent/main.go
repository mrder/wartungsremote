// Command wr-agent is the WartungsRemote agent: a visible, documented
// system service that reports inventory/metrics and provides remote
// maintenance capabilities to an authorized wr-core server. See
// docs/AGENT.md and docs/PROJECT_CONCEPT.md §36 (no hidden processes).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"

	"wartungsremote/internal/agentcore"
	"wartungsremote/internal/agentupdate"
	"wartungsremote/internal/config"
)

const agentVersion = "0.1.0-dev"

type program struct {
	cancel context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.run(ctx)
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func (p *program) run(ctx context.Context) {
	if err := runAgent(ctx); err != nil {
		slog.Error("wr-agent exited with error", "error", err)
	}
}

func main() {
	configPath := flag.String("config", "", "path to agent.yaml (default: OS-specific documented location)")
	action := flag.String("service", "", "service control action: install, uninstall, start, stop, run (default: run in foreground)")
	flag.Parse()

	paths := agentcore.DefaultPaths()
	if *configPath == "" {
		*configPath = paths.ConfigFile
	}

	svcConfig := &service.Config{
		Name:        "wartungsremote-agent",
		DisplayName: "WartungsRemote Agent",
		Description: "WartungsRemote remote maintenance agent. See docs/AGENT.md. Visibly installed, no hidden functions.",
		Arguments:   []string{"--config", *configPath},
	}

	prg := &program{}
	svc, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wr-agent: create service:", err)
		os.Exit(1)
	}

	switch *action {
	case "install", "uninstall", "start", "stop", "restart":
		if err := service.Control(svc, *action); err != nil {
			fmt.Fprintln(os.Stderr, "wr-agent:", *action, "failed:", err)
			os.Exit(1)
		}
		fmt.Printf("wr-agent: %s completed\n", *action)
		return
	case "run", "":
		// Running under a service manager (systemd/SCM) calls svc.Run(),
		// which invokes Start/Stop; running interactively falls through to
		// the same runAgent path without the service manager wrapper.
		if service.Interactive() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := runAgent(ctx); err != nil {
				slog.Error("wr-agent exited with error", "error", err)
				os.Exit(1)
			}
			return
		}
		if err := svc.Run(); err != nil {
			slog.Error("wr-agent service run failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "wr-agent: unknown --service action %q\n", *action)
		os.Exit(1)
	}
}

func runAgent(ctx context.Context) error {
	configPath := agentcore.DefaultPaths().ConfigFile
	for i, a := range os.Args {
		if a == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
		}
	}

	cfg, err := config.LoadAgent(configPath)
	if err != nil {
		return err
	}
	paths := agentcore.DefaultPaths()
	cfg.DataDir = paths.DataDir
	cfg.LogDir = paths.LogDir

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}))
	slog.SetDefault(logger)
	slog.Info("wr-agent starting", "version", agentVersion, "server_url", cfg.ServerURL, "config", configPath)

	store := agentcore.NewCredentialStore(cfg.DataDir)
	if !store.Exists() {
		tokenFile := filepath.Join(cfg.DataDir, "enroll.token")
		if _, err := os.Stat(tokenFile); err != nil {
			return fmt.Errorf("wr-agent: not enrolled and no enrollment token found at %s; see docs/AGENT.md §5", tokenFile)
		}
		hostname, _ := os.Hostname()
		slog.Info("enrolling", "token_file", tokenFile)
		identity, err := agentcore.Enroll(ctx, cfg.ServerURL, tokenFile, store, agentVersion, runtime.GOOS, runtime.GOARCH, hostname)
		if err != nil {
			return fmt.Errorf("wr-agent: enrollment failed: %w", err)
		}
		slog.Info("enrollment successful", "device_id", identity.DeviceID)
	}

	identity, err := agentcore.Load(store)
	if err != nil {
		return fmt.Errorf("wr-agent: load device identity: %w", err)
	}

	if err := reconcilePendingUpdate(cfg.DataDir); err != nil {
		// A rollback attempt failing is logged, not fatal — worst case the
		// agent keeps running whatever binary is currently in place.
		slog.Error("pending update reconciliation failed", "error", err)
	}
	onFirstConnect := commitPendingUpdate(cfg.DataDir)

	provider := newProvider(agentVersion)
	agentcore.Run(ctx, cfg.ServerURL, agentVersion, identity, provider, cfg.Policy, cfg.DataDir, onFirstConnect)
	return nil
}

// reconcilePendingUpdate implements docs/AGENT.md §15 step 11 ("Rollback
// bei Fehler"): if an update marker survived from a previous run, this
// process starting at all means the previous attempt didn't commit
// (commitPendingUpdate never got to delete it). After
// agentupdate.MaxBootAttempts consecutive uncommitted starts — a crash
// loop, not a one-off restart — restore the pre-update binary and exit so
// the service manager relaunches the known-good version.
func reconcilePendingUpdate(dataDir string) error {
	if dataDir == "" {
		return nil
	}
	markerPath := filepath.Join(dataDir, "update.marker")
	m, ok, err := agentupdate.LoadMarker(markerPath)
	if err != nil || !ok {
		return err
	}
	m.BootAttempts++
	if m.BootAttempts > agentupdate.MaxBootAttempts {
		slog.Error("agent update rollback: repeated failed boots since last update, restoring previous binary", "attempts", m.BootAttempts, "version", m.Version)
		exePath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("determine own executable path: %w", err)
		}
		if err := agentupdate.RestoreBackup(exePath, m.BackupPath); err != nil {
			return fmt.Errorf("restore backup: %w", err)
		}
		_ = agentupdate.DeleteMarker(markerPath)
		os.Exit(1) // let the service manager relaunch with the restored binary
	}
	return agentupdate.SaveMarker(markerPath, m)
}

// commitPendingUpdate returns a callback for agentcore.Run's onFirstConnect
// hook: once this (possibly just-updated) binary proves it can actually
// reach the server, the update is considered successful — delete the
// marker and the backup binary (docs/AGENT.md §15 step 10 "Health Signal").
func commitPendingUpdate(dataDir string) func() {
	if dataDir == "" {
		return nil
	}
	markerPath := filepath.Join(dataDir, "update.marker")
	return func() {
		m, ok, err := agentupdate.LoadMarker(markerPath)
		if err != nil || !ok {
			return
		}
		slog.Info("agent update committed: control channel reconnected successfully", "version", m.Version)
		_ = os.Remove(m.BackupPath)
		_ = agentupdate.DeleteMarker(markerPath)
	}
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
