// Package httpapi implements the WartungsRemote HTTP API V1 (docs/API.md)
// and wires together auth, enrollment, device registry and the agent
// control channel.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"wartungsremote/internal/agentrelease"
	"wartungsremote/internal/alerting"
	"wartungsremote/internal/appsettings"
	"wartungsremote/internal/audit"
	"wartungsremote/internal/auth"
	"wartungsremote/internal/config"
	"wartungsremote/internal/controlhub"
	"wartungsremote/internal/customer"
	"wartungsremote/internal/device"
	"wartungsremote/internal/enrollment"
	"wartungsremote/internal/help"
	"wartungsremote/internal/maintenance"
	"wartungsremote/internal/monitoring"
	"wartungsremote/internal/netutil"
	"wartungsremote/internal/notify"
	"wartungsremote/internal/relay"
	"wartungsremote/internal/remotesession"
	"wartungsremote/internal/support"
)

type Dependencies struct {
	Pool    *pgxpool.Pool
	Config  config.ServerConfig
	Audit   *audit.Logger
	Version string
}

type Router struct {
	Public http.Handler
	Admin  http.Handler

	hubCancel context.CancelFunc
}

func (r *Router) Close() {
	if r.hubCancel != nil {
		r.hubCancel()
	}
}

func NewRouter(deps Dependencies) (*Router, error) {
	cfg := deps.Config

	// Already validated at config load time (config.ServerConfig.Validate);
	// re-parsing here just turns it into the matcher both the HTTP handlers
	// and the control channel need.
	trustedProxies, err := netutil.ParseTrustedProxies(cfg.Security.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("httpapi: %w", err)
	}

	devices := device.NewRepo(deps.Pool)
	healthEngine := monitoring.NewEngine(devices, monitoring.DefaultThresholds())
	enroll := enrollment.New(deps.Pool)

	sessions := auth.NewSessionStore(deps.Pool, cfg.Security.SessionCookieName, cfg.Admin.SessionAbsoluteTTL, cfg.Admin.SessionIdleTTL)
	mfaStore := auth.NewMFAStore(deps.Pool, deps.Config.Secrets.TOTPEncryptionKey)
	argon2Params := auth.Argon2Params{
		MemoryKiB:   cfg.Security.Argon2MemoryKiB,
		Iterations:  cfg.Security.Argon2Iterations,
		Parallelism: cfg.Security.Argon2Parallelism,
		SaltLen:     16,
		KeyLen:      32,
	}
	authSvc := auth.NewService(deps.Pool, sessions, mfaStore, argon2Params, cfg.Admin.PrivilegeTTL, "WartungsRemote")

	hubCtx, hubCancel := context.WithCancel(context.Background())
	hub := controlhub.NewHub(devices, healthEngine, deps.Audit, controlhub.Timing{
		HeartbeatInterval:     cfg.Agent.HeartbeatInterval,
		ConnectionLostAfter:   cfg.Agent.ConnectionLostAfter,
		OfflineAfter:          cfg.Agent.OfflineAfter,
		StatusInterval:        cfg.Agent.StatusInterval,
		NetworkUploadInterval: cfg.Agent.NetworkUploadInterval,
	}, trustedProxies)
	go hub.Run(hubCtx)

	broker := relay.NewBroker(hub)
	sessionRepo := remotesession.NewRepo(deps.Pool)
	privilegeRepo := remotesession.NewPrivilegeRepo(deps.Pool)
	tunnelRepo := remotesession.NewTunnelRepo(deps.Pool)
	maintenanceRepo := maintenance.NewRepo(deps.Pool)
	sessionSvc := remotesession.NewService(sessionRepo, hub, maintenanceRepo, deps.Audit)
	go sessionSvc.RunPrivilegeExpirySweeper(hubCtx, privilegeRepo)
	customers := customer.NewRepo(deps.Pool)

	// V1 simplification: invalidate any outstanding (unused) tunnel tickets
	// on startup rather than trying to resume them across a restart
	// (docs/RELAY.md §8).
	_ = tunnelRepo.InvalidateAllOutstanding(context.Background())

	// docs/SPECIFICATION.md §23: "Sessions nach Neustart als interrupted
	// markieren" — a server restart drops every control-channel connection,
	// so any session left 'active' at that moment would otherwise hang
	// until its own expiry with no agent ever able to close it.
	if interrupted, err := sessionRepo.InterruptAllActive(context.Background()); err == nil {
		for _, s := range interrupted {
			if s.MaintenanceSessionID != nil {
				_ = maintenanceRepo.Close(context.Background(), *s.MaintenanceSessionID, "interrupted", "Server restarted while session was active")
			}
			_ = deps.Audit.Record(context.Background(), audit.Event{
				ActorType: audit.ActorSystem, SessionID: &s.ID,
				EventType: "remote_session.interrupted", Result: audit.ResultSuccess,
				Metadata: map[string]any{"reason": "server_restart"},
			})
		}
	}

	settingsRepo := appsettings.NewRepo(deps.Pool)
	go device.RunMetricsRetentionSweeper(hubCtx, devices, settingsRepo, cfg.Metrics.RawRetention, cfg.Metrics.HourlyRetention, cfg.Metrics.NetworkRawRetention, cfg.Metrics.NetworkHourlyRetention, 10*time.Minute)

	telegramRepo := notify.NewTelegramRepo(deps.Pool, deps.Config.Secrets.TOTPEncryptionKey)

	alertsRepo := alerting.NewRepo(deps.Pool)
	alertEngine := alerting.NewEngine(alertsRepo, devices, hub, deps.Audit, telegramRepo)
	go alerting.RunSweeper(hubCtx, alertEngine, time.Minute)

	releasesRepo := agentrelease.NewRepo(deps.Pool)
	hub.SetVersionBlockedChecker(releasesRepo.IsVersionBlocked)

	supportRepo := support.NewRepo(deps.Pool, deps.Config.Secrets.TOTPEncryptionKey)
	hub.SetSupportCredentialHandler(supportRepo.Upsert)
	go support.RunRotationSweeper(hubCtx, supportRepo, settingsRepo, hub, time.Hour)

	helpSections, err := help.Load(cfg.Help.ContentDir)
	if err != nil {
		slog.Warn("dashboard help content unavailable", "error", err)
	}

	// Agent-initiated close (e.g. the terminal process exited on its own):
	// clean up server-side state and unblock any browser still reading from
	// the broker stream. See docs/STATE_MACHINES.md §3.
	hub.SetSessionCloseHandler(func(deviceID uuid.UUID, sessionIDStr, reason string) {
		sessionID, err := uuid.Parse(sessionIDStr)
		if err != nil {
			return
		}
		sess, err := sessionRepo.Get(hubCtx, sessionID)
		if err != nil || sess.DeviceID != deviceID || sess.State != remotesession.StateActive {
			return
		}
		broker.Unregister(sessionID)
		_ = sessionSvc.Close(hubCtx, sess, "agent_"+reason)
	})

	// Agent connection lost entirely: interrupt every active remote session
	// for that device immediately instead of leaving the browser side
	// hanging until the session's own expiry (docs/RELAY.md §8,
	// docs/STATE_MACHINES.md §3 — INTERRUPTED, never silently re-ACTIVE).
	hub.SetDeviceDisconnectHandler(func(deviceID uuid.UUID) {
		interrupted, err := sessionRepo.InterruptAllForDevice(hubCtx, deviceID)
		if err != nil {
			return
		}
		for _, s := range interrupted {
			broker.Unregister(s.ID)
			if s.MaintenanceSessionID != nil {
				_ = maintenanceRepo.Close(hubCtx, *s.MaintenanceSessionID, "interrupted", "Agent disconnected while session was active")
			}
			_ = deps.Audit.Record(hubCtx, audit.Event{
				ActorType: audit.ActorSystem,
				DeviceID:  &deviceID,
				SessionID: &s.ID,
				EventType: "remote_session.interrupted",
				Result:    audit.ResultSuccess,
				Metadata:  map[string]any{"reason": "agent_disconnected"},
			})
		}
	})

	h := &handlers{
		cfg:            cfg,
		trustedProxies: trustedProxies,
		devices:        devices,
		enroll:         enroll,
		auth:           authSvc,
		hub:            hub,
		health:         healthEngine,
		audit:          deps.Audit,
		version:        deps.Version,
		sessions:       sessionSvc,
		sessionRepo:    sessionRepo,
		privilege:      privilegeRepo,
		tunnels:        tunnelRepo,
		broker:         broker,
		privilegeTTL:   cfg.Admin.PrivilegeTTL,
		maintenance:    maintenanceRepo,
		customers:      customers,
		alerts:         alertsRepo,
		releases:       releasesRepo,
		settings:       settingsRepo,
		support:        supportRepo,
		telegram:       telegramRepo,
		help:           helpSections,
	}

	public := http.NewServeMux()
	public.HandleFunc("GET /health", h.handleHealth)
	public.HandleFunc("POST /api/v1/agent/enroll", h.handleAgentEnroll)
	public.HandleFunc("GET /api/v1/agent/control", h.handleAgentControl)
	// wr-helper is a native binary with no browser session; it authenticates
	// with a single-use ticket instead, so this lives on the public listener
	// rather than the session-cookie-protected admin one. See docs/RELAY.md.
	public.HandleFunc("GET /api/v1/tunnels/stream", h.handleTunnelStream)

	admin := http.NewServeMux()
	admin.HandleFunc("GET /health", h.handleHealth)
	admin.HandleFunc("POST /api/v1/auth/login", h.handleLogin)
	admin.HandleFunc("POST /api/v1/auth/totp", h.handleTOTP)
	admin.HandleFunc("POST /api/v1/auth/mfa-setup", h.handleMFASetupConfirm)
	admin.HandleFunc("POST /api/v1/auth/logout", h.handleLogout)
	admin.HandleFunc("POST /api/v1/auth/reauth", h.handleReauth)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/v1/me", h.handleMe)
	protected.HandleFunc("POST /api/v1/auth/change-password", h.handleChangePassword)
	protected.HandleFunc("POST /api/v1/enrollments", h.handleCreateEnrollment)
	protected.HandleFunc("GET /api/v1/enrollments", h.handleListEnrollments)
	protected.HandleFunc("DELETE /api/v1/enrollments/{id}", h.handleRevokeEnrollment)
	protected.HandleFunc("GET /api/v1/devices", h.handleListDevices)
	protected.HandleFunc("GET /api/v1/devices/{id}", h.handleGetDevice)
	protected.HandleFunc("PATCH /api/v1/devices/{id}", h.handlePatchDevice)
	protected.HandleFunc("POST /api/v1/devices/{id}/status-request", h.handleStatusRequest)
	protected.HandleFunc("GET /api/v1/devices/{id}/health", h.handleDeviceHealth)
	protected.HandleFunc("GET /api/v1/devices/{id}/ip-history", h.handleDeviceIPHistory)
	protected.HandleFunc("GET /api/v1/devices/{id}/metrics", h.handleDeviceMetrics)
	protected.HandleFunc("GET /api/v1/devices/{id}/network-metrics", h.handleDeviceNetworkMetrics)
	protected.HandleFunc("GET /api/v1/network-usage", h.handleNetworkUsageSummary)
	protected.HandleFunc("POST /api/v1/devices/{id}/revoke", h.handleRevokeDevice)
	protected.HandleFunc("GET /api/v1/devices/{id}/audit", h.handleDeviceAuditLog)
	protected.HandleFunc("GET /api/v1/audit", h.handleAuditLog)
	protected.HandleFunc("GET /api/v1/audit/export", h.handleExportAuditLog)
	protected.HandleFunc("POST /api/v1/audit/verify", h.handleVerifyAuditChain)

	protected.HandleFunc("POST /api/v1/devices/{id}/sessions", h.handleCreateSession)
	protected.HandleFunc("GET /api/v1/sessions/{id}", h.handleGetSession)
	protected.HandleFunc("DELETE /api/v1/sessions/{id}", h.handleCloseSession)
	protected.HandleFunc("GET /api/v1/sessions/{id}/stream", h.handleSessionStream)
	protected.HandleFunc("POST /api/v1/sessions/{id}/privilege", h.handleGrantPrivilege)
	protected.HandleFunc("DELETE /api/v1/sessions/{id}/privilege", h.handleRevokePrivilege)

	// Closing a tunnel reuses DELETE /api/v1/sessions/:id (the session_id
	// returned from tunnel creation) — remote_sessions and its close path
	// already handle every kind, terminal or tunnel, generically.
	protected.HandleFunc("POST /api/v1/devices/{id}/tunnels", h.handleCreateTunnel)

	protected.HandleFunc("GET /api/v1/devices/{id}/services", h.handleListServices)
	protected.HandleFunc("POST /api/v1/devices/{id}/services/{service}/start", h.handleServiceAction)
	protected.HandleFunc("POST /api/v1/devices/{id}/services/{service}/stop", h.handleServiceAction)
	protected.HandleFunc("POST /api/v1/devices/{id}/services/{service}/restart", h.handleServiceAction)

	protected.HandleFunc("GET /api/v1/devices/{id}/processes", h.handleListProcesses)
	protected.HandleFunc("POST /api/v1/devices/{id}/processes/{pid}/terminate", h.handleTerminateProcess)

	protected.HandleFunc("GET /api/v1/devices/{id}/logs", h.handleQueryLogs)

	protected.HandleFunc("GET /api/v1/devices/{id}/files", h.handleFilesList)
	protected.HandleFunc("GET /api/v1/devices/{id}/files/download", h.handleFilesDownload)
	protected.HandleFunc("POST /api/v1/devices/{id}/files/upload", h.handleFilesUpload)
	protected.HandleFunc("POST /api/v1/devices/{id}/files/mkdir", h.handleFilesMkdir)
	protected.HandleFunc("POST /api/v1/devices/{id}/files/rename", h.handleFilesRename)
	protected.HandleFunc("DELETE /api/v1/devices/{id}/files", h.handleFilesDelete)

	protected.HandleFunc("GET /api/v1/customers", h.handleListCustomers)
	protected.HandleFunc("POST /api/v1/customers", h.handleCreateCustomer)
	protected.HandleFunc("PATCH /api/v1/customers/{id}", h.handleUpdateCustomer)
	protected.HandleFunc("GET /api/v1/groups", h.handleListGroups)
	protected.HandleFunc("POST /api/v1/groups", h.handleCreateGroup)
	protected.HandleFunc("PATCH /api/v1/groups/{id}", h.handleRenameGroup)
	protected.HandleFunc("DELETE /api/v1/groups/{id}", h.handleDeleteGroup)
	protected.HandleFunc("GET /api/v1/devices/{id}/maintenance", h.handleListMaintenance)

	protected.HandleFunc("GET /api/v1/alert-rules", h.handleListAlertRules)
	protected.HandleFunc("POST /api/v1/alert-rules", h.handleCreateAlertRule)
	protected.HandleFunc("PATCH /api/v1/alert-rules/{id}", h.handleSetAlertRuleEnabled)
	protected.HandleFunc("DELETE /api/v1/alert-rules/{id}", h.handleDeleteAlertRule)
	protected.HandleFunc("GET /api/v1/alerts", h.handleListAlerts)
	protected.HandleFunc("GET /api/v1/alerts/open-count", h.handleAlertsOpenCount)
	protected.HandleFunc("GET /api/v1/devices/{id}/support-credential", h.handleGetSupportCredential)
	protected.HandleFunc("POST /api/v1/devices/{id}/support-credential/rotate", h.handleRotateSupportCredential)
	protected.HandleFunc("GET /api/v1/settings/retention", h.handleGetRetentionSettings)
	protected.HandleFunc("PATCH /api/v1/settings/retention", h.handleSetRetentionSettings)
	protected.HandleFunc("GET /api/v1/settings/network-retention", h.handleGetNetworkRetentionSettings)
	protected.HandleFunc("PATCH /api/v1/settings/network-retention", h.handleSetNetworkRetentionSettings)
	protected.HandleFunc("GET /api/v1/settings/support-credential-rotation", h.handleGetSupportCredentialRotationSettings)
	protected.HandleFunc("PATCH /api/v1/settings/support-credential-rotation", h.handleSetSupportCredentialRotationSettings)
	protected.HandleFunc("GET /api/v1/settings/telegram", h.handleGetTelegramSettings)
	protected.HandleFunc("PATCH /api/v1/settings/telegram", h.handleSetTelegramSettings)
	protected.HandleFunc("POST /api/v1/settings/telegram/test", h.handleTestTelegramSettings)
	protected.HandleFunc("POST /api/v1/alerts/{id}/acknowledge", h.handleAcknowledgeAlert)
	protected.HandleFunc("POST /api/v1/alerts/{id}/resolve", h.handleResolveAlert)
	protected.HandleFunc("DELETE /api/v1/alerts/{id}", h.handleDeleteAlert)

	protected.HandleFunc("GET /api/v1/agent/releases", h.handleListReleases)
	protected.HandleFunc("POST /api/v1/agent/releases", h.handleCreateRelease)
	protected.HandleFunc("PATCH /api/v1/agent/releases/{id}", h.handleSetReleaseBlocked)
	protected.HandleFunc("POST /api/v1/devices/{id}/update", h.handleTriggerDeviceUpdate)

	protected.HandleFunc("POST /api/v1/enrollments/revoke-all", h.handleRevokeAllEnrollments)
	protected.HandleFunc("GET /api/v1/users", h.handleListUsers)
	protected.HandleFunc("PATCH /api/v1/users/{id}", h.handleSetUserStatus)
	protected.HandleFunc("POST /api/v1/users/{id}/revoke-sessions", h.handleRevokeUserSessions)

	protected.HandleFunc("GET /api/v1/help/index", h.handleHelpIndex)
	protected.HandleFunc("GET /api/v1/help/{slug}", h.handleHelpPage)

	admin.Handle("/api/v1/", withCSRF(cfg, auth.RequireSession(authSvc)(protected)))

	router := &Router{
		Public:    withSecurityHeaders(cfg, public),
		Admin:     withSecurityHeaders(cfg, admin),
		hubCancel: hubCancel,
	}
	return router, nil
}

func (h *handlers) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "ok", "version": h.version}})
}
