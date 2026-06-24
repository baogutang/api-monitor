# API Monitor Frontend Implementation Guide

This document is the frontend integration contract for Cursor. Build the
frontend inside this same repository, preferably under `web/`, while the Go
backend lives under `cmd/` and `internal/`.

The product is an operations console for monitoring upstream AI relay accounts,
official OpenAI/Anthropic API accounts, manual subscriptions, and generic HTTP
balance endpoints. It must feel like a mature monitoring SaaS dashboard, not a
marketing site.

## Product Rules

- First screen is the real operations dashboard. Do not build a landing page.
- All user-visible configuration must be editable from Settings pages, not only
  from `.env`.
- Treat upstream new-api and sub2api as normal user accounts. Do not expose any
  admin-only setup in the UI.
- Some providers cannot expose true balance. Show capability badges such as
  `balance`, `usage`, `health`, `manual_plan` instead of pretending everything
  has a balance.
- Never display secrets in full. Show fingerprints only.
- Use table-first workflows: filter, sort, inspect, scan, silence, acknowledge.

## Suggested Stack

- React + TypeScript + Vite
- TanStack Router or React Router
- TanStack Query for API state
- Tailwind CSS
- shadcn/ui components
- lucide-react icons
- Recharts or Tremor charts
- Zod for API schema validation

The backend serves only JSON APIs. Frontend dev can run on `localhost:5173` and
proxy `/api` to `localhost:8080`.

## Navigation

Routes:

- `/` Dashboard
- `/assets` Monitored assets
- `/assets/:id` Optional full detail route; drawer can be enough for MVP
- `/instances` Upstream instances
- `/alerts` Alert center
- `/rules` Alert rules
- `/notifications` Notification channels
- `/scans` Scan runs
- `/settings` System settings
- `/login` Login
- `/setup` First-run setup

## Dashboard

Primary widgets:

- Total monitored assets
- Critical assets
- Warning assets
- Scan success rate
- Month-to-date official API cost
- Balance at risk
- Last 24h alert count

Sections:

- Risk table: most urgent assets sorted by `riskScore`
- Trend chart: cost/balance trend by provider
- Recent alerts
- Scan failures
- Provider capability coverage

## Assets Page

This is the core screen.

Columns:

- Status
- Name
- Provider
- Kind
- Group
- Capability badges
- Balance
- Quota
- Plan
- Monthly cost
- Last scan
- Risk
- Actions

Filters:

- Provider
- Kind
- Status
- Group
- Capability
- Risk range
- Text search

Actions:

- Scan now
- View details
- Edit overrides
- Silence
- Disable monitoring

Asset detail tabs or drawer sections:

- Overview
- Balance snapshots
- Usage snapshots
- Alerts
- Raw scan JSON
- Configuration overrides

## Instances Page

Purpose: configure upstream accounts and provider credentials.

Instance create/edit fields:

- Name
- Provider kind
- Base URL when applicable
- Group
- Scan interval seconds
- Enabled
- Credential type
- Credential fields
- Capability mode
- Optional default currency
- Optional manual monthly budget
- Optional manual renewal day

Provider kinds:

- `newapi_user`: normal new-api user session/token
- `newapi_token`: single new-api API key
- `sub2api_user`: normal sub2api user token
- `sub2api_token`: single sub2api API key
- `openai_admin`: official OpenAI Admin API key
- `openai_key`: official OpenAI regular API key
- `anthropic_key`: official Anthropic API key
- `manual_subscription`: manually tracked ChatGPT/Claude subscription
- `generic_http`: custom JSON endpoint

Credential UX:

- Do not show stored secret values.
- Show `Configured` plus fingerprint after saved.
- Provide `Test connection` before save when possible.
- Provide `Discover assets` after save.

Generic HTTP fields:

- Method
- URL
- Headers key/value list
- Body
- JSONPath for amount
- JSONPath for currency
- JSONPath for used quota
- JSONPath for total quota
- JSONPath for renewal/expiry date
- Health status JSONPath

## Alert Rules Page

Rule builder fields:

- Name
- Scope: global, provider, group, instance, asset
- Severity: warning, critical, phone
- Condition type:
  - `balance_below`
  - `remaining_quota_below`
  - `remaining_percent_below`
  - `days_until_expiry_below`
  - `scan_failures_gte`
  - `health_not_healthy`
  - `monthly_cost_above`
  - `cost_spike_percent_above`
- Threshold value
- Sustain count
- Cooldown minutes
- Notification channels
- Enabled

Show a natural-language preview, for example:

`When balance is below 10 USD for 2 consecutive scans, send critical alert to Feishu Ops.`

## Notification Channels Page

Supported channel types:

