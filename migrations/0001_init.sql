CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'admin',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS system_settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS instances (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    provider_kind TEXT NOT NULL,
    base_url TEXT NOT NULL DEFAULT '',
    group_name TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    scan_interval_seconds INTEGER NOT NULL DEFAULT 60,
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    credential_type TEXT NOT NULL DEFAULT 'none',
    credential_ciphertext TEXT NOT NULL DEFAULT '',
    credential_fingerprint TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_instances_provider_kind ON instances(provider_kind);
CREATE INDEX IF NOT EXISTS idx_instances_enabled ON instances(enabled);

CREATE TABLE IF NOT EXISTS monitor_targets (
    id TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    provider_kind TEXT NOT NULL,
    kind TEXT NOT NULL,
    name TEXT NOT NULL,
    external_id TEXT NOT NULL DEFAULT '',
    group_name TEXT NOT NULL DEFAULT '',
    key_fingerprint TEXT NOT NULL DEFAULT '',
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'unknown',
    balance_amount DOUBLE PRECISION,
    balance_currency TEXT,
    quota_used DOUBLE PRECISION,
    quota_total DOUBLE PRECISION,
    quota_remaining DOUBLE PRECISION,
    quota_unit TEXT,
    plan_name TEXT,
    plan_renew_at TIMESTAMPTZ,
    plan_expire_at TIMESTAMPTZ,
    monthly_cost_amount DOUBLE PRECISION,
    monthly_cost_currency TEXT,
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_scan_at TIMESTAMPTZ,
    next_scan_at TIMESTAMPTZ,
    risk_score INTEGER NOT NULL DEFAULT 0,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(instance_id, kind, external_id)
);

CREATE INDEX IF NOT EXISTS idx_monitor_targets_instance ON monitor_targets(instance_id);
CREATE INDEX IF NOT EXISTS idx_monitor_targets_status ON monitor_targets(status);
CREATE INDEX IF NOT EXISTS idx_monitor_targets_next_scan ON monitor_targets(next_scan_at) WHERE enabled = TRUE;

CREATE TABLE IF NOT EXISTS balance_snapshots (
    id TEXT PRIMARY KEY,
    target_id TEXT NOT NULL REFERENCES monitor_targets(id) ON DELETE CASCADE,
    captured_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    status TEXT NOT NULL,
    balance_amount DOUBLE PRECISION,
    balance_currency TEXT,
    quota_used DOUBLE PRECISION,
    quota_total DOUBLE PRECISION,
    quota_remaining DOUBLE PRECISION,
    quota_unit TEXT,
    monthly_cost_amount DOUBLE PRECISION,
    monthly_cost_currency TEXT,
    raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_balance_snapshots_target_time ON balance_snapshots(target_id, captured_at DESC);

CREATE TABLE IF NOT EXISTS alert_rules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    scope_type TEXT NOT NULL DEFAULT 'global',
    scope_value TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT 'warning',
    condition_type TEXT NOT NULL,
    threshold_value DOUBLE PRECISION NOT NULL,
    threshold_unit TEXT NOT NULL DEFAULT '',
    sustain_count INTEGER NOT NULL DEFAULT 1,
    cooldown_seconds INTEGER NOT NULL DEFAULT 1800,
    notification_channel_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules(enabled);

CREATE TABLE IF NOT EXISTS alert_events (
    id TEXT PRIMARY KEY,
    target_id TEXT REFERENCES monitor_targets(id) ON DELETE SET NULL,
    rule_id TEXT REFERENCES alert_rules(id) ON DELETE SET NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    opened_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    acknowledged_at TIMESTAMPTZ,
    silence_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_alert_events_status ON alert_events(status);
CREATE INDEX IF NOT EXISTS idx_alert_events_target ON alert_events(target_id);

CREATE TABLE IF NOT EXISTS notification_channels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    secret_ciphertext TEXT NOT NULL DEFAULT '',
    secret_fingerprint TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS alert_deliveries (
    id TEXT PRIMARY KEY,
    alert_id TEXT NOT NULL REFERENCES alert_events(id) ON DELETE CASCADE,
    channel_id TEXT REFERENCES notification_channels(id) ON DELETE SET NULL,
    status TEXT NOT NULL,
    response TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS scan_runs (
    id TEXT PRIMARY KEY,
    target_id TEXT REFERENCES monitor_targets(id) ON DELETE SET NULL,
    instance_id TEXT REFERENCES instances(id) ON DELETE SET NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    error TEXT NOT NULL DEFAULT '',
    raw JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_scan_runs_target_time ON scan_runs(target_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_scan_runs_status ON scan_runs(status);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
