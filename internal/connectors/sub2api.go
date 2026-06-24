package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"api-monitor/internal/domain"
)

type sub2APIUserConnector struct {
	client *http.Client
}

func (c *sub2APIUserConnector) Kind() domain.ProviderKind { return domain.ProviderSub2APIUser }

func (c *sub2APIUserConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	headers, loginRaw, err := sub2APIUserHeaders(ctx, c.client, instance)
	if err != nil {
		return &domain.ProbeResult{OK: false, Status: flexibleStatus(err), Message: err.Error(), Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth, domain.CapabilityManualPlan, domain.CapabilityWindowQuota), Raw: loginRaw}, err
	}
	raw, _, err := requestFirstJSON(ctx, c.client, http.MethodGet, baseURL(instance, ""), []string{"/api/v1/auth/me", "/api/v1/users/me", "/api/v1/user/profile"}, headers, nil)
	return &domain.ProbeResult{OK: err == nil, Status: flexibleStatus(err), Message: messageFromErr(err, "sub2Api user API is reachable"), Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth, domain.CapabilityManualPlan, domain.CapabilityWindowQuota), Raw: raw}, err
}

func (c *sub2APIUserConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	headers, _, err := sub2APIUserHeaders(ctx, c.client, instance)
	if err != nil {
		return nil, err
	}
	root := baseURL(instance, "")
	raw, _, err := requestFirstJSON(ctx, c.client, http.MethodGet, root, []string{"/api/v1/auth/me", "/api/v1/users/me", "/api/v1/user/profile"}, headers, nil)
	if err != nil {
		return nil, err
	}
	user := objectFromAny(unwrapData(raw))
	quotaRaw, _, _ := requestJSON(ctx, c.client, http.MethodGet, joinURL(root, "/api/v1/user/platform-quotas"), headers, nil)
	windows := usageWindowsFromSub2Quota(quotaRaw)
	rawWithWindows := mergeRaw(raw, map[string]any{"usageWindows": windows, "source": "sub2api_user"})
	targets := []domain.MonitorTarget{{
		InstanceID:   instance.ID,
		ProviderKind: instance.ProviderKind,
		Kind:         domain.TargetUser,
		Name:         firstNonEmpty(stringFromJSON(user, "username", "email", "name"), instance.Name),
		ExternalID:   firstNonEmpty(stringFromJSON(user, "id", "user_id", "email"), "self"),
		GroupName:    instance.GroupName,
		Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth, domain.CapabilityManualPlan, domain.CapabilityWindowQuota),
		Status:       domain.StatusUnknown,
		Balance:      inferBalance(user),
		Quota:        firstQuota(inferQuota(user), quotaFromUsageWindows(windows)),
		Plan:         parsePlan(user),
		MonthlyCost:  inferMonthlyCost(user),
		Raw:          rawWithWindows,
		Enabled:      true,
	}}
	if keysRaw, _, keyErr := requestFirstJSON(ctx, c.client, http.MethodGet, root, []string{"/api/v1/api-keys", "/api/v1/keys", "/api/v1/user/keys"}, headers, nil); keyErr == nil {
		for _, item := range arrayFromAny(unwrapData(keysRaw)) {
			obj := objectFromAny(item)
			key := stringFromJSON(obj, "key", "api_key", "token")
			name := firstNonEmpty(stringFromJSON(obj, "name", "label"), "API Key")
			targets = append(targets, domain.MonitorTarget{
				InstanceID:     instance.ID,
				ProviderKind:   instance.ProviderKind,
				Kind:           domain.TargetAPIKey,
				Name:           name,
				ExternalID:     firstNonEmpty(stringFromJSON(obj, "id"), keyFingerprint(key), name),
				GroupName:      instance.GroupName,
				KeyFingerprint: keyFingerprint(key),
				Capabilities:   capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
				Status:         domain.StatusUnknown,
				Quota:          inferQuota(obj),
				Plan:           parsePlan(obj),
				MonthlyCost:    inferMonthlyCost(obj),
				Raw:            makeRaw(obj),
				Enabled:        true,
			})
		}
	}
	if subsRaw, _, subErr := requestFirstJSON(ctx, c.client, http.MethodGet, root, []string{"/api/v1/subscriptions", "/api/v1/subscriptions/active", "/api/v1/user/subscriptions"}, headers, nil); subErr == nil {
		for _, item := range arrayFromAny(unwrapData(subsRaw)) {
			obj := objectFromAny(item)
			name := firstNonEmpty(stringFromJSON(obj, "name", "plan", "title"), "Subscription")
			targets = append(targets, domain.MonitorTarget{
				InstanceID:   instance.ID,
				ProviderKind: instance.ProviderKind,
				Kind:         domain.TargetSubscription,
				Name:         name,
				ExternalID:   firstNonEmpty(stringFromJSON(obj, "id"), name),
				GroupName:    instance.GroupName,
				Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth, domain.CapabilityManualPlan),
				Status:       domain.StatusUnknown,
				Balance:      inferBalance(obj),
				Quota:        inferQuota(obj),
				Plan:         parsePlan(obj),
				MonthlyCost:  inferMonthlyCost(obj),
				Raw:          makeRaw(obj),
				Enabled:      true,
			})
		}
	}
	targets = append(targets, sub2APIWatchTargets(instance)...)
	return targets, nil
}

