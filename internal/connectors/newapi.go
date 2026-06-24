package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"api-monitor/internal/domain"
)

type newAPIUserConnector struct {
	client *http.Client
}

func (c *newAPIUserConnector) Kind() domain.ProviderKind { return domain.ProviderNewAPIUser }

func (c *newAPIUserConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	headers, loginRaw, err := newAPIUserHeaders(ctx, c.client, instance)
	if err != nil {
		return &domain.ProbeResult{OK: false, Status: flexibleStatus(err), Message: err.Error(), Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth), Raw: loginRaw}, err
	}
	raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/api/user/self"), headers, nil)
	return &domain.ProbeResult{
		OK:           err == nil,
		Status:       flexibleStatus(err),
		Message:      messageFromErr(err, "new-api user API is reachable"),
		Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
		Raw:          raw,
	}, err
}

func (c *newAPIUserConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	headers, _, err := newAPIUserHeaders(ctx, c.client, instance)
	if err != nil {
		return nil, err
	}
	raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/api/user/self"), headers, nil)
	if err != nil {
		return nil, err
	}
	user := objectFromAny(unwrapData(raw))
	targets := []domain.MonitorTarget{{
		InstanceID:   instance.ID,
		ProviderKind: instance.ProviderKind,
		Kind:         domain.TargetUser,
		Name:         firstNonEmpty(stringFromJSON(user, "username", "display_name", "name"), instance.Name),
		ExternalID:   firstNonEmpty(stringFromJSON(user, "id", "user_id", "username"), "self"),
		GroupName:    firstNonEmpty(stringFromJSON(user, "group"), instance.GroupName),
		Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
		Status:       domain.StatusUnknown,
		Balance:      newAPIBalance(user),
		Quota:        inferQuota(user),
		Plan:         parsePlan(user),
		MonthlyCost:  newAPIQuotaMoney(user, "used_quota", "usedQuota", "used"),
		Raw:          raw,
		Enabled:      true,
	}}
	tokenRaw, _, tokenErr := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/api/token/"), headers, nil)
	if tokenErr == nil {
		for _, item := range arrayFromAny(unwrapData(tokenRaw)) {
			obj := objectFromAny(item)
			name := firstNonEmpty(stringFromJSON(obj, "name", "key", "id"), "API Key")
			key := stringFromJSON(obj, "key", "token")
			targets = append(targets, domain.MonitorTarget{
				InstanceID:     instance.ID,
				ProviderKind:   instance.ProviderKind,
				Kind:           domain.TargetAPIKey,
				Name:           name,
				ExternalID:     firstNonEmpty(stringFromJSON(obj, "id", "key"), name),
				GroupName:      instance.GroupName,
				KeyFingerprint: keyFingerprint(key),
				Capabilities:   capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
				Status:         domain.StatusUnknown,
				Quota:          newAPITokenQuota(obj),
				Plan:           parsePlan(obj),
				MonthlyCost:    newAPIQuotaMoney(obj, "used_quota", "usedQuota", "used"),
				Raw:            makeRaw(obj),
				Enabled:        true,
			})
		}
	}
	return targets, nil
}

func (c *newAPIUserConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	if target.Kind == domain.TargetAPIKey {
		headers, _, authErr := newAPIUserHeaders(ctx, c.client, instance)
		if authErr != nil {
			return &domain.ScanResult{Status: flexibleStatus(authErr), Error: authErr.Error()}, authErr
		}
		raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/api/token/"), headers, nil)
		if err != nil {
			return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
		}
		obj := matchNewAPITokenTarget(arrayFromAny(unwrapData(raw)), target)
		if len(obj) == 0 {
			err := errMissingTargetToken()
			return &domain.ScanResult{Status: domain.StatusWarning, Error: err.Error(), Raw: raw}, err
		}
		return &domain.ScanResult{
			Status:       domain.StatusHealthy,
			Quota:        newAPITokenQuota(obj),
			Plan:         parsePlan(obj),
			MonthlyCost:  newAPIQuotaMoney(obj, "used_quota", "usedQuota", "used"),
			Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
			Raw:          mergeRaw(makeRaw(obj), map[string]any{"source": "newapi_api_key"}),
		}, nil
	}

	headers, _, authErr := newAPIUserHeaders(ctx, c.client, instance)
	if authErr != nil {
		return &domain.ScanResult{Status: flexibleStatus(authErr), Error: authErr.Error()}, authErr
	}
	raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/api/user/self"), headers, nil)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	obj := objectFromAny(unwrapData(raw))
	return &domain.ScanResult{
		Status:       domain.StatusHealthy,
		Balance:      newAPIBalance(obj),
		Quota:        inferQuota(obj),
		Plan:         parsePlan(obj),
		MonthlyCost:  newAPIQuotaMoney(obj, "used_quota", "usedQuota", "used"),
		Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
		Raw:          raw,
	}, nil
}