- DingTalk robot
- Feishu bot
- WeCom robot
- Generic webhook
- Phone escalation webhook
- SMTP email
- SendGrid email
- Twilio SMS
- Aliyun SMS
- Tencent Cloud SMS

Fields:

- Name
- Type
- Enabled
- Encrypted `secretValue`; leave empty on update to keep the stored secret
- Per-channel connection fields in `settings`
- Editable message templates:
  - `titleTemplate`
  - `markdownTemplate`
  - `textTemplate`
  - `htmlTemplate`
- Field-level help popovers/previews
- Real-time rendered message preview
- Test payload button for saved channels

Create/update payload:

```ts
type UpsertNotificationChannelRequest = {
  name: string
  type:
    | "dingtalk"
    | "feishu"
    | "wecom"
    | "webhook"
    | "phone"
    | "email_smtp"
    | "sendgrid_email"
    | "twilio_sms"
    | "aliyun_sms"
    | "tencent_sms"
  enabled: boolean
  settings: {
    webhookUrl?: string
    authHeader?: string
    phoneProvider?: string
    phoneNumbers?: string[]
    callTemplate?: string
    region?: string
    retryCount?: number
    escalateAfterMinutes?: number
    smtpHost?: string
    smtpPort?: number
    smtpUsername?: string
    fromEmail?: string
    fromName?: string
    toEmails?: string[]
    smtpStartTLS?: boolean
    smtpUseTLS?: boolean
    smtpSkipVerify?: boolean
    accountSid?: string
    fromNumber?: string
    messagingServiceSid?: string
    toNumbers?: string[]
    accessKeyId?: string
    endpoint?: string
    action?: string
    version?: string
    signName?: string
    from?: string
    templateCode?: string
    templateParam?: string
    secretId?: string
    endpointHost?: string
    smsSdkAppId?: string
    templateId?: string
    templateParamSet?: string[]
    titleTemplate?: string
    markdownTemplate?: string
    textTemplate?: string
    htmlTemplate?: string
  }
  secretValue?: string
}
```

For existing channels, leave `secretValue` empty to keep the saved encrypted
secret. The backend does not return the secret, only `secretFingerprint`.

Phone channels are generic phone-provider webhooks. The backend sends the phone
numbers, provider, template, retry count, and escalation delay to the configured
webhook; the actual PSTN call is performed by the user's provider/proxy.

Email/SMS channels are direct adapters, not placeholders:

- `email_smtp`: SMTP AUTH with STARTTLS or implicit TLS.
- `sendgrid_email`: SendGrid Mail Send API with Bearer API key.
- `twilio_sms`: Twilio Messages API with Account SID + Auth Token.
- `aliyun_sms`: Aliyun signed query using HMAC-SHA1 common parameters.
- `tencent_sms`: Tencent Cloud SendSms using TC3-HMAC-SHA256.

## API Base

Default backend base:

```ts
const API_BASE = import.meta.env.VITE_API_BASE ?? "http://localhost:8080";
```

All application endpoints are under `/api/v1`.

Auth:

- `POST /api/v1/setup` for first admin creation.
- `POST /api/v1/auth/login` returns `{ token, user }`.
- Pass `Authorization: Bearer <token>`.
- Store token in memory plus localStorage for MVP.

## Shared Types

```ts
export type ProviderKind =
  | "newapi_user"
  | "newapi_token"
  | "sub2api_user"
  | "sub2api_token"
  | "openai_admin"
  | "openai_key"
  | "anthropic_key"
  | "manual_subscription"
  | "generic_http";

export type TargetKind =
  | "user"
  | "api_key"
  | "subscription"
  | "project"
  | "organization"
  | "endpoint";

export type HealthStatus = "healthy" | "warning" | "critical" | "unknown";

export type Capability =
  | "balance"
  | "usage"
  | "cost"
  | "health"
  | "manual_plan";

export type Money = {
  amount: number;
  currency: string;
};

export type Quota = {
  used?: number;
  total?: number;
  remaining?: number;
  unit: string;
};

export type PlanInfo = {
  name?: string;
  renewAt?: string;
  expireAt?: string;
};

export type Instance = {
  id: string;
  name: string;
  providerKind: ProviderKind;
  baseUrl?: string;
  groupName?: string;
  enabled: boolean;
  scanIntervalSeconds: number;
  capabilities: Capability[];
  credentialFingerprint?: string;
  createdAt: string;
  updatedAt: string;
};

export type MonitorTarget = {
  id: string;
  instanceId: string;
  providerKind: ProviderKind;
  kind: TargetKind;
  name: string;
  externalId?: string;
  groupName?: string;
  keyFingerprint?: string;
  capabilities: Capability[];
  status: HealthStatus;
  balance?: Money;
  quota?: Quota;
  plan?: PlanInfo;
  monthlyCost?: Money;
  lastScanAt?: string;
  nextScanAt?: string;
  riskScore: number;
  enabled: boolean;
};

export type DashboardSummary = {
  totalTargets: number;
  healthyTargets: number;
  warningTargets: number;
  criticalTargets: number;
  unknownTargets: number;
  scanSuccessRate: number;
  monthlyCost?: Money;
  atRiskBalance?: Money;
  alerts24h: number;
};

export type BalanceSnapshot = {
  id: string;
  targetId: string;
  capturedAt: string;
  status: HealthStatus;
  balance?: Money;
  quota?: Quota;
  monthlyCost?: Money;
};

export type AlertEvent = {
  id: string;
  targetId?: string;
  ruleId?: string;
  severity: "info" | "warning" | "critical" | "phone";
  status: "open" | "acknowledged" | "silenced" | "resolved";
  title: string;
  message: string;
  openedAt: string;
  resolvedAt?: string;
  acknowledgedAt?: string;
};
```

