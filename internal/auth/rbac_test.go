package auth

import "testing"

import "github.com/google/uuid"

func TestHasPermissionGlobalGrantAllowsAnyResource(t *testing.T) {
	custA := uuid.New()
	grants := []PermissionGrant{{Permission: PermDeviceRead, Scope: ScopeGlobal}}

	if !HasPermission(grants, PermDeviceRead, Resource{CustomerID: &custA}) {
		t.Fatal("expected global grant to allow scoped resource")
	}
	if !HasPermission(grants, PermDeviceRead, Resource{}) {
		t.Fatal("expected global grant to allow unscoped resource")
	}
}

func TestHasPermissionCustomerScopeIsRestricted(t *testing.T) {
	custA := uuid.New()
	custB := uuid.New()
	grants := []PermissionGrant{{Permission: PermDeviceRead, Scope: ScopeCustomer, ScopeID: &custA}}

	if !HasPermission(grants, PermDeviceRead, Resource{CustomerID: &custA}) {
		t.Fatal("expected matching customer scope to be allowed")
	}
	if HasPermission(grants, PermDeviceRead, Resource{CustomerID: &custB}) {
		t.Fatal("expected different customer to be denied (no cross-tenant leakage)")
	}
	if HasPermission(grants, PermDeviceRead, Resource{}) {
		t.Fatal("expected unscoped resource to be denied for a customer-scoped grant")
	}
}

func TestHasPermissionDefaultDenyOnEmptyGrants(t *testing.T) {
	if HasPermission(nil, PermDeviceManage, Resource{}) {
		t.Fatal("expected default-deny with no grants")
	}
}

func TestHasPermissionWrongPermissionNameDenied(t *testing.T) {
	grants := []PermissionGrant{{Permission: PermDeviceRead, Scope: ScopeGlobal}}
	if HasPermission(grants, PermDeviceManage, Resource{}) {
		t.Fatal("expected a grant for a different permission name to be denied")
	}
}

func TestHasAnyGrant(t *testing.T) {
	grants := []PermissionGrant{{Permission: PermAuditRead, Scope: ScopeGlobal}}
	if !HasAnyGrant(grants, PermAuditRead) {
		t.Fatal("expected HasAnyGrant to find matching permission")
	}
	if HasAnyGrant(grants, PermUserManage) {
		t.Fatal("expected HasAnyGrant to reject non-granted permission")
	}
}
