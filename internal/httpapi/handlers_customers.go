package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/customer"
)

func (h *handlers) handleListCustomers(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermCustomerRead) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	list, err := h.customers.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to list customers")
		return
	}
	writeJSON(w, http.StatusOK, list, nil)
}

type createCustomerRequest struct {
	Name           string `json:"name"`
	CustomerNumber string `json:"customer_number"`
	Notes          string `json:"notes"`
}

func (h *handlers) handleCreateCustomer(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermCustomerManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	var req createCustomerRequest
	if err := decodeJSON(r, &req); err != nil || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	c, err := h.customers.Create(r.Context(), req.Name, req.CustomerNumber, req.Notes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to create customer")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, CustomerID: &c.ID,
		EventType: "customer.created", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
	})
	writeJSON(w, http.StatusCreated, c, nil)
}

func (h *handlers) handleUpdateCustomer(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermCustomerManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid id")
		return
	}
	var req struct {
		Name           string `json:"name"`
		CustomerNumber string `json:"customer_number"`
		Notes          string `json:"notes"`
		Status         string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if err := h.customers.Update(r.Context(), id, req.Name, req.CustomerNumber, req.Notes, req.Status); err != nil {
		writeErr(w, http.StatusNotFound, "resource_not_found", "Customer not found")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, CustomerID: &id,
		EventType: "customer.updated", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "updated"}, nil)
}

func (h *handlers) handleListGroups(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermCustomerRead) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	var customerID *uuid.UUID
	if v := r.URL.Query().Get("customer_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid customer_id")
			return
		}
		customerID = &id
	}
	list, err := h.customers.ListGroups(r.Context(), customerID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to list groups")
		return
	}
	writeJSON(w, http.StatusOK, list, nil)
}

func (h *handlers) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermCustomerManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	var req struct {
		CustomerID *string `json:"customer_id"`
		Name       string  `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	var customerID *uuid.UUID
	if req.CustomerID != nil && *req.CustomerID != "" {
		id, err := uuid.Parse(*req.CustomerID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid customer_id")
			return
		}
		customerID = &id
	}
	g, err := h.customers.CreateGroup(r.Context(), customerID, req.Name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to create group")
		return
	}
	writeJSON(w, http.StatusCreated, g, nil)
}

func (h *handlers) handleRenameGroup(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermCustomerManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid group id")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Name == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if err := h.customers.RenameGroup(r.Context(), id, req.Name); err != nil {
		if err == customer.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "Group not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to rename group")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "group.renamed", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"group_id": id, "name": req.Name},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermCustomerManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid group id")
		return
	}
	if err := h.customers.DeleteGroup(r.Context(), id); err != nil {
		if err == customer.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "Group not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to delete group")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "group.deleted", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"group_id": id},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleListMaintenance(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermMaintenanceRead)
	if !ok {
		return
	}
	list, err := h.maintenance.ListForDevice(r.Context(), d.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to list maintenance history")
		return
	}
	writeJSON(w, http.StatusOK, list, nil)
}