## Endpoints

Health/setup:

```http
GET  /healthz
GET  /api/v1/setup/status
POST /api/v1/setup
```

Auth:

```http
POST /api/v1/auth/login
GET  /api/v1/auth/me
```

Dashboard:

```http
GET /api/v1/dashboard/summary
GET /api/v1/dashboard/trends?range=24h|7d|30d
GET /api/v1/dashboard/risk-targets?limit=10
```

Instances:

```http
GET    /api/v1/instances
POST   /api/v1/instances
GET    /api/v1/instances/:id
PATCH  /api/v1/instances/:id
DELETE /api/v1/instances/:id
POST   /api/v1/instances/:id/test
POST   /api/v1/instances/:id/discover
```

Create/update payload:

```ts
export type UpsertInstanceRequest = {
  name: string;
  providerKind: ProviderKind;
  baseUrl?: string;
  groupName?: string;
  enabled: boolean;
  scanIntervalSeconds: number;
  settings?: Record<string, unknown>;
  credential?: {
    type: "bearer" | "api_key" | "basic" | "json" | "none";
    value?: string;
    username?: string;
    password?: string;
    json?: Record<string, unknown>;
  };
};
```

Targets:

```http
GET  /api/v1/targets?providerKind=&status=&groupName=&q=&limit=&offset=
GET  /api/v1/targets/:id
PATCH /api/v1/targets/:id
POST /api/v1/targets/:id/scan
GET  /api/v1/targets/:id/snapshots?range=24h|7d|30d
GET  /api/v1/targets/:id/alerts
```

Alert rules:

```http
GET    /api/v1/alert-rules
POST   /api/v1/alert-rules
PATCH  /api/v1/alert-rules/:id
DELETE /api/v1/alert-rules/:id
```

Alerts:

```http
GET  /api/v1/alerts?status=&severity=&limit=&offset=
POST /api/v1/alerts/:id/ack
POST /api/v1/alerts/:id/silence
POST /api/v1/alerts/:id/resolve
```

Notification channels:

```http
GET    /api/v1/notification-channels
POST   /api/v1/notification-channels
PATCH  /api/v1/notification-channels/:id
DELETE /api/v1/notification-channels/:id
POST   /api/v1/notification-channels/:id/test
```

Scan runs and audit:

```http
GET /api/v1/scan-runs?targetId=&status=&limit=&offset=
GET /api/v1/audit-logs?limit=&offset=
```

System settings:

```http
GET   /api/v1/settings
PATCH /api/v1/settings
```

## Error Shape

All errors use:

```ts
type APIError = {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
};
```

## Frontend Implementation Notes

- Create a single `apiClient` wrapper that injects bearer token and handles
  `401` globally.
- Use optimistic UI only for acknowledgement/silence actions.
- Use polling every 15 seconds on dashboard and alert center.
- Use manual refresh buttons on all critical tables.
- Show skeletons, empty states, and error retry affordances.
- All destructive actions need confirmation dialogs.
- Secret fields should use "replace secret" behavior instead of trying to edit
  the existing secret value.

## Local Development

Expected commands after the backend is ready:

```bash
docker compose up postgres redis
go run ./cmd/api-monitor migrate
go run ./cmd/api-monitor api
go run ./cmd/api-monitor worker
cd web && npm run dev
```

Frontend `.env.local`:

```bash
VITE_API_BASE=http://localhost:8080
```

## MVP Acceptance

- First-run setup works.
- Login works.
- Instance can be created with secret fields.
- Test connection returns capability/result details.
- Discover creates targets.
- Targets page shows status, capabilities, balance/quota/cost when available.
- Scan now updates a target.
- Alert rule can be created.
- Notification channel can be created and test-sent.
- Dashboard reflects scans and alerts.
