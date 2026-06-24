package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"api-monitor/internal/domain"
)

type manualSubscriptionConnector struct{}

func (c *manualSubscriptionConnector) Kind() domain.ProviderKind { return domain.ProviderManualSub }

func (c *manualSubscriptionConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	return &domain.ProbeResult{
		OK:           true,
		Status:       domain.StatusHealthy,
		Message:      "manual subscription is configured",
		Capabilities: capabilities(domain.CapabilityManualPlan, domain.CapabilityHealth),
		Raw:          instance.Settings,
	}, nil
}

func (c *manualSubscriptionConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	settings := rawObject(instance.Settings)
	return []domain.MonitorTarget{{
		InstanceID:   instance.ID,
		ProviderKind: instance.ProviderKind,
		Kind:         domain.TargetSubscription,
		Name:         firstNonEmpty(stringFromJSON(settings, "name", "planName", "plan"), instance.Name),
		ExternalID:   firstNonEmpty(stringFromJSON(settings, "externalId"), "manual"),
		GroupName:    instance.GroupName,
		Capabilities: capabilities(domain.CapabilityManualPlan, domain.CapabilityHealth),
		Status:       domain.StatusUnknown,
		Balance:      inferBalance(settings),
		Quota:        inferQuota(settings),
		Plan:         parsePlan(settings),
		MonthlyCost:  inferMonthlyCost(settings),
		Raw:          instance.Settings,
		Enabled:      true,
	}}, nil
}

func (c *manualSubscriptionConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	settings := rawObject(instance.Settings)
	return &domain.ScanResult{
		Status:       domain.StatusHealthy,
		Balance:      inferBalance(settings),
		Quota:        inferQuota(settings),
		Plan:         parsePlan(settings),
		MonthlyCost:  inferMonthlyCost(settings),
		Capabilities: capabilities(domain.CapabilityManualPlan, domain.CapabilityHealth),
		Raw:          instance.Settings,
	}, nil
}

type genericHTTPConnector struct {
	client *http.Client
}

func (c *genericHTTPConnector) Kind() domain.ProviderKind { return domain.ProviderGenericHTTP }

func (c *genericHTTPConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	raw, err := c.call(ctx, instance)
	return &domain.ProbeResult{OK: err == nil, Status: flexibleStatus(err), Message: messageFromErr(err, "generic endpoint is reachable"), Capabilities: capabilities(domain.CapabilityBalance, domain.CapabilityUsage, domain.CapabilityHealth), Raw: raw}, err
}

func (c *genericHTTPConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	return []domain.MonitorTarget{{
		InstanceID:   instance.ID,
		ProviderKind: instance.ProviderKind,
		Kind:         domain.TargetEndpoint,
		Name:         instance.Name,
		ExternalID:   "endpoint",
		GroupName:    instance.GroupName,
		Capabilities: capabilities(domain.CapabilityBalance, domain.CapabilityUsage, domain.CapabilityHealth),
		Status:       domain.StatusUnknown,
		Enabled:      true,
	}}, nil
}

func (c *genericHTTPConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	raw, err := c.call(ctx, instance)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	settings := rawObject(instance.Settings)
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	balance := genericMoney(obj, settings)
	quota := genericQuota(obj, settings)
	return &domain.ScanResult{
		Status:       domain.StatusHealthy,
		Balance:      balance,
		Quota:        quota,
		Plan:         parsePlan(obj),
		Capabilities: capabilities(domain.CapabilityBalance, domain.CapabilityUsage, domain.CapabilityHealth),
		Raw:          raw,
	}, nil
}

func (c *genericHTTPConnector) call(ctx context.Context, instance domain.Instance) (json.RawMessage, error) {
	settings := rawObject(instance.Settings)
	method := firstNonEmpty(stringFromJSON(settings, "method"), http.MethodGet)
	endpoint := firstNonEmpty(stringFromJSON(settings, "url"), instance.BaseURL)
	if endpoint == "" {
		return nil, errMissingCredential()
	}
	headers := map[string]string{}
	if instance.Credential != nil {
		for key, value := range instance.Credential.Headers {
			headers[key] = value
		}
		if instance.Credential.Value != "" {
			headers["Authorization"] = "Bearer " + instance.Credential.Value
		}
	}
	if rawHeaders, ok := settings["headers"].(map[string]any); ok {
		for key, value := range rawHeaders {
			headers[key] = stringFromJSON(map[string]any{"v": value}, "v")
		}
	}
	body := []byte(stringFromJSON(settings, "body"))
	raw, _, err := requestJSON(ctx, c.client, method, endpoint, headers, body)
	return raw, err
}

func genericMoney(obj, settings map[string]any) *domain.Money {
	path := stringFromJSON(settings, "amountPath", "balancePath")
	if path == "" {
		return inferBalance(obj)
	}
	value := valueAtPath(obj, path)
	amount := toFloat(value)
	if amount == nil {
		return nil
	}
	currency := "USD"
	if curPath := stringFromJSON(settings, "currencyPath"); curPath != "" {
		if cur := toString(valueAtPath(obj, curPath)); cur != "" {
			currency = cur
		}
	}
	return &domain.Money{Amount: *amount, Currency: currency}
}

func genericQuota(obj, settings map[string]any) *domain.Quota {
	used := toFloat(valueAtPath(obj, stringFromJSON(settings, "quotaUsedPath")))
	total := toFloat(valueAtPath(obj, stringFromJSON(settings, "quotaTotalPath")))
	remaining := toFloat(valueAtPath(obj, stringFromJSON(settings, "quotaRemainingPath")))
	if used == nil && total == nil && remaining == nil {
		return inferQuota(obj)
	}
	unit := firstNonEmpty(stringFromJSON(settings, "quotaUnit"), "quota")
	return &domain.Quota{Used: used, Total: total, Remaining: remaining, Unit: unit}
}

func valueAtPath(obj map[string]any, path string) any {
	if path == "" {
		return nil
	}
	var cur any = obj
	for _, part := range strings.Split(strings.TrimPrefix(path, "$."), ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

func toFloat(value any) *float64 {
	switch typed := value.(type) {
	case float64:
		return &typed
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return ""
	}
}
