// API client for the WartungsRemote admin dashboard. Auth is entirely
// cookie-based (server-set __Host- session cookie, HttpOnly) — this file
// never reads or stores an auth token itself, per docs/SECURITY.md §9.
// State-changing requests carry the CSRF double-submit header.

const BASE = '/api/v1'

export class ApiError extends Error {
  code: string
  status: number
  constructor(status: number, code: string, message: string) {
    super(message)
    this.code = code
    this.status = status
  }
}

function readCsrfCookie(): string | null {
  const match = document.cookie.match(/(?:^|; )wr_csrf=([^;]*)/)
  return match ? decodeURIComponent(match[1]) : null
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const csrf = readCsrfCookie()
  if (csrf && method !== 'GET') headers['X-CSRF-Token'] = csrf

  const resp = await fetch(BASE + path, {
    method,
    headers,
    credentials: 'include',
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  const contentType = resp.headers.get('content-type') || ''
  const data = contentType.includes('application/json') ? await resp.json() : null

  if (!resp.ok) {
    const err = data?.error
    throw new ApiError(resp.status, err?.code ?? 'unknown', err?.message ?? resp.statusText)
  }
  return (data?.data ?? data) as T
}

export const api = {
  get: <T,>(path: string) => request<T>('GET', path),
  post: <T,>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  patch: <T,>(path: string, body?: unknown) => request<T>('PATCH', path, body ?? {}),
  del: <T,>(path: string, body?: unknown) => request<T>('DELETE', path, body),
}

// --- Types matching docs/API.md -------------------------------------------

export interface Advisory {
  code: string
  severity: 'info' | 'warning' | 'critical'
  message: string
}

export interface Me {
  id: string
  username: string
  display_name: string
  permissions: string[]
  mfa_confirmed: boolean
  session_expires_at: string
  public_base_url: string
  server_version: string
  advisories?: Advisory[]
}

export type LoginState = 'mfa_required' | 'mfa_setup_required' | 'authenticated'

export interface LoginResponse {
  state: LoginState
  challenge_id?: string
  setup_uri?: string
}

export interface Device {
  id: string
  install_id: string
  customer_id: string | null
  group_id: string | null
  display_name: string
  hostname: string
  os_family: string
  os_name: string
  os_version: string
  architecture: string
  agent_version: string
  status: 'unknown' | 'online' | 'connection_lost' | 'offline' | 'revoked'
  health: 'healthy' | 'warning' | 'critical' | 'offline' | 'unknown'
  health_reasons: string[]
  tags: string[]
  last_seen_at: string | null
  last_public_ip: string
  transport_secure: boolean | null
  online?: boolean
  support_credential_available?: boolean
  support_credential_updated_at?: string
}

export interface IPHistoryEntry {
  IP: string
  FirstSeen: string
  LastSeen: string
}

export interface AuditEntry {
  ID: number
  OccurredAt: string
  ActorType: string
  ActorID: string | null
  ActorUsername: string | null
  DeviceID: string | null
  EventType: string
  Result: string
  SourceIP: string
  Metadata: Record<string, unknown>
}

export interface EnrollmentCreated {
  id: string
  token: string
  expires_at: string
}

export interface HelpIndexEntry {
  slug: string
  title: string
}

export interface HelpPage {
  slug: string
  title: string
  html: string
}

export const HelpApi = {
  index: () => api.get<HelpIndexEntry[]>('/help/index'),
  page: (slug: string) => api.get<HelpPage>(`/help/${slug}`),
}

export interface ChainVerification {
  Valid: boolean
  EntriesCheck: number
  EntriesPreChain: number
  BrokenAtID: number | null
}

export const AuditApi = {
  exportUrl: (format: 'json' | 'csv', deviceId?: string) => {
    const qs = new URLSearchParams({ format })
    if (deviceId) qs.set('device_id', deviceId)
    return `${BASE}/audit/export?${qs.toString()}`
  },
  verifyChain: () => api.post<ChainVerification>('/audit/verify', {}),
}

export const AuthApi = {
  login: (username: string, password: string) => api.post<LoginResponse>('/auth/login', { username, password }),
  totp: (challenge_id: string, code: string) => api.post<LoginResponse>('/auth/totp', { challenge_id, code }),
  confirmMfaSetup: (username: string, password: string, code: string) =>
    api.post<{ state: string; recovery_codes: string[] }>('/auth/mfa-setup', { username, password, code }),
  logout: () => api.post('/auth/logout'),
  me: () => api.get<Me>('/me'),
}

export const DeviceApi = {
  list: (params: Record<string, string> = {}) => {
    const qs = new URLSearchParams(params).toString()
    return api.get<Device[]>('/devices' + (qs ? '?' + qs : ''))
  },
  get: (id: string) => api.get<Device>(`/devices/${id}`),
  statusRequest: (id: string) => api.post(`/devices/${id}/status-request`),
  audit: (id: string) => api.get<AuditEntry[]>(`/devices/${id}/audit`),
  metrics: (id: string, resolution: 'raw' | 'hourly' = 'raw') =>
    api.get<Array<{ observed_at: string; cpu_percent: number; memory_used_bytes: number; memory_total_bytes: number; disk_used_bytes: number; disk_total_bytes: number }>>(
      `/devices/${id}/metrics?resolution=${resolution}`
    ),
  networkMetrics: (id: string, resolution: 'raw' | 'hourly' = 'raw') =>
    api.get<NetworkMetricsPoint[]>(`/devices/${id}/network-metrics?resolution=${resolution}`),
  patch: (id: string, body: { display_name?: string; customer_id?: string | null; group_id?: string | null; tags?: string[] }) =>
    api.patch(`/devices/${id}`, body),
  maintenance: (id: string) => api.get<MaintenanceSession[]>(`/devices/${id}/maintenance`),
  ipHistory: (id: string, hours = 24) => api.get<IPHistoryEntry[]>(`/devices/${id}/ip-history?hours=${hours}`),
  revoke: (id: string, reauthId: string) => api.post(`/devices/${id}/revoke`, { reauth_id: reauthId }),
  // Only succeeds server-side for a device that has never connected
  // (no last_seen_at) — anything with real history must be revoked
  // instead, never hard-deleted.
  delete: (id: string, reauthId: string) => api.del(`/devices/${id}`, { reauth_id: reauthId }),
}

export interface NetworkMetricsPoint {
  observed_at: string
  bytes_sent_total: number
  bytes_recv_total: number
  bytes_sent_control: number
  bytes_recv_control: number
  bytes_sent_total_per_sec: number
  bytes_recv_total_per_sec: number
  bytes_sent_control_per_sec: number
  bytes_recv_control_per_sec: number
}

export interface DeviceNetworkTotal {
  device_id: string
  display_name: string
  bytes_sent_total: number
  bytes_recv_total: number
  bytes_sent_control: number
  bytes_recv_control: number
}

export const NetworkUsageApi = {
  summary: (hours = 24) => api.get<DeviceNetworkTotal[]>(`/network-usage?hours=${hours}`),
}

export interface MaintenanceSession {
  ID: string
  DeviceID: string
  UserID: string
  StartedAt: string
  EndedAt: string | null
  Result: string
  Summary: string
}

export interface Customer {
  ID: string
  Name: string
  CustomerNumber: string
  Status: string
  Notes: string
}

export interface Group {
  ID: string
  CustomerID: string | null
  Name: string
}

export const CustomerApi = {
  list: () => api.get<Customer[]>('/customers'),
  create: (name: string, customerNumber: string, notes: string) =>
    api.post<Customer>('/customers', { name, customer_number: customerNumber, notes }),
  update: (id: string, name: string, customerNumber: string, notes: string, status: string) =>
    api.patch(`/customers/${id}`, { name, customer_number: customerNumber, notes, status }),
  groups: (customerId?: string) => api.get<Group[]>('/groups' + (customerId ? `?customer_id=${customerId}` : '')),
  createGroup: (name: string, customerId?: string) => api.post<Group>('/groups', { name, customer_id: customerId }),
  renameGroup: (id: string, name: string) => api.patch(`/groups/${id}`, { name }),
  deleteGroup: (id: string) => api.del(`/groups/${id}`),
}

export interface OutstandingEnrollment {
  ID: string
  DisplayName: string | null
  CustomerID: string | null
  IsReusable: boolean
  UseCount: number
  LastUsedAt: string | null
  ExpiresAt: string
  CreatedAt: string
}

export const EnrollmentApi = {
  create: (displayName: string, expiresInSeconds: number, reusable = false) =>
    api.post<EnrollmentCreated>('/enrollments', { display_name: displayName, expires_in_seconds: expiresInSeconds, reusable }),
  list: () => api.get<OutstandingEnrollment[]>('/enrollments'),
  revoke: (id: string) => api.del(`/enrollments/${id}`),
  revokeAll: () => api.post<{ revoked_count: number }>('/enrollments/revoke-all'),
}

export interface AdminUser {
  id: string
  username: string
  display_name: string
  status: 'active' | 'disabled' | 'locked'
  mfa_required: boolean
  created_at: string
  last_login_at: string | null
}

export const UserApi = {
  list: () => api.get<AdminUser[]>('/users'),
  create: (username: string, displayName: string, role: string) =>
    api.post<{ id: string; username: string; password: string }>('/users', { username, display_name: displayName, role }),
  setStatus: (id: string, status: 'active' | 'disabled' | 'locked') => api.patch(`/users/${id}`, { status }),
  setMfaRequired: (id: string, mfaRequired: boolean) => api.patch(`/users/${id}`, { mfa_required: mfaRequired }),
  revokeSessions: (id: string) => api.post(`/users/${id}/revoke-sessions`),
}

// --- Remote sessions / terminal / privilege --------------------------------

export interface RemoteSession {
  session_id: string
  state: string
  expires_at: string
}

export const SessionApi = {
  createTerminal: (deviceId: string) => api.post<RemoteSession>(`/devices/${deviceId}/sessions`, { kind: 'terminal' }),
  close: (sessionId: string) => api.del(`/sessions/${sessionId}`),
  grantPrivilege: (sessionId: string, reauthId: string, durationSeconds: number) =>
    api.post<{ privilege_session_id: string; valid_until: string }>(`/sessions/${sessionId}/privilege`, {
      reauth_id: reauthId,
      duration_seconds: durationSeconds,
    }),
  revokePrivilege: (sessionId: string) => api.del(`/sessions/${sessionId}/privilege`),
}

export interface TunnelCreated {
  tunnel_id: string
  session_id: string
  helper_ticket: string
  expires_at: string
}

export const TunnelApi = {
  create: (deviceId: string, targetType: 'ssh_local' | 'rdp_local') =>
    api.post<TunnelCreated>(`/devices/${deviceId}/tunnels`, { target_type: targetType }),
}

export const SupportCredentialApi = {
  get: (deviceId: string) =>
    api.get<{ username: string; password: string; updated_at: string }>(`/devices/${deviceId}/support-credential`),
  rotate: (deviceId: string) => api.post(`/devices/${deviceId}/support-credential/rotate`),
}

export interface AlertRule {
  ID: string
  ScopeType: 'global' | 'customer' | 'group' | 'device'
  ScopeID: string | null
  RuleType: 'offline' | 'cpu' | 'ram' | 'disk' | 'service' | 'agent_version'
  Config: Record<string, unknown>
  Enabled: boolean
}

export interface Alert {
  ID: string
  DeviceID: string
  RuleID: string | null
  Severity: 'warning' | 'critical'
  State: 'open' | 'acknowledged' | 'resolved'
  OpenedAt: string
  ResolvedAt: string | null
  Summary: string
}

export const AlertApi = {
  listRules: () => api.get<AlertRule[]>('/alert-rules'),
  createRule: (rule: { scope_type: string; scope_id?: string; rule_type: string; config: Record<string, unknown>; enabled?: boolean }) =>
    api.post<AlertRule>('/alert-rules', rule),
  setRuleEnabled: (id: string, enabled: boolean) => api.patch(`/alert-rules/${id}`, { enabled }),
  deleteRule: (id: string) => api.del(`/alert-rules/${id}`),
  list: (params: { device_id?: string; state?: string } = {}) => {
    const qs = new URLSearchParams(params as Record<string, string>).toString()
    return api.get<Alert[]>('/alerts' + (qs ? '?' + qs : ''))
  },
  openCount: () => api.get<{ open_count: number }>('/alerts/open-count'),
  acknowledge: (id: string) => api.post(`/alerts/${id}/acknowledge`),
  resolve: (id: string) => api.post(`/alerts/${id}/resolve`),
  delete: (id: string) => api.del(`/alerts/${id}`),
}

export interface AgentRelease {
  ID: string
  Version: string
  OSFamily: string
  Architecture: string
  Channel: string
  ArtifactURL: string
  ArtifactSHA256Hex: string
  SignatureBase64: string
  PublishedAt: string
  MinimumSupported: boolean
  Blocked: boolean
}

export const ReleaseApi = {
  list: () => api.get<AgentRelease[]>('/agent/releases'),
  create: (rl: {
    version: string; os_family: string; architecture: string; channel?: string
    artifact_url: string; artifact_sha256_hex: string; signature_base64: string; minimum_supported?: boolean
  }) => api.post<AgentRelease>('/agent/releases', rl),
  setBlocked: (id: string, blocked: boolean) => api.patch(`/agent/releases/${id}`, { blocked }),
  triggerUpdate: (deviceId: string, channel?: string) => api.post<{ target_version: string }>(`/devices/${deviceId}/update`, { channel }),
  syncFromGitHub: () => api.post<{ imported: number; skipped: number; errors: string[] }>('/agent/releases/sync'),
}

export const ReauthApi = {
  reauth: (password: string, code: string) => api.post<{ reauth_id: string }>('/auth/reauth', { password, code }),
}

export const AccountApi = {
  changePassword: (reauthId: string, newPassword: string) =>
    api.post('/auth/change-password', { reauth_id: reauthId, new_password: newPassword }),
}

export const SettingsApi = {
  getRetention: () => api.get<{ raw_retention_hours: number; hourly_retention_hours: number }>('/settings/retention'),
  setRetention: (rawRetentionHours: number, hourlyRetentionHours: number) =>
    api.patch('/settings/retention', { raw_retention_hours: rawRetentionHours, hourly_retention_hours: hourlyRetentionHours }),
  getNetworkRetention: () => api.get<{ raw_retention_hours: number; hourly_retention_hours: number }>('/settings/network-retention'),
  setNetworkRetention: (rawRetentionHours: number, hourlyRetentionHours: number) =>
    api.patch('/settings/network-retention', { raw_retention_hours: rawRetentionHours, hourly_retention_hours: hourlyRetentionHours }),
  getSupportCredentialRotation: () => api.get<{ rotation_days: number }>('/settings/support-credential-rotation'),
  setSupportCredentialRotation: (rotationDays: number) =>
    api.patch('/settings/support-credential-rotation', { rotation_days: rotationDays }),
  getTelegram: () => api.get<{ configured: boolean; chat_id: string; updated_at: string }>('/settings/telegram'),
  setTelegram: (botToken: string, chatId: string) => api.patch('/settings/telegram', { bot_token: botToken, chat_id: chatId }),
  testTelegram: () => api.post('/settings/telegram/test'),
}

// --- Services / Processes ---------------------------------------------------

export interface ServiceInfo {
  name: string
  display_name?: string
  status: string
}

export interface ProcessInfo {
  pid: number
  name: string
  cpu_percent: number
  memory_rss_bytes: number
  username?: string
  start_time_unix_ms: number
}

export const ServiceApi = {
  list: (deviceId: string) => api.get<ServiceInfo[]>(`/devices/${deviceId}/services`),
  action: (deviceId: string, name: string, action: 'start' | 'stop' | 'restart') =>
    api.post(`/devices/${deviceId}/services/${encodeURIComponent(name)}/${action}`),
}

export const ProcessApi = {
  list: (deviceId: string) => api.get<ProcessInfo[]>(`/devices/${deviceId}/processes`),
  terminate: (deviceId: string, pid: number, startTimeUnixMs: number) =>
    api.post(`/devices/${deviceId}/processes/${pid}/terminate`, { start_time_unix_ms: startTimeUnixMs }),
}

// --- Files -------------------------------------------------------------------

export interface FileEntry {
  name: string
  is_dir: boolean
  size: number
  mod_time_unix_ms: number
}

export interface LogEntry {
  time: string
  level: string
  source: string
  message: string
}

export const LogApi = {
  query: (deviceId: string, params: { query?: string; level?: string; limit?: number }) => {
    const qs = new URLSearchParams()
    if (params.query) qs.set('query', params.query)
    if (params.level) qs.set('level', params.level)
    if (params.limit) qs.set('limit', String(params.limit))
    const suffix = qs.toString()
    return api.get<LogEntry[]>(`/devices/${deviceId}/logs` + (suffix ? '?' + suffix : ''))
  },
}

export const FileApi = {
  list: (deviceId: string, path: string) => api.get<FileEntry[]>(`/devices/${deviceId}/files?path=${encodeURIComponent(path)}`),
  mkdir: (deviceId: string, path: string) => api.post(`/devices/${deviceId}/files/mkdir`, { path }),
  rename: (deviceId: string, from: string, to: string) => api.post(`/devices/${deviceId}/files/rename`, { from, to }),
  delete: (deviceId: string, path: string) => api.del(`/devices/${deviceId}/files?path=${encodeURIComponent(path)}`),
  downloadUrl: (deviceId: string, path: string) => `${BASE}/devices/${deviceId}/files/download?path=${encodeURIComponent(path)}`,
  async upload(deviceId: string, path: string, file: File): Promise<{ state: string; bytes: number }> {
    const csrf = readCsrfCookie()
    const headers: Record<string, string> = {}
    if (csrf) headers['X-CSRF-Token'] = csrf
    const resp = await fetch(`${BASE}/devices/${deviceId}/files/upload?path=${encodeURIComponent(path)}`, {
      method: 'POST',
      headers,
      credentials: 'include',
      body: file,
    })
    const data = await resp.json().catch(() => null)
    if (!resp.ok) {
      const err = data?.error
      throw new ApiError(resp.status, err?.code ?? 'unknown', err?.message ?? resp.statusText)
    }
    return data?.data
  },
}
