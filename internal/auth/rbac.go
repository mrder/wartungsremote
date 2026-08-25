package auth

import "github.com/google/uuid"

// Resource describes the scope a permission check applies to. A nil
// CustomerID/GroupID means the resource is not scoped that way (e.g. a
// global action such as user management).
type Resource struct {
	CustomerID *uuid.UUID
	GroupID    *uuid.UUID
}

// HasPermission implements default-deny RBAC per docs/PERMISSIONS.md §5:
// a grant matches if its permission name matches AND (its scope is global,
// OR its scope matches the resource's customer/group).
func HasPermission(grants []PermissionGrant, permission string, resource Resource) bool {
	for _, g := range grants {
		if g.Permission != permission {
			continue
		}
		switch g.Scope {
		case ScopeGlobal:
			return true
		case ScopeCustomer:
			if resource.CustomerID != nil && g.ScopeID != nil && *g.ScopeID == *resource.CustomerID {
				return true
			}
		case ScopeGroup:
			if resource.GroupID != nil && g.ScopeID != nil && *g.ScopeID == *resource.GroupID {
				return true
			}
		}
	}
	return false
}

// HasAnyGrant reports whether the user holds the permission at all, in any
// scope. Useful for coarse UI-affecting checks; MUST NOT be used to
// authorize a specific device/customer action.
func HasAnyGrant(grants []PermissionGrant, permission string) bool {
	for _, g := range grants {
		if g.Permission == permission {
			return true
		}
	}
	return false
}
