import { api } from '@/lib/api-client'
import type {
  AlertEvent,
  AlertRule,
  BalanceSnapshot,
  DashboardSummary,
  Instance,
  MonitorTarget,
  NotificationChannel,
  Paginated,
  ProbeResult,
  ScanRun,
  SystemSettings,
  TrendPoint,
  UpsertNotificationChannelRequest,
  UpsertInstanceRequest,
  User,
  VersionInfo,
} from '@/lib/types'

export const authApi = {
  setupStatus: () => api.get<{ needsSetup: boolean }>('/api/v1/setup/status'),
  setup: (body: { email: string; password: string; name?: string }) =>
    api.post<{ token: string; user: User }>('/api/v1/setup', body),
  login: (body: { email: string; password: string }) =>
    api.post<{ token: string; user: User }>('/api/v1/auth/login', body),
  me: () => api.get<User>('/api/v1/auth/me'),
}

export const dashboardApi = {
  summary: () => api.get<DashboardSummary>('/api/v1/dashboard/summary'),
  trends: (range = '7d') =>
    api.get<TrendPoint[]>(`/api/v1/dashboard/trends?range=${range}`),
  riskTargets: (limit = 10) =>
    api.get<MonitorTarget[]>(`/api/v1/dashboard/risk-targets?limit=${limit}`),
}

export type TargetFilters = {
  providerKind?: string
  status?: string
  groupName?: string
  q?: string
  limit?: number
  offset?: number
}

export const targetsApi = {
  list: (filters: TargetFilters = {}) => {
    const params = new URLSearchParams()
    Object.entries(filters).forEach(([k, v]) => {
      if (v != null && v !== '') params.set(k, String(v))
    })
    const qs = params.toString()
    return api.get<Paginated<MonitorTarget>>(`/api/v1/targets${qs ? `?${qs}` : ''}`)
  },
  get: (id: string) => api.get<MonitorTarget>(`/api/v1/targets/${id}`),
  patch: (id: string, body: Partial<MonitorTarget>) =>
    api.patch<MonitorTarget>(`/api/v1/targets/${id}`, body),
  scan: (id: string) => api.post<MonitorTarget>(`/api/v1/targets/${id}/scan`),
  snapshots: (id: string, range = '7d') =>
    api.get<BalanceSnapshot[]>(`/api/v1/targets/${id}/snapshots?range=${range}`),
  alerts: (id: string) => api.get<AlertEvent[]>(`/api/v1/targets/${id}/alerts`),
}

export const instancesApi = {
  list: () => api.get<Instance[]>('/api/v1/instances'),
  get: (id: string) => api.get<Instance>(`/api/v1/instances/${id}`),
  create: (body: UpsertInstanceRequest) => api.post<Instance>('/api/v1/instances', body),
  testDraft: (body: UpsertInstanceRequest) => api.post<ProbeResult>('/api/v1/instances/test-draft', body),
  patch: (id: string, body: Partial<UpsertInstanceRequest>) =>
    api.patch<Instance>(`/api/v1/instances/${id}`, body),
  delete: (id: string) => api.delete<void>(`/api/v1/instances/${id}`),
  test: (id: string) => api.post<ProbeResult>(`/api/v1/instances/${id}/test`),
  discover: (id: string) => api.post<{ created: number; items?: MonitorTarget[] }>(`/api/v1/instances/${id}/discover`),
}

export type AccountOAuthProvider = 'openai_account' | 'gemini_account' | 'anthropic_account'
export type AccountOAuthAuthorizeResponse = {
  authUrl: string
  sessionId: string
  state: string
  redirectUri: string
  expiresAt: string
}
export type AccountOAuthExchangeResponse = {
  credential: NonNullable<UpsertInstanceRequest['credential']>
  account: Record<string, unknown>
}

export const accountOAuthApi = {
  authorize: (
    provider: AccountOAuthProvider,
    body: { redirectUri?: string; oauthType?: string; projectId?: string } = {},
  ) => api.post<AccountOAuthAuthorizeResponse>(`/api/v1/account-oauth/${provider}/authorize`, body),
  exchange: (
    provider: AccountOAuthProvider,
    body: { sessionId: string; callbackUrl?: string; code?: string; state?: string },
  ) => api.post<AccountOAuthExchangeResponse>(`/api/v1/account-oauth/${provider}/exchange`, body),
}

export const alertsApi = {
  list: (params: { status?: string; severity?: string; limit?: number; offset?: number } = {}) => {
    const qs = new URLSearchParams()
    Object.entries(params).forEach(([k, v]) => {
      if (v != null) qs.set(k, String(v))
    })
    const q = qs.toString()
    return api.get<Paginated<AlertEvent>>(`/api/v1/alerts${q ? `?${q}` : ''}`)
  },
  ack: (id: string) => api.post<AlertEvent>(`/api/v1/alerts/${id}/ack`),
  silence: (id: string, body?: { until?: string }) =>
    api.post<AlertEvent>(`/api/v1/alerts/${id}/silence`, body),
  resolve: (id: string) => api.post<AlertEvent>(`/api/v1/alerts/${id}/resolve`),
}

export const rulesApi = {
  list: () => api.get<AlertRule[]>('/api/v1/alert-rules'),
  create: (body: Partial<AlertRule>) => api.post<AlertRule>('/api/v1/alert-rules', body),
  patch: (id: string, body: Partial<AlertRule>) =>
    api.patch<AlertRule>(`/api/v1/alert-rules/${id}`, body),
  delete: (id: string) => api.delete<void>(`/api/v1/alert-rules/${id}`),
}

export const channelsApi = {
  list: () => api.get<NotificationChannel[]>('/api/v1/notification-channels'),
  create: (body: UpsertNotificationChannelRequest) =>
    api.post<NotificationChannel>('/api/v1/notification-channels', body),
  patch: (id: string, body: UpsertNotificationChannelRequest) =>
    api.patch<NotificationChannel>(`/api/v1/notification-channels/${id}`, body),
  delete: (id: string) => api.delete<void>(`/api/v1/notification-channels/${id}`),
  test: (id: string) =>
    api.post<{ ok: boolean; message?: string; response?: string }>(`/api/v1/notification-channels/${id}/test`),
  testDraft: (body: UpsertNotificationChannelRequest) =>
    api.post<{ ok: boolean; message?: string; response?: string }>('/api/v1/notification-channels/test-draft', body),
}

export const scansApi = {
  list: (params: { targetId?: string; status?: string; limit?: number; offset?: number } = {}) => {
    const qs = new URLSearchParams()
    Object.entries(params).forEach(([k, v]) => {
      if (v != null) qs.set(k, String(v))
    })
    const q = qs.toString()
    return api.get<Paginated<ScanRun>>(`/api/v1/scan-runs${q ? `?${q}` : ''}`)
  },
}

export const settingsApi = {
  get: () => api.get<SystemSettings>('/api/v1/settings'),
  patch: (body: SystemSettings) => api.patch<SystemSettings>('/api/v1/settings', body),
}

export const versionApi = {
  current: () => api.get<VersionInfo>('/api/v1/version'),
  check: () => api.post<VersionInfo>('/api/v1/version/check'),
  update: () => api.post<{ ok: boolean; output: string }>('/api/v1/version/update'),
}