type newAPITokenConnector struct {
	client *http.Client
}

func (c *newAPITokenConnector) Kind() domain.ProviderKind { return domain.ProviderNewAPIToken }

func (c *newAPITokenConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return &domain.ProbeResult{OK: false, Status: domain.StatusCritical, Message: "missing API key"}, errMissingCredential()
	}
	headers := map[string]string{"Authorization": "Bearer " + key}
	raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/api/usage/token/"), headers, nil)
	return &domain.ProbeResult{OK: err == nil, Status: flexibleStatus(err), Message: messageFromErr(err, "new-api token is usable"), Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth), Raw: raw}, err
}

func (c *newAPITokenConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return nil, errMissingCredential()
	}
	return []domain.MonitorTarget{{
		InstanceID:     instance.ID,
		ProviderKind:   instance.ProviderKind,
		Kind:           domain.TargetAPIKey,
		Name:           instance.Name,
		ExternalID:     keyFingerprint(key),
		GroupName:      instance.GroupName,
		KeyFingerprint: keyFingerprint(key),
		Capabilities:   capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
		Status:         domain.StatusUnknown,
		Enabled:        true,
	}}, nil
}

func (c *newAPITokenConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	key := apiKeyValue(instance)
	headers := map[string]string{"Authorization": "Bearer " + key}
	raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/api/usage/token/"), headers, nil)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	obj := objectFromAny(unwrapData(raw))
	return &domain.ScanResult{Status: domain.StatusHealthy, Quota: inferQuota(obj), Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth), Raw: raw}, nil
}

func messageFromErr(err error, ok string) string {
	if err == nil {
		return ok
	}
	return err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func marshalRaw(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

func errMissingTargetToken() error {
	return errors.New("synchronized New API key was not found; sync monitored assets again")
}

func matchNewAPITokenTarget(items []any, target domain.MonitorTarget) map[string]any {
	for _, item := range items {
		obj := objectFromAny(item)
		if tokenObjectMatchesTarget(obj, target) {
			return obj
		}
	}
	return nil
}

func tokenObjectMatchesTarget(obj map[string]any, target domain.MonitorTarget) bool {
	id := stringFromJSON(obj, "id")
	if id != "" && id == target.ExternalID {
		return true
	}
	key := stringFromJSON(obj, "key", "token", "api_key", "apiKey")
	if key != "" && target.KeyFingerprint != "" && keyFingerprint(key) == target.KeyFingerprint {
		return true
	}
	name := stringFromJSON(obj, "name")
	return name != "" && name == target.Name
}

func newAPIBalance(object map[string]any) *domain.Money {
	if money := inferBalance(object); money != nil {
		return money
	}
	return newAPIQuotaMoney(object, "quota", "remaining_quota", "remain_quota")
}

func newAPIQuotaMoney(object map[string]any, keys ...string) *domain.Money {
	value := floatFromJSON(object, keys...)
	if value == nil {
		return nil
	}
	scale := 500000.0
	if configured := floatFromJSON(object, "quota_per_unit", "quotaPerUnit", "quota_unit_scale"); configured != nil && *configured > 0 {
		scale = *configured
	}
	currency := stringFromJSON(object, "currency", "balance_currency")
	if currency == "" {
		currency = "USD"
	}
	return &domain.Money{Amount: *value / scale, Currency: currency}
}

func newAPITokenQuota(object map[string]any) *domain.Quota {
	used := floatFromJSON(object, "used_quota", "usedQuota", "used", "usage")
	total := floatFromJSON(object, "quota", "total_quota", "total", "limit")
	remaining := floatFromJSON(object, "remain_quota", "remaining_quota", "remaining", "available_quota")
	if boolFromJSON(object, "unlimited_quota", "unlimitedQuota") {
		return &domain.Quota{Used: used, Unit: "quota"}
	}
	if used == nil && total == nil && remaining == nil {
		return nil
	}
	if remaining == nil && total != nil && used != nil {
		value := *total - *used
		remaining = &value
	}
	return &domain.Quota{Used: used, Total: total, Remaining: remaining, Unit: "quota"}
}
