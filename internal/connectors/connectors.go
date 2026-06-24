package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appcrypto "api-monitor/internal/crypto"
	"api-monitor/internal/domain"
)

type Connector interface {
	Kind() domain.ProviderKind
	Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error)
	Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error)
	Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error)
}

type Registry struct {
	connectors map[domain.ProviderKind]Connector
}

func NewRegistry(client *http.Client) *Registry {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	all := []Connector{
		&newAPIUserConnector{client: client},
		&newAPITokenConnector{client: client},
		&sub2APIUserConnector{client: client},
		&sub2APITokenConnector{client: client},
		&openAIAccountConnector{client: client},
		&geminiAccountConnector{client: client},
		&anthropicAccountConnector{client: client},
		&openAIAdminConnector{client: client},
		&openAIKeyConnector{client: client},
		&anthropicKeyConnector{client: client},
		&manualSubscriptionConnector{},
		&genericHTTPConnector{client: client},
	}
	registry := &Registry{connectors: map[domain.ProviderKind]Connector{}}
	for _, connector := range all {
		registry.connectors[connector.Kind()] = connector
	}
	return registry
}

func (r *Registry) Get(kind domain.ProviderKind) (Connector, bool) {
	connector, ok := r.connectors[kind]
	return connector, ok
}

type httpConnector struct {
	client *http.Client
}

func requestJSON(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, body []byte) (json.RawMessage, int, error) {
	raw, status, _, err := requestJSONWithHeaders(ctx, client, method, endpoint, headers, body)
	return raw, status, err
}

func requestJSONWithHeaders(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, body []byte) (json.RawMessage, int, http.Header, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, nil, err
	}
	if len(body) > 0 && headers["Content-Type"] == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		if value != "" {
			req.Header.Set(key, value)
		}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, res.StatusCode, res.Header, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return json.RawMessage(data), res.StatusCode, res.Header, fmt.Errorf("upstream status %d: %s", res.StatusCode, string(data))
	}
	if len(data) == 0 {
		data = []byte(`{}`)
	}
	return json.RawMessage(data), res.StatusCode, res.Header, nil
}

func baseURL(instance domain.Instance, fallback string) string {
	value := strings.TrimRight(instance.BaseURL, "/")
	if value == "" {
		return fallback
	}
	return value
}

func bearerHeaders(instance domain.Instance) map[string]string {
	headers := map[string]string{}
	if instance.Credential == nil {
		return headers
	}
	token := instance.Credential.Value
	if token == "" {
		token = stringFromJSON(instance.Credential.JSON, "token")
	}
	if token != "" {
		headers["Authorization"] = "Bearer " + token
	}
	return headers
}

func apiKeyValue(instance domain.Instance) string {
	if instance.Credential == nil {
		return ""
	}
	if instance.Credential.Value != "" {
		return instance.Credential.Value
	}
	if instance.Credential.JSON != nil {
		for _, key := range []string{"api_key", "apiKey", "token", "key"} {
			if value := stringFromJSON(instance.Credential.JSON, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func rawObject(raw json.RawMessage) map[string]any {
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	return out
}

func unwrapData(raw json.RawMessage) any {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"data", "result"} {
			if object[key] != nil {
				return object[key]
			}
		}
	}
	return value
}

func objectFromAny(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	return map[string]any{}
}

func arrayFromAny(value any) []any {
	if arr, ok := value.([]any); ok {
		return arr
	}
	if object, ok := value.(map[string]any); ok {
		for _, key := range []string{"items", "list", "data"} {
			if arr, ok := object[key].([]any); ok {
				return arr
			}
		}
	}
	return nil
}

func stringFromJSON(object map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key]; ok {
			switch typed := value.(type) {
			case string:
				return typed
			case float64:
				return strconv.FormatFloat(typed, 'f', -1, 64)
			case int:
				return strconv.Itoa(typed)
			case bool:
				return strconv.FormatBool(typed)
			}
		}
	}
	return ""
}

func floatFromJSON(object map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		value, ok := object[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return &typed
		case int:
			value := float64(typed)
			return &value
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			if err == nil {
				return &parsed
			}
		}
	}
	return nil
}

func boolFromJSON(object map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := object[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case float64:
			return typed != 0
		case int:
			return typed != 0
		case string:
			text := strings.TrimSpace(typed)
			if parsed, err := strconv.ParseBool(text); err == nil {
				return parsed
			}
			return text == "1"
		}
	}
	return false
}

