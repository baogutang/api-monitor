package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"api-monitor/internal/domain"
)

type openAIAdminConnector struct {
	client *http.Client
}

func (c *openAIAdminConnector) Kind() domain.ProviderKind { return domain.ProviderOpenAIAdmin }

func (c *openAIAdminConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	raw, _, err := c.costs(ctx, instance)
	return &domain.ProbeResult{OK: err == nil, Status: flexibleStatus(err), Message: messageFromErr(err, "OpenAI Admin API is reachable"), Capabilities: capabilities(domain.CapabilityCost, domain.CapabilityUsage, domain.CapabilityHealth), Raw: raw}, err
}

func (c *openAIAdminConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return nil, errMissingCredential()
	}
	return []domain.MonitorTarget{{
		InstanceID:     instance.ID,
		ProviderKind:   instance.ProviderKind,
		Kind:           domain.TargetOrganization,
		Name:           instance.Name,
		ExternalID:     keyFingerprint(key),
		GroupName:      instance.GroupName,
		KeyFingerprint: keyFingerprint(key),
		Capabilities:   capabilities(domain.CapabilityCost, domain.CapabilityUsage, domain.CapabilityHealth),
		Status:         domain.StatusUnknown,
		Enabled:        true,
	}}, nil
}

func (c *openAIAdminConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	raw, _, err := c.costs(ctx, instance)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	obj := objectFromAny(unwrapData(raw))
	return &domain.ScanResult{
		Status:       domain.StatusHealthy,
		MonthlyCost:  inferOpenAICost(obj),
		Capabilities: capabilities(domain.CapabilityCost, domain.CapabilityUsage, domain.CapabilityHealth),
		Raw:          raw,
	}, nil
}

func (c *openAIAdminConnector) costs(ctx context.Context, instance domain.Instance) (jsonRaw json.RawMessage, status int, err error) {
	key := apiKeyValue(instance)
	if key == "" {
		return nil, 0, errMissingCredential()
	}
	root := baseURL(instance, "https://api.openai.com")
	start := time.Now().UTC().AddDate(0, 0, -30).Unix()
	endpoint := appendQuery(joinURL(root, "/v1/organization/costs"), map[string]string{"start_time": strconv.FormatInt(start, 10), "bucket_width": "1d"})
	return requestJSON(ctx, c.client, http.MethodGet, endpoint, map[string]string{"Authorization": "Bearer " + key}, nil)
}

type openAIKeyConnector struct {
	client *http.Client
}

func (c *openAIKeyConnector) Kind() domain.ProviderKind { return domain.ProviderOpenAIKey }

func (c *openAIKeyConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	raw, _, err := c.models(ctx, instance)
	return &domain.ProbeResult{OK: err == nil, Status: flexibleStatus(err), Message: messageFromErr(err, "OpenAI API key is usable"), Capabilities: capabilities(domain.CapabilityHealth), Raw: raw}, err
}

func (c *openAIKeyConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return nil, errMissingCredential()
	}
	targets := []domain.MonitorTarget{{
		InstanceID:     instance.ID,
		ProviderKind:   instance.ProviderKind,
		Kind:           domain.TargetAPIKey,
		Name:           instance.Name,
		ExternalID:     keyFingerprint(key),
		GroupName:      instance.GroupName,
		KeyFingerprint: keyFingerprint(key),
		Capabilities:   capabilities(domain.CapabilityHealth),
		Status:         domain.StatusUnknown,
		Enabled:        true,
	}}
	targets = append(targets, officialWatchTargets(instance, "openai", instance.ProviderKind)...)
	return targets, nil
}

func (c *openAIKeyConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	if isWatchTarget(target.Kind) {
		return scanOfficialWatch(ctx, c.client, instance, "openai", target)
	}
	raw, _, err := c.models(ctx, instance)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	return &domain.ScanResult{Status: domain.StatusHealthy, Capabilities: capabilities(domain.CapabilityHealth), Raw: raw}, nil
}

func (c *openAIKeyConnector) models(ctx context.Context, instance domain.Instance) (json.RawMessage, int, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return nil, 0, errMissingCredential()
	}
	return requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, "https://api.openai.com"), "/v1/models"), map[string]string{"Authorization": "Bearer " + key}, nil)
}

type anthropicKeyConnector struct {
	client *http.Client
}

func (c *anthropicKeyConnector) Kind() domain.ProviderKind { return domain.ProviderAnthropicKey }

func (c *anthropicKeyConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	raw, _, err := c.models(ctx, instance)
	return &domain.ProbeResult{OK: err == nil, Status: flexibleStatus(err), Message: messageFromErr(err, "Anthropic API key is usable"), Capabilities: capabilities(domain.CapabilityHealth), Raw: raw}, err
}

func (c *anthropicKeyConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return nil, errMissingCredential()
	}
	targets := []domain.MonitorTarget{{
		InstanceID:     instance.ID,
		ProviderKind:   instance.ProviderKind,
		Kind:           domain.TargetAPIKey,
		Name:           instance.Name,
		ExternalID:     keyFingerprint(key),
		GroupName:      instance.GroupName,
		KeyFingerprint: keyFingerprint(key),
		Capabilities:   capabilities(domain.CapabilityHealth),
		Status:         domain.StatusUnknown,
		Enabled:        true,
	}}
	targets = append(targets, officialWatchTargets(instance, "anthropic", instance.ProviderKind)...)
	return targets, nil
}

func (c *anthropicKeyConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	if isWatchTarget(target.Kind) {
		return scanOfficialWatch(ctx, c.client, instance, "anthropic", target)
	}
	raw, _, err := c.models(ctx, instance)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	return &domain.ScanResult{Status: domain.StatusHealthy, Capabilities: capabilities(domain.CapabilityHealth), Raw: raw}, nil
}

func (c *anthropicKeyConnector) models(ctx context.Context, instance domain.Instance) (json.RawMessage, int, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return nil, 0, errMissingCredential()
	}
	headers := map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"}
	return requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, "https://api.anthropic.com"), "/v1/models"), headers, nil)
}

func inferOpenAICost(object map[string]any) *domain.Money {
	total := 0.0
	for _, item := range arrayFromAny(object) {
		bucket := objectFromAny(item)
		for _, result := range arrayFromAny(bucket["results"]) {
			obj := objectFromAny(result)
			if amount := floatFromJSON(obj, "amount", "cost"); amount != nil {
				total += *amount
			}
		}
	}
	if total == 0 {
		if amount := floatFromJSON(object, "total", "amount", "cost"); amount != nil {
			total = *amount
		}
	}
	return &domain.Money{Amount: total, Currency: "USD"}
}
