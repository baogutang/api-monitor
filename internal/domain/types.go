package domain

import (
	"encoding/json"
	"time"
)

type ProviderKind string

const (
	ProviderNewAPIUser       ProviderKind = "newapi_user"
	ProviderNewAPIToken      ProviderKind = "newapi_token"
	ProviderSub2APIUser      ProviderKind = "sub2api_user"
	ProviderSub2APIToken     ProviderKind = "sub2api_token"
	ProviderOpenAIAccount    ProviderKind = "openai_account"
	ProviderGeminiAccount    ProviderKind = "gemini_account"
	ProviderAnthropicAccount ProviderKind = "anthropic_account"
	ProviderOpenAIAdmin      ProviderKind = "openai_admin"
	ProviderOpenAIKey        ProviderKind = "openai_key"
	ProviderAnthropicKey     ProviderKind = "anthropic_key"
	ProviderManualSub        ProviderKind = "manual_subscription"
	ProviderGenericHTTP      ProviderKind = "generic_http"
)

type TargetKind string

const (
	TargetUser         TargetKind = "user"
	TargetAPIKey       TargetKind = "api_key"
	TargetSubscription TargetKind = "subscription"
	TargetProject      TargetKind = "project"
	TargetOrganization TargetKind = "organization"
	TargetEndpoint     TargetKind = "endpoint"
	TargetAnnouncement TargetKind = "announcement_feed"
	TargetNewsFeed     TargetKind = "news_feed"
	TargetDeprecation  TargetKind = "deprecation_feed"
	TargetGroupCatalog TargetKind = "group_catalog"
	TargetModelCatalog TargetKind = "model_catalog"
	TargetPricing      TargetKind = "pricing_catalog"
)

type Capability string

const (
	CapabilityBalance      Capability = "balance"
	CapabilityUsage        Capability = "usage"
	CapabilityCost         Capability = "cost"
	CapabilityHealth       Capability = "health"
	CapabilityWindowQuota  Capability = "window_quota"
	CapabilityManualPlan   Capability = "manual_plan"
	CapabilityAnnouncement Capability = "announcement"
	CapabilityNews         Capability = "news"
	CapabilityDeprecation  Capability = "deprecation"
	CapabilityGroupCatalog Capability = "group_catalog"
	CapabilityModelCatalog Capability = "model_catalog"
	CapabilityPricing      Capability = "pricing_catalog"
	CapabilityChangeWatch  Capability = "change_watch"
)

type HealthStatus string

const (
	StatusHealthy  HealthStatus = "healthy"
	StatusWarning  HealthStatus = "warning"
	StatusCritical HealthStatus = "critical"
	StatusUnknown  HealthStatus = "unknown"
)

type Money struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

type Quota struct {
	Used      *float64 `json:"used,omitempty"`
	Total     *float64 `json:"total,omitempty"`
	Remaining *float64 `json:"remaining,omitempty"`
	Unit      string   `json:"unit"`
}

type PlanInfo struct {
	Name     string     `json:"name,omitempty"`
	RenewAt  *time.Time `json:"renewAt,omitempty"`
	ExpireAt *time.Time `json:"expireAt,omitempty"`
}

type Credential struct {
	Type     string                 `json:"type"`
	Value    string                 `json:"value,omitempty"`
	Username string                 `json:"username,omitempty"`
	Password string                 `json:"password,omitempty"`
	JSON     map[string]any         `json:"json,omitempty"`
	Headers  map[string]string      `json:"headers,omitempty"`
	Extra    map[string]interface{} `json:"extra,omitempty"`
}

