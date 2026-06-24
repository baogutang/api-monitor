export type ProviderKind =
  | 'newapi_user'
  | 'newapi_token'
  | 'sub2api_user'
  | 'sub2api_token'
  | 'openai_account'
  | 'gemini_account'
  | 'anthropic_account'
  | 'openai_admin'
  | 'openai_key'
  | 'anthropic_key'
  | 'manual_subscription'
  | 'generic_http'

export type TargetKind =
  | 'user'
  | 'api_key'
  | 'subscription'
  | 'project'
  | 'organization'
  | 'endpoint'
  | 'announcement_feed'
  | 'news_feed'
  | 'deprecation_feed'
  | 'group_catalog'
  | 'model_catalog'
  | 'pricing_catalog'

export type HealthStatus = 'healthy' | 'warning' | 'critical' | 'unknown'

export type Capability =
  | 'balance'
  | 'usage'
  | 'cost'
  | 'health'
  | 'window_quota'
  | 'manual_plan'
  | 'announcement'
  | 'news'
  | 'deprecation'
  | 'group_catalog'
  | 'model_catalog'
  | 'pricing_catalog'
  | 'change_watch'

export type Money = { amount: number; currency: string }

export type Quota = {
  used?: number
  total?: number
  remaining?: number
  unit: string
}

export type UsageWindow = {
  key?: string
  label: string
  window?: string
  used?: number
  total?: number
  remaining?: number
  utilization?: number
  unit?: string
  resetAt?: string
  windowFrom?: string
  status?: HealthStatus
  source?: string
  warnPercent?: number
  criticalPercent?: number
}

export type PlanInfo = {
  name?: string
  renewAt?: string
  expireAt?: string
}

export type Instance = {
  id: string
  name: string
  providerKind: ProviderKind
  baseUrl?: string
  groupName?: string
  enabled: boolean
  scanIntervalSeconds: number
  capabilities: Capability[]
  credentialFingerprint?: string
  settings?: Record<string, unknown>
  createdAt: string
  updatedAt: string
}

export type MonitorTarget = {
  id: string
  instanceId: string
  providerKind: ProviderKind
  kind: TargetKind
  name: string
  externalId?: string
  groupName?: string
  keyFingerprint?: string
  capabilities: Capability[]
  status: HealthStatus
  balance?: Money
  quota?: Quota
  plan?: PlanInfo
  monthlyCost?: Money
  raw?: {
    usageWindows?: UsageWindow[]
    usage_windows?: UsageWindow[]
    watchKind?: string
    source?: string
    sourceUrl?: string
    fingerprint?: string
    summary?: string
    count?: number
    items?: Array<Record<string, unknown>>
    [key: string]: unknown
  }
  lastScanAt?: string
  nextScanAt?: string
  riskScore: number
  enabled: boolean
  createdAt?: string
  updatedAt?: string
}

export type DashboardSummary = {
  totalTargets: number
  healthyTargets: number
  warningTargets: number
  criticalTargets: number
  unknownTargets: number
  scanSuccessRate: number
  monthlyCost?: Money
  atRiskBalance?: Money
  alerts24h: number
}

export type TrendPoint = {
  capturedAt: string
  providerKind?: ProviderKind
  balance?: Money
  cost?: Money
}

export type BalanceSnapshot = {
  id: string
  targetId: string
  capturedAt: string
  status: HealthStatus
  balance?: Money
  quota?: Quota
  monthlyCost?: Money
  raw?: unknown
}

export type AlertRule = {
  id: string
  name: string
  scopeType: string
  scopeValue?: string
  severity: 'warning' | 'critical' | 'phone'
  conditionType: string
  thresholdValue: number
  thresholdUnit?: string
  sustainCount: number
  cooldownSeconds: number
  notificationChannelIds: string[]
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export type AlertEvent = {
  id: string
  targetId?: string
  ruleId?: string
  severity: 'info' | 'warning' | 'critical' | 'phone'
  status: 'open' | 'acknowledged' | 'silenced' | 'resolved'
  title: string
  message: string
  openedAt: string
  resolvedAt?: string
  acknowledgedAt?: string
  silenceUntil?: string
}

export type NotificationChannel = {
  id: string
  name: string
  type: string
  enabled: boolean
  settings?: Record<string, unknown>
  secretFingerprint?: string
  createdAt: string
  updatedAt: string
}

export type UpsertNotificationChannelRequest = {
  id?: string
  name: string
  type: string
  enabled: boolean
  settings?: Record<string, unknown>
  secretValue?: string
}

export type ScanRun = {
  id: string
  targetId?: string
  instanceId?: string
  status: string
  startedAt: string
  finishedAt?: string
  error?: string
  raw?: unknown
}

export type User = {
  id: string
  email: string
  name?: string
}

export type SystemSettings = {
  defaultScanIntervalSeconds?: number
  defaultLocale?: string
  retentionDays?: number
  [key: string]: unknown
}

export type VersionInfo = {
  version: string
  commit: string
  date: string
  repository: string
  latestVersion?: string
  latestUrl?: string
  updateAvailable: boolean
  selfUpdateEnabled: boolean
  checkedAt?: string
  error?: string
}

export type UpsertInstanceRequest = {
  name: string
  providerKind: ProviderKind
  baseUrl?: string
  groupName?: string
  enabled: boolean
  scanIntervalSeconds: number
  settings?: Record<string, unknown>
  credential?: {
    type: 'bearer' | 'api_key' | 'basic' | 'json' | 'oauth' | 'session' | 'cookie' | 'none'
    value?: string
    username?: string
    password?: string
    json?: Record<string, unknown>
  }
}

export type ProbeResult = {
  ok: boolean
  status: HealthStatus
  message: string
  capabilities: Capability[]
  raw?: unknown
}

export type Paginated<T> = {
  items: T[]
  total: number
  limit: number
  offset: number
}

export type APIError = {
  error: { code: string; message: string; details?: unknown }
}
