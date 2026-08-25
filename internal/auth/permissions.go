package auth

// Permission names, must match the seed data in migrations/0002_seed_rbac.sql
// and the matrix in docs/PERMISSIONS.md.
const (
	PermDeviceRead             = "device.read"
	PermDeviceManage           = "device.manage"
	PermCustomerRead           = "customer.read"
	PermCustomerManage         = "customer.manage"
	PermMonitoringRead         = "monitoring.read"
	PermRemoteTerminal         = "remote.terminal"
	PermRemoteTunnelSSH        = "remote.tunnel.ssh"
	PermRemoteTunnelRDP        = "remote.tunnel.rdp"
	PermRemoteFilesRead        = "remote.files.read"
	PermRemoteFilesWrite       = "remote.files.write"
	PermRemoteServiceControl   = "remote.service.control"
	PermRemoteProcessTerminate = "remote.process.terminate"
	PermRemotePower            = "remote.power"
	PermPrivilegeRequest       = "privilege.request"
	PermMaintenanceRead        = "maintenance.read"
	PermMaintenanceWrite       = "maintenance.write"
	PermAuditRead              = "audit.read"
	PermEnrollmentCreate       = "enrollment.create"
	PermCredentialRevoke       = "credential.revoke"
	PermAgentUpdate            = "agent.update"
	PermUserManage             = "user.manage"
	PermRoleManage             = "role.manage"
	PermSystemSettings         = "system.settings"
	PermAlertManage            = "alert.manage"
)

// ScopeType identifies the granularity a role assignment applies to.
type ScopeType string

const (
	ScopeGlobal   ScopeType = "global"
	ScopeCustomer ScopeType = "customer"
	ScopeGroup    ScopeType = "group"
)
