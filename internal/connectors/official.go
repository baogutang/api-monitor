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
	usageRaw, _, usageErr := c.usage(ctx, instance)
	combinedRaw := mergeRaw(raw, map[string]any{
		"source":       "openai_admin",
		"usage":        rawObject(usageRaw),
		"usageSummary": openAIAdminUsageSummary(raw, usageRaw),
	})
	if usageErr != nil {
		combinedRaw = mergeRaw(combinedRaw, map[string]any{"usageError": usageErr.Error()})
	}
	return &domain.ScanResult{
		Status:       domain.StatusHealthy,
		MonthlyCost:  inferOpenAICostRaw(raw),
		Capabilities: capabilities(domain.CapabilityCost, domain.CapabilityUsage, domain.CapabilityHealth),
		Raw:          combinedRaw,
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

func (c *openAIAdminConnector) usage(ctx context.Context, instance domain.Instance) (json.RawMessage, int, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return nil, 0, errMissingCredential()
	}
	root := baseURL(instance, "https://api.openai.com")
	start := time.Now().UTC().AddDate(0, 0, -30).Unix()
	endpoint := appendQuery(joinURL(root, "/v1/organization/usage/completions"), map[string]string{"start_time": strconv.FormatInt(start, 10), "bucket_width": "1d"})
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
	for _, item := range openAIResponseItems(object) {
		bucket := objectFromAny(item)
		for _, result := range arrayFromAny(bucket["results"]) {
			obj := objectFromAny(result)
			if amount := openAIMoneyAmount(obj["amount"]); amount != nil {
				total += *amount
			} else if amount := floatFromJSON(obj, "amount", "cost"); amount != nil {
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

func inferOpenAICostRaw(raw json.RawMessage) *domain.Money {
	return inferOpenAICost(rawObject(raw))
}

func openAIAdminUsageSummary(costRaw, usageRaw json.RawMessage) map[string]any {
	summary := newUsageSummary("openai_admin")
	costRanges := openAICostRanges(costRaw)
	for id, value := range costRanges {
		usageSummarySetRange(summary, id, value)
	}
	usageRanges := openAIUsageRanges(usageRaw)
	for id, usage := range usageRanges {
		rangeValue := objectFromAny(usageSummaryRanges(summary)[id])
		if len(rangeValue) == 0 {
			rangeValue = map[string]any{"currency": "USD"}
		}
		for key, value := range usage {
			rangeValue[key] = value
		}
		usageSummarySetRange(summary, id, rangeValue)
	}
	return summary
}

func openAICostRanges(raw json.RawMessage) map[string]map[string]any {
	out := map[string]map[string]any{
		"today": {"currency": "USD"},
		"24h":   {"currency": "USD"},
		"7d":    {"currency": "USD"},
		"30d":   {"currency": "USD"},
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, item := range openAIResponseItems(rawObject(raw)) {
		bucket := objectFromAny(item)
		start := openAIBucketStart(bucket)
		for _, result := range arrayFromAny(bucket["results"]) {
			obj := objectFromAny(result)
			amount := openAIMoneyAmount(obj["amount"])
			if amount == nil {
				amount = floatFromJSON(obj, "amount", "cost")
			}
			if amount == nil {
				continue
			}
			if !start.Before(today) {
				addToRange(out["today"], "cost", *amount)
				addToRange(out["today"], "actualCost", *amount)
			}
			if start.After(now.Add(-24 * time.Hour)) {
				addToRange(out["24h"], "cost", *amount)
				addToRange(out["24h"], "actualCost", *amount)
			}
			if start.After(now.Add(-7 * 24 * time.Hour)) {
				addToRange(out["7d"], "cost", *amount)
				addToRange(out["7d"], "actualCost", *amount)
			}
			addToRange(out["30d"], "cost", *amount)
			addToRange(out["30d"], "actualCost", *amount)
		}
	}
	for key, value := range out {
		if len(value) <= 1 {
			delete(out, key)
		}
	}
	return out
}

func openAIUsageRanges(raw json.RawMessage) map[string]map[string]any {
	out := map[string]map[string]any{
		"today": {"currency": "USD"},
		"24h":   {"currency": "USD"},
		"7d":    {"currency": "USD"},
		"30d":   {"currency": "USD"},
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for _, item := range openAIResponseItems(rawObject(raw)) {
		bucket := objectFromAny(item)
		start := openAIBucketStart(bucket)
		rangeTargets := []string{"30d"}
		if !start.Before(today) {
			rangeTargets = append(rangeTargets, "today")
		}
		if start.After(now.Add(-24 * time.Hour)) {
			rangeTargets = append(rangeTargets, "24h")
		}
		if start.After(now.Add(-7 * 24 * time.Hour)) {
			rangeTargets = append(rangeTargets, "7d")
		}
		for _, result := range arrayFromAny(bucket["results"]) {
			obj := objectFromAny(result)
			requests := firstFloatFromJSON(obj, "num_model_requests", "requests", "total_requests")
			input := firstFloatFromJSON(obj, "input_tokens", "inputTokens")
			output := firstFloatFromJSON(obj, "output_tokens", "outputTokens")
			for _, rangeID := range rangeTargets {
				if requests != nil {
					addToRange(out[rangeID], "requests", *requests)
				}
				if input != nil {
					addToRange(out[rangeID], "inputTokens", *input)
					addToRange(out[rangeID], "tokens", *input)
				}
				if output != nil {
					addToRange(out[rangeID], "outputTokens", *output)
					addToRange(out[rangeID], "tokens", *output)
				}
			}
		}
	}
	for key, value := range out {
		if len(value) <= 1 {
			delete(out, key)
		}
	}
	return out
}

func openAIResponseItems(object map[string]any) []any {
	if items := arrayFromAny(object["data"]); len(items) > 0 {
		return items
	}
	return arrayFromAny(object)
}

func openAIBucketStart(bucket map[string]any) time.Time {
	if value := firstFloatFromJSON(bucket, "start_time", "startTime"); value != nil {
		return time.Unix(int64(*value), 0).UTC()
	}
	return time.Now().UTC().Add(-30 * 24 * time.Hour)
}

func openAIMoneyAmount(value any) *float64 {
	if obj := objectFromAny(value); len(obj) > 0 {
		return firstFloatFromJSON(obj, "value", "amount", "cost")
	}
	if parsed, ok := numberFromAny(value); ok {
		return &parsed
	}
	return nil
}

func addToRange(out map[string]any, key string, value float64) {
	current, _ := numberFromAny(out[key])
	out[key] = current + value
}