func flexibleStatus(err error) domain.HealthStatus {
	if err == nil {
		return domain.StatusHealthy
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "quota") || strings.Contains(text, "billing") || strings.Contains(text, "insufficient") || strings.Contains(text, "429") {
		return domain.StatusCritical
	}
	return domain.StatusWarning
}

func keyFingerprint(key string) string {
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "sk-") && len(key) > 10 {
		return key[:6] + "..." + key[len(key)-4:]
	}
	return appcrypto.Fingerprint(key)
}

func capabilities(values ...domain.Capability) []domain.Capability {
	return values
}

func makeRaw(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

func parsePlan(object map[string]any) *domain.PlanInfo {
	name := stringFromJSON(object, "plan", "plan_name", "subscription", "package", "package_name")
	renewAt := parseTime(stringFromJSON(object, "renew_at", "renewAt", "renewal_at", "next_billing_at"))
	expireAt := parseTime(stringFromJSON(object, "expire_at", "expireAt", "expires_at", "expiresAt", "expired_at", "expiredAt", "expired_time", "expiredTime", "end_at", "endAt"))
	if name == "" && renewAt == nil && expireAt == nil {
		return nil
	}
	return &domain.PlanInfo{Name: name, RenewAt: renewAt, ExpireAt: expireAt}
}

func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	formats := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"}
	for _, format := range formats {
		parsed, err := time.Parse(format, value)
		if err == nil {
			return &parsed
		}
	}
	if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
		if ts <= 0 {
			return nil
		}
		if ts > 1_000_000_000_000 {
			t := time.UnixMilli(ts)
			return &t
		}
		t := time.Unix(ts, 0)
		return &t
	}
	return nil
}

func inferBalance(object map[string]any) *domain.Money {
	amount := floatFromJSON(object,
		"balance",
		"credit",
		"amount",
		"available",
		"available_balance",
		"availableBalance",
		"current_balance",
		"currentBalance",
		"remaining_balance",
		"remainingBalance",
		"remain_balance",
		"remainBalance",
		"remaining_credit",
		"remainingCredit",
		"wallet_balance",
		"walletBalance",
	)
	if amount == nil {
		return nil
	}
	currency := stringFromJSON(object, "currency", "balance_currency")
	if currency == "" {
		currency = "USD"
	}
	return &domain.Money{Amount: *amount, Currency: currency}
}

func inferQuota(object map[string]any) *domain.Quota {
	used := floatFromJSON(object, "used_quota", "usedQuota", "quota_used", "quotaUsed", "used", "usage", "used_amount", "usedAmount")
	total := floatFromJSON(object, "quota", "total_quota", "totalQuota", "total", "limit", "quota_limit", "quotaLimit")
	remaining := floatFromJSON(object, "remaining_quota", "remainingQuota", "remaining", "remain_quota", "remainQuota", "available_quota", "availableQuota")
	if used == nil && total == nil && remaining == nil {
		return nil
	}
	unit := stringFromJSON(object, "quota_unit", "unit")
	if unit == "" {
		unit = "quota"
	}
	if remaining == nil && total != nil && used != nil {
		value := *total - *used
		remaining = &value
	}
	return &domain.Quota{Used: used, Total: total, Remaining: remaining, Unit: unit}
}

func inferMonthlyCost(object map[string]any) *domain.Money {
	amount := floatFromJSON(object,
		"quota_used",
		"quotaUsed",
		"monthly_cost",
		"monthlyCost",
		"month_cost",
		"used_amount",
		"usedAmount",
		"usage_cost",
		"usageCost",
		"total_cost",
		"totalCost",
		"actual_cost",
		"actualCost",
		"today_cost",
		"todayCost",
		"daily_cost",
		"dailyCost",
		"spent",
		"consumed",
		"cost",
	)
	if amount == nil {
		return nil
	}
	currency := stringFromJSON(object, "currency")
	if currency == "" {
		currency = "USD"
	}
	return &domain.Money{Amount: *amount, Currency: currency}
}

func joinURL(root, path string) string {
	return strings.TrimRight(root, "/") + "/" + strings.TrimLeft(path, "/")
}

func appendQuery(endpoint string, params map[string]string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	q := u.Query()
	for key, value := range params {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func errMissingCredential() error {
	return errors.New("missing credential")
}