type Instance struct {
	ID                    string          `json:"id"`
	Name                  string          `json:"name"`
	ProviderKind          ProviderKind    `json:"providerKind"`
	BaseURL               string          `json:"baseUrl,omitempty"`
	GroupName             string          `json:"groupName,omitempty"`
	Enabled               bool            `json:"enabled"`
	ScanIntervalSeconds   int             `json:"scanIntervalSeconds"`
	Capabilities          []Capability    `json:"capabilities"`
	Settings              json.RawMessage `json:"settings,omitempty"`
	Credential            *Credential     `json:"-"`
	CredentialType        string          `json:"-"`
	CredentialFingerprint string          `json:"credentialFingerprint,omitempty"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}

type MonitorTarget struct {
	ID             string          `json:"id"`
	InstanceID     string          `json:"instanceId"`
	ProviderKind   ProviderKind    `json:"providerKind"`
	Kind           TargetKind      `json:"kind"`
	Name           string          `json:"name"`
	ExternalID     string          `json:"externalId,omitempty"`
	GroupName      string          `json:"groupName,omitempty"`
	KeyFingerprint string          `json:"keyFingerprint,omitempty"`
	Capabilities   []Capability    `json:"capabilities"`
	Status         HealthStatus    `json:"status"`
	Balance        *Money          `json:"balance,omitempty"`
	Quota          *Quota          `json:"quota,omitempty"`
	Plan           *PlanInfo       `json:"plan,omitempty"`
	MonthlyCost    *Money          `json:"monthlyCost,omitempty"`
	Raw            json.RawMessage `json:"raw,omitempty"`
	LastScanAt     *time.Time      `json:"lastScanAt,omitempty"`
	NextScanAt     *time.Time      `json:"nextScanAt,omitempty"`
	RiskScore      int             `json:"riskScore"`
	Enabled        bool            `json:"enabled"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type Snapshot struct {
	ID          string          `json:"id"`
	TargetID    string          `json:"targetId"`
	CapturedAt  time.Time       `json:"capturedAt"`
	Status      HealthStatus    `json:"status"`
	Balance     *Money          `json:"balance,omitempty"`
	Quota       *Quota          `json:"quota,omitempty"`
	MonthlyCost *Money          `json:"monthlyCost,omitempty"`
	Raw         json.RawMessage `json:"raw,omitempty"`
}

type ProbeResult struct {
	OK           bool            `json:"ok"`
	Status       HealthStatus    `json:"status"`
	Message      string          `json:"message"`
	Capabilities []Capability    `json:"capabilities"`
	Raw          json.RawMessage `json:"raw,omitempty"`
}

type ScanResult struct {
	Status       HealthStatus    `json:"status"`
	Balance      *Money          `json:"balance,omitempty"`
	Quota        *Quota          `json:"quota,omitempty"`
	Plan         *PlanInfo       `json:"plan,omitempty"`
	MonthlyCost  *Money          `json:"monthlyCost,omitempty"`
	Capabilities []Capability    `json:"capabilities,omitempty"`
	Raw          json.RawMessage `json:"raw,omitempty"`
	Error        string          `json:"error,omitempty"`
}

type AlertRule struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	ScopeType              string    `json:"scopeType"`
	ScopeValue             string    `json:"scopeValue,omitempty"`
	Severity               string    `json:"severity"`
	ConditionType          string    `json:"conditionType"`
	ThresholdValue         float64   `json:"thresholdValue"`
	ThresholdUnit          string    `json:"thresholdUnit,omitempty"`
	SustainCount           int       `json:"sustainCount"`
	CooldownSeconds        int       `json:"cooldownSeconds"`
	NotificationChannelIDs []string  `json:"notificationChannelIds"`
	Enabled                bool      `json:"enabled"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

type AlertEvent struct {
	ID             string     `json:"id"`
	TargetID       string     `json:"targetId,omitempty"`
	RuleID         string     `json:"ruleId,omitempty"`
	Severity       string     `json:"severity"`
	Status         string     `json:"status"`
	Title          string     `json:"title"`
	Message        string     `json:"message"`
	OpenedAt       time.Time  `json:"openedAt"`
	ResolvedAt     *time.Time `json:"resolvedAt,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt,omitempty"`
	SilenceUntil   *time.Time `json:"silenceUntil,omitempty"`
}

type NotificationChannel struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Type              string          `json:"type"`
	Enabled           bool            `json:"enabled"`
	Settings          json.RawMessage `json:"settings,omitempty"`
	SecretFingerprint string          `json:"secretFingerprint,omitempty"`
	SecretValue       string          `json:"-"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type ScanRun struct {
	ID         string          `json:"id"`
	TargetID   string          `json:"targetId,omitempty"`
	InstanceID string          `json:"instanceId,omitempty"`
	Status     string          `json:"status"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	Error      string          `json:"error,omitempty"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}
