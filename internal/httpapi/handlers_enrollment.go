package httpapi

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/enrollment"
)

type createEnrollmentRequest struct {
	CustomerID       *string  `json:"customer_id"`
	GroupID          *string  `json:"group_id"`
	DisplayName      string   `json:"display_name"`
	ExpiresInSeconds int      `json:"expires_in_seconds"`
	Tags             []string `json:"tags"`
	Reusable         bool     `json:"reusable"`
}

func (h *handlers) handleCreateEnrollment(w http.ResponseWriter, r *http.Request) {
	user, _ := authpkg.UserFromContext(r.Context())
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermEnrollmentCreate) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}

	var req createEnrollmentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}

	var customerID, groupID *uuid.UUID
	if req.CustomerID != nil && *req.CustomerID != "" {
		id, err := uuid.Parse(*req.CustomerID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid customer_id")
			return
		}
		customerID = &id
	}
	if req.GroupID != nil && *req.GroupID != "" {
		id, err := uuid.Parse(*req.GroupID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid group_id")
			return
		}
		groupID = &id
	}
	if !authpkg.HasPermission(grants, authpkg.PermEnrollmentCreate, authpkg.Resource{CustomerID: customerID, GroupID: groupID}) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted for this customer/group scope")
		return
	}

	created, err := h.enroll.Create(r.Context(), enrollment.CreateParams{
		CustomerID:  customerID,
		GroupID:     groupID,
		DisplayName: req.DisplayName,
		ExpiresIn:   time.Duration(req.ExpiresInSeconds) * time.Second,
		Tags:        req.Tags,
		CreatedBy:   user.ID,
		Reusable:    req.Reusable,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to create enrollment")
		return
	}

	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, CustomerID: customerID,
		EventType: audit.EventEnrollmentCreated, Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"enrollment_id": created.ID, "reusable": req.Reusable},
	})

	// The plaintext token is shown exactly once, per docs/API.md §4.
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         created.ID,
		"token":      created.Token,
		"expires_at": created.ExpiresAt,
	}, nil)
}

// handleListEnrollments lists outstanding (still-usable) enrollment
// tokens — never the plaintext/hash, only enough for an admin to tell
// which ones exist and revoke a specific one individually instead of
// nuking every outstanding token via revoke-all.
func (h *handlers) handleListEnrollments(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermEnrollmentCreate) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	list, err := h.enroll.ListOutstanding(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to list enrollment tokens")
		return
	}
	writeJSON(w, http.StatusOK, list, nil)
}

func (h *handlers) handleRevokeEnrollment(w http.ResponseWriter, r *http.Request) {
	user, _ := authpkg.UserFromContext(r.Context())
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermEnrollmentCreate) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid id")
		return
	}
	if err := h.enroll.Revoke(r.Context(), id); err != nil {
		writeErr(w, http.StatusNotFound, "resource_not_found", "Enrollment not found or already used")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &user.ID, EventType: audit.EventEnrollmentRevoked, Result: audit.ResultSuccess, SourceIP: h.clientIP(r), Metadata: map[string]any{"enrollment_id": id}})
	writeJSON(w, http.StatusOK, map[string]any{"state": "revoked"}, nil)
}

// --- Public agent endpoints -------------------------------------------------

var enrollLimiter = authpkg.NewRateLimiter(20, time.Minute)

type agentEnrollRequest struct {
	Token        string `json:"token"`
	InstallID    string `json:"install_id"`
	PublicKey    string `json:"public_key"` // base64
	AgentVersion string `json:"agent_version"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Hostname     string `json:"hostname"`
}

func (h *handlers) handleAgentEnroll(w http.ResponseWriter, r *http.Request) {
	if !enrollLimiter.Allow("enroll:" + h.clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "Too many enrollment attempts")
		return
	}

	var req agentEnrollRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	installID, err := uuid.Parse(req.InstallID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid install_id")
		return
	}
	pubKey, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil || len(pubKey) != ed25519.PublicKeySize {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid public_key")
		return
	}
	if len(req.AgentVersion) > 64 || len(req.OS) > 32 || len(req.Arch) > 32 || len(req.Hostname) > 255 {
		writeErr(w, http.StatusBadRequest, "invalid_request", "field too long")
		return
	}

	result, err := h.enroll.Consume(r.Context(), enrollment.AgentEnrollRequest{
		Token:        req.Token,
		InstallID:    installID,
		PublicKey:    pubKey,
		AgentVersion: req.AgentVersion,
		OS:           req.OS,
		Arch:         req.Arch,
		Hostname:     req.Hostname,
	})
	if err != nil {
		_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorAgent, EventType: audit.EventEnrollmentRejected, Result: audit.ResultFailure, SourceIP: h.clientIP(r), Metadata: map[string]any{"reason": err.Error()}})
		switch {
		case errors.Is(err, enrollment.ErrTokenInvalid):
			writeErr(w, http.StatusUnauthorized, "unauthenticated", "Enrollment token invalid or expired")
		case errors.Is(err, enrollment.ErrInstallExists):
			writeErr(w, http.StatusConflict, "invalid_request", "Install ID already enrolled")
		default:
			writeErr(w, http.StatusInternalServerError, "internal_error", "Enrollment failed")
		}
		return
	}

	_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorAgent, DeviceID: &result.DeviceID, EventType: audit.EventEnrollmentConsumed, Result: audit.ResultSuccess, SourceIP: h.clientIP(r)})
	writeJSON(w, http.StatusCreated, map[string]any{"device_id": result.DeviceID}, nil)
}

// controlLimiter caps handshake ATTEMPTS per source IP, not successful
// connections — a device with a valid identity reconnecting frequently
// (e.g. flapping network) is unaffected since each attempt that reaches
// ServeAgentWS still only counts once per call here, same as any other
// caller. This exists purely as defense-in-depth against connection-flood
// DoS and device-ID enumeration; forging a valid handshake itself is
// already infeasible without the Ed25519 private key.
var controlLimiter = authpkg.NewRateLimiter(120, time.Minute)

func (h *handlers) handleAgentControl(w http.ResponseWriter, r *http.Request) {
	if !controlLimiter.Allow("control:" + h.clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "Too many connection attempts")
		return
	}
	h.hub.ServeAgentWS(w, r)
}
