package auth

import (
	"context"
	"encoding/json"
	"net/http"
)

type ctxKey int

const (
	ctxKeySession ctxKey = iota
	ctxKeyUser
	ctxKeyGrants
)

// RequireSession is HTTP middleware that resolves the session cookie,
// rejecting the request with 401 if absent/expired/revoked. The frontend is
// never the authority for this; every state-changing admin API route must be
// wrapped by this middleware server-side (docs/AI_IMPLEMENTATION_GUIDE.md §8).
func RequireSession(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := svc.Sessions.ReadCookie(r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated", "No active session")
				return
			}
			sess, err := svc.Sessions.Validate(r.Context(), token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated", "Session invalid or expired")
				return
			}
			user, err := svc.Repo.GetByID(r.Context(), sess.UserID)
			if err != nil || user.Status != UserActive {
				writeError(w, http.StatusUnauthorized, "unauthenticated", "Session invalid or expired")
				return
			}
			grants, err := svc.Repo.PermissionsForUser(r.Context(), user.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "Failed to resolve permissions")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeySession, sess)
			ctx = context.WithValue(ctx, ctxKeyUser, user)
			ctx = context.WithValue(ctx, ctxKeyGrants, grants)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func SessionFromContext(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(ctxKeySession).(Session)
	return s, ok
}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKeyUser).(User)
	return u, ok
}

func GrantsFromContext(ctx context.Context) []PermissionGrant {
	g, _ := ctx.Value(ctxKeyGrants).([]PermissionGrant)
	return g
}

// RequirePermission returns middleware that denies (403) unless the caller's
// grants include permission for the resource resolved by resourceFn. Must be
// chained after RequireSession. Default-deny: any resolution failure denies.
func RequirePermission(permission string, resourceFn func(r *http.Request) Resource) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			grants := GrantsFromContext(r.Context())
			var res Resource
			if resourceFn != nil {
				res = resourceFn(r)
			}
			if !HasPermission(grants, permission, res) {
				writeError(w, http.StatusForbidden, "permission_denied", "Not permitted")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GlobalResource is a convenience resourceFn for permissions that are not
// scoped to a specific customer/group (e.g. user.manage).
func GlobalResource(*http.Request) Resource { return Resource{} }

// writeError renders the standard API error envelope from docs/API.md §1.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

// WriteError is the exported form for use by other packages (httpapi).
var WriteError = writeError