func (c *sub2APIUserConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	headers, _, authErr := sub2APIUserHeaders(ctx, c.client, instance)
	if authErr != nil {
		return &domain.ScanResult{Status: flexibleStatus(authErr), Error: authErr.Error()}, authErr
	}
	if isWatchTarget(target.Kind) {
		return scanSub2APIWatch(ctx, c.client, instance, target, headers)
	}
	root := baseURL(instance, "")
	if target.Kind == domain.TargetAPIKey {
		raw, _, err := requestFirstJSON(ctx, c.client, http.MethodGet, root, []string{"/api/v1/api-keys", "/api/v1/keys", "/api/v1/user/keys"}, headers, nil)
		if err != nil {
			return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
		}
		obj := matchSub2APIKeyTarget(arrayFromAny(unwrapData(raw)), target)
		if len(obj) == 0 {
			err := errors.New("synchronized Sub2Api key was not found; sync monitored assets again")
			return &domain.ScanResult{Status: domain.StatusWarning, Error: err.Error(), Raw: raw}, err
		}
		return &domain.ScanResult{
			Status:       domain.StatusHealthy,
			Quota:        inferQuota(obj),
			Plan:         parsePlan(obj),
			MonthlyCost:  inferMonthlyCost(obj),
			Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
			Raw:          mergeRaw(makeRaw(obj), map[string]any{"source": "sub2api_api_key"}),
		}, nil
	}

	path := "/api/v1/auth/me"
	if target.Kind == domain.TargetSubscription {
		path = "/api/v1/subscriptions/progress"
	}
	raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(root, path), headers, nil)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	obj := objectFromAny(unwrapData(raw))
	quotaRaw, _, _ := requestJSON(ctx, c.client, http.MethodGet, joinURL(root, "/api/v1/user/platform-quotas"), headers, nil)
	windows := usageWindowsFromSub2Quota(quotaRaw)
	return &domain.ScanResult{
		Status:       domain.StatusHealthy,
		Balance:      inferBalance(obj),
		Quota:        firstQuota(inferQuota(obj), quotaFromUsageWindows(windows)),
		Plan:         parsePlan(obj),
		MonthlyCost:  inferMonthlyCost(obj),
		Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth, domain.CapabilityManualPlan, domain.CapabilityWindowQuota),
		Raw:          mergeRaw(raw, map[string]any{"usageWindows": windows, "source": "sub2api_user"}),
	}, nil
}

type sub2APITokenConnector struct {
	client *http.Client
}

func (c *sub2APITokenConnector) Kind() domain.ProviderKind { return domain.ProviderSub2APIToken }

func (c *sub2APITokenConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	key := apiKeyValue(instance)
	if key == "" {
		return &domain.ProbeResult{OK: false, Status: domain.StatusCritical, Message: "missing API key"}, errMissingCredential()
	}
	raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/v1/models"), map[string]string{"Authorization": "Bearer " + key}, nil)
	return &domain.ProbeResult{OK: err == nil, Status: flexibleStatus(err), Message: messageFromErr(err, "sub2api token is usable"), Capabilities: capabilities(domain.CapabilityHealth), Raw: raw}, err
}

func (c *sub2APITokenConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
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
		Capabilities:   capabilities(domain.CapabilityHealth),
		Status:         domain.StatusUnknown,
		Enabled:        true,
	}}, nil
}

func (c *sub2APITokenConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	key := apiKeyValue(instance)
	raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(baseURL(instance, ""), "/v1/models"), map[string]string{"Authorization": "Bearer " + key}, nil)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	return &domain.ScanResult{Status: domain.StatusHealthy, Capabilities: capabilities(domain.CapabilityHealth), Raw: raw}, nil
}

func usageWindowsFromSub2Quota(raw json.RawMessage) []map[string]any {
	if len(raw) == 0 {
		return nil
	}
	root := objectFromAny(unwrapData(raw))
	items := arrayFromAny(root["platform_quotas"])
	if len(items) == 0 {
		items = arrayFromAny(root["platformQuotas"])
	}
	if len(items) == 0 {
		items = arrayFromAny(unwrapData(raw))
	}
	var windows []map[string]any
	for _, item := range items {
		obj := objectFromAny(item)
		platform := firstNonEmpty(stringFromJSON(obj, "platform", "provider"), "platform")
		windows = append(windows, quotaWindowFromObject(obj, platform+"_daily", platform+" daily", "daily_usage_usd", "daily_usage", "daily_limit_usd", "daily_limit", "daily_reset_at", "daily_window_start"))
		windows = append(windows, quotaWindowFromObject(obj, platform+"_weekly", platform+" weekly", "weekly_usage_usd", "weekly_usage", "weekly_limit_usd", "weekly_limit", "weekly_reset_at", "weekly_window_start"))
		windows = append(windows, quotaWindowFromObject(obj, platform+"_monthly", platform+" monthly", "monthly_usage_usd", "monthly_usage", "monthly_limit_usd", "monthly_limit", "monthly_reset_at", "monthly_window_start"))
	}
	return compactWindows(windows)
}

func matchSub2APIKeyTarget(items []any, target domain.MonitorTarget) map[string]any {
	for _, item := range items {
		obj := objectFromAny(item)
		if sub2APIKeyObjectMatchesTarget(obj, target) {
			return obj
		}
	}
	return nil
}

func sub2APIKeyObjectMatchesTarget(obj map[string]any, target domain.MonitorTarget) bool {
	id := stringFromJSON(obj, "id")
	if id != "" && id == target.ExternalID {
		return true
	}
	key := stringFromJSON(obj, "key", "api_key", "apiKey", "token")
	if key != "" && target.KeyFingerprint != "" && keyFingerprint(key) == target.KeyFingerprint {
		return true
	}
	name := stringFromJSON(obj, "name", "label")
	return name != "" && name == target.Name
}
