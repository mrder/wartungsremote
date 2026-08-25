package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("auth: not found")
var ErrLocked = errors.New("auth: account locked")

type UserStatus string

const (
	UserActive   UserStatus = "active"
	UserDisabled UserStatus = "disabled"
	UserLocked   UserStatus = "locked"
)

type User struct {
	ID               uuid.UUID
	Username         string
	DisplayName      string
	PasswordHash     string
	Status           UserStatus
	MFARequired      bool
	FailedLoginCount int
	LockedUntil      *time.Time
	CreatedAt        time.Time
	LastLoginAt      *time.Time
}

// MarshalJSON deliberately omits PasswordHash. User has no json tags, so a
// naive json.Marshal(user) — e.g. a future handler that returns a User
// directly instead of building a redacted map by hand — would otherwise
// serialize the Argon2 hash straight into an API response. This makes the
// safe behavior the default rather than something every call site has to
// remember; see TestUserJSONNeverLeaksPasswordHash.
func (u User) MarshalJSON() ([]byte, error) {
	type safeUser struct {
		ID               uuid.UUID  `json:"id"`
		Username         string     `json:"username"`
		DisplayName      string     `json:"display_name"`
		Status           UserStatus `json:"status"`
		MFARequired      bool       `json:"mfa_required"`
		FailedLoginCount int        `json:"failed_login_count"`
		LockedUntil      *time.Time `json:"locked_until,omitempty"`
		CreatedAt        time.Time  `json:"created_at"`
		LastLoginAt      *time.Time `json:"last_login_at,omitempty"`
	}
	return json.Marshal(safeUser{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Status: u.Status,
		MFARequired: u.MFARequired, FailedLoginCount: u.FailedLoginCount,
		LockedUntil: u.LockedUntil, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt,
	})
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) GetByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, COALESCE(display_name,''), password_hash, status, mfa_required,
		       failed_login_count, locked_until, created_at, last_login_at
		FROM users WHERE username = $1
	`, username).Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Status, &u.MFARequired,
		&u.FailedLoginCount, &u.LockedUntil, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: get user by username: %w", err)
	}
	return u, nil
}

func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx, `
		SELECT id, username, COALESCE(display_name,''), password_hash, status, mfa_required,
		       failed_login_count, locked_until, created_at, last_login_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Status, &u.MFARequired,
		&u.FailedLoginCount, &u.LockedUntil, &u.CreatedAt, &u.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: get user by id: %w", err)
	}
	return u, nil
}

// List returns every user (id, username, status, ... — never
// PasswordHash) for admin user management / incident response.
func (r *Repo) List(ctx context.Context) ([]User, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, username, COALESCE(display_name,''), '', status, mfa_required,
		       failed_login_count, locked_until, created_at, last_login_at
		FROM users ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("auth: list users: %w", err)
	}
	defer rows.Close()

	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Status, &u.MFARequired,
			&u.FailedLoginCount, &u.LockedUntil, &u.CreatedAt, &u.LastLoginAt); err != nil {
			return nil, fmt.Errorf("auth: scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetStatus changes a user's account status (docs/SECURITY.md §20 "User
// sperren"). Setting anything other than active also clears any temporary
// failed-login lockout, since the explicit status now governs access.
func (r *Repo) SetStatus(ctx context.Context, id uuid.UUID, status UserStatus) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE users SET status = $2, locked_until = NULL, updated_at = now() WHERE id = $1
	`, id, status)
	if err != nil {
		return fmt.Errorf("auth: set user status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMFARequired toggles whether a specific user must set up/use TOTP at
// login. This is a per-user override on top of the server-wide
// admin.require_mfa setting (docs/CONFIGURATION.md) — Login() enforces
// MFA if EITHER is true. Useful to let one test/lab account skip MFA
// without weakening the server's production-wide default.
func (r *Repo) SetMFARequired(ctx context.Context, id uuid.UUID, required bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET mfa_required = $2, updated_at = now() WHERE id = $1`, id, required)
	if err != nil {
		return fmt.Errorf("auth: set mfa required: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateUser creates a new user. Intended for the explicit super-admin
// bootstrap CLI and later admin-driven user management; there is no open
// self-registration per docs/SPECIFICATION.md §12.
func (r *Repo) CreateUser(ctx context.Context, username, displayName, passwordHash string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO users (username, display_name, password_hash, status, mfa_required)
		VALUES ($1, $2, $3, 'active', true)
		RETURNING id
	`, username, displayName, passwordHash).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: create user: %w", err)
	}
	return id, nil
}

func (r *Repo) AssignRole(ctx context.Context, userID, roleID uuid.UUID, scope ScopeType, scopeID *uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id, scope_type, scope_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING
	`, userID, roleID, string(scope), scopeID)
	if err != nil {
		return fmt.Errorf("auth: assign role: %w", err)
	}
	return nil
}

func (r *Repo) RoleIDByName(ctx context.Context, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT id FROM roles WHERE name = $1`, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: role by name: %w", err)
	}
	return id, nil
}

// RegisterFailedLogin increments the failure counter and locks the account
// with exponential-ish backoff once a threshold is exceeded. This is a
// per-account lock (in addition to any IP-based limiter) so it must never be
// permanent, per docs/SECURITY.md §10.
func (r *Repo) RegisterFailedLogin(ctx context.Context, userID uuid.UUID) error {
	const maxFailures = 10
	const lockDuration = 15 * time.Minute

	_, err := r.pool.Exec(ctx, `
		UPDATE users
		SET failed_login_count = failed_login_count + 1,
		    locked_until = CASE
		        WHEN failed_login_count + 1 >= $2 THEN now() + $3::interval
		        ELSE locked_until
		    END,
		    updated_at = now()
		WHERE id = $1
	`, userID, maxFailures, lockDuration)
	if err != nil {
		return fmt.Errorf("auth: register failed login: %w", err)
	}
	return nil
}

func (r *Repo) ResetFailedLogins(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE users SET failed_login_count = 0, locked_until = NULL, last_login_at = now(), updated_at = now()
		WHERE id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("auth: reset failed logins: %w", err)
	}
	return nil
}

// Permissions returns the set of permission names effectively granted to the
// user, each annotated with the scope it applies under.
type PermissionGrant struct {
	Permission string
	Scope      ScopeType
	ScopeID    *uuid.UUID
}

func (r *Repo) PermissionsForUser(ctx context.Context, userID uuid.UUID) ([]PermissionGrant, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT p.name, ur.scope_type, ur.scope_id
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: permissions for user: %w", err)
	}
	defer rows.Close()

	var out []PermissionGrant
	for rows.Next() {
		var g PermissionGrant
		var scope string
		if err := rows.Scan(&g.Permission, &scope, &g.ScopeID); err != nil {
			return nil, fmt.Errorf("auth: scan permission grant: %w", err)
		}
		g.Scope = ScopeType(scope)
		out = append(out, g)
	}
	return out, rows.Err()
}
