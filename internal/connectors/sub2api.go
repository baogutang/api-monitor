package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"api-monitor/internal/domain"
)

type sub2APIUserConnector struct {
	client *http.Client
}

var sub2APIKeyListPaths = []string{
	"/api/v1/keys?page=1&page_size=100",
	"/api/v1/keys",
	"/api/v1/api-keys?page=1&page_size=100",
	"/api/v1/api-keys",
	"/api/v1/user/api-keys",
	"/api/v1/user/keys",
	"/api/keys",
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
	usageSummary := sub2APIUserUsageSummary(ctx, c.client, root, headers)
	rawWithWindows := mergeRaw(raw, map[string]any{"usageWindows": windows, "usageSummary": usageSummary, "source": "sub2api_user"})
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
		MonthlyCost:  firstMoney(usageSummaryCost(usageSummary, "30d"), inferMonthlyCost(user)),
		Raw:          rawWithWindows,
		Enabled:      true,
	}}
	if keysRaw, _, keyErr := requestFirstJSON(ctx, c.client, http.MethodGet, root, sub2APIKeyListPaths, headers, nil); keyErr == nil {
		keyItems := sub2APIKeyItems(keysRaw)
		keyIDs := make([]string, 0, len(keyItems))
		for _, item := range keyItems {
			obj := objectFromAny(item)
			if id := sub2APIKeyID(obj); id != "" {
				keyIDs = append(keyIDs, id)
			}
		}
		batchUsage := sub2APIBatchUsageByID(sub2APIBatchAPIKeyUsageRaw(ctx, c.client, root, headers, keyIDs))
		for _, item := range keyItems {
			obj := objectFromAny(item)
			key := stringFromJSON(obj, "key", "api_key", "apiKey", "token", "value")
			name := firstNonEmpty(stringFromJSON(obj, "name", "label", "title", "remark", "description"), "API Key")
			groupName, _ := sub2APIKeyGroupInfo(obj)
			id := firstNonEmpty(sub2APIKeyID(obj), keyFingerprint(key), name)
			dailyRaw := sub2APIKeyDailyUsageRaw(ctx, c.client, root, headers, id)
			keyRaw := sub2APIKeyRaw(obj, dailyRaw, batchUsage[id])
			keyUsageSummary := objectFromAny(rawObject(keyRaw)["usageSummary"])
			targets = append(targets, domain.MonitorTarget{
				InstanceID:     instance.ID,
				ProviderKind:   instance.ProviderKind,
				Kind:           domain.TargetAPIKey,
				Name:           name,
				ExternalID:     id,
				GroupName:      firstNonEmpty(groupName, instance.GroupName),
				KeyFingerprint: keyFingerprint(key),
				Capabilities:   capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
				Status:         domain.StatusUnknown,
				Quota:          sub2APIKeyQuota(obj),
				Plan:           parsePlan(obj),
				MonthlyCost:    usageSummaryCost(keyUsageSummary, "30d"),
				Raw:            keyRaw,
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
		raw, _, err := requestFirstJSON(ctx, c.client, http.MethodGet, root, sub2APIKeyListPaths, headers, nil)
		if err != nil {
			return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
		}
		obj := matchSub2APIKeyTarget(sub2APIKeyItems(raw), target)
		if len(obj) == 0 {
			err := errors.New("synchronized Sub2Api key was not found; sync monitored assets again")
			return &domain.ScanResult{Status: domain.StatusWarning, Error: err.Error(), Raw: raw}, err
		}
		keyID := sub2APIKeyID(obj)
		dailyRaw := sub2APIKeyDailyUsageRaw(ctx, c.client, root, headers, keyID)
		batchUsage := sub2APIBatchUsageByID(sub2APIBatchAPIKeyUsageRaw(ctx, c.client, root, headers, []string{keyID}))
		keyRaw := sub2APIKeyRaw(obj, dailyRaw, batchUsage[keyID])
		keyUsageSummary := objectFromAny(rawObject(keyRaw)["usageSummary"])
		return &domain.ScanResult{
			Status:       domain.StatusHealthy,
			Quota:        sub2APIKeyQuota(obj),
			Plan:         parsePlan(obj),
			MonthlyCost:  usageSummaryCost(keyUsageSummary, "30d"),
			Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth),
			Raw:          keyRaw,
		}, nil
	}

	if target.Kind == domain.TargetSubscription {
		raw, _, err := requestJSON(ctx, c.client, http.MethodGet, joinURL(root, "/api/v1/subscriptions/progress"), headers, nil)
		if err != nil {
			return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
		}
		obj := objectFromAny(unwrapData(raw))
		return &domain.ScanResult{
			Status:       domain.StatusHealthy,
			Balance:      inferBalance(obj),
			Quota:        inferQuota(obj),
			Plan:         parsePlan(obj),
			MonthlyCost:  inferMonthlyCost(obj),
			Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth, domain.CapabilityManualPlan),
			Raw:          makeRaw(obj),
		}, nil
	}
	raw, _, err := requestFirstJSON(ctx, c.client, http.MethodGet, root, []string{"/api/v1/auth/me", "/api/v1/users/me", "/api/v1/user/profile"}, headers, nil)
	if err != nil {
		return &domain.ScanResult{Status: flexibleStatus(err), Error: err.Error(), Raw: raw}, err
	}
	obj := objectFromAny(unwrapData(raw))
	quotaRaw, _, _ := requestJSON(ctx, c.client, http.MethodGet, joinURL(root, "/api/v1/user/platform-quotas"), headers, nil)
	windows := usageWindowsFromSub2Quota(quotaRaw)
	usageSummary := sub2APIUserUsageSummary(ctx, c.client, root, headers)
	return &domain.ScanResult{
		Status:       domain.StatusHealthy,
		Balance:      inferBalance(obj),
		Quota:        firstQuota(inferQuota(obj), quotaFromUsageWindows(windows)),
		Plan:         parsePlan(obj),
		MonthlyCost:  firstMoney(usageSummaryCost(usageSummary, "30d"), inferMonthlyCost(obj)),
		Capabilities: capabilities(domain.CapabilityUsage, domain.CapabilityHealth, domain.CapabilityManualPlan, domain.CapabilityWindowQuota),
		Raw:          mergeRaw(raw, map[string]any{"usageWindows": windows, "usageSummary": usageSummary, "source": "sub2api_user"}),
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

func sub2APIKeyGroupInfo(obj map[string]any) (string, *float64) {
	group := objectFromAny(obj["group"])
	name := firstNonEmpty(
		stringFromJSON(obj, "group_name", "groupName", "group"),
		stringFromJSON(group, "name", "display_name", "displayName", "title", "slug"),
	)
	if name == "" {
		if id := stringFromJSON(obj, "group_id", "groupId"); id != "" && id != "0" {
			name = "Group " + id
		}
	}
	rate := floatFromJSON(obj, "group_rate", "groupRate", "rate_multiplier", "rateMultiplier", "rate", "ratio", "multiplier")
	if rate == nil {
		rate = floatFromJSON(group, "rate_multiplier", "rateMultiplier", "rate", "ratio", "multiplier")
	}
	return name, rate
}

func sub2APIKeyQuota(obj map[string]any) *domain.Quota {
	quota := inferQuota(obj)
	if quota == nil {
		return nil
	}
	if quota.Unit == "quota" {
		quota.Unit = "USD"
	}
	if quota.Total != nil && *quota.Total == 0 {
		quota.Total = nil
		quota.Remaining = nil
	}
	return quota
}

func sub2APIKeyRaw(obj map[string]any, dailyRaw json.RawMessage, batchUsage map[string]any) json.RawMessage {
	extras := map[string]any{"source": "sub2api_api_key"}
	groupName, groupRate := sub2APIKeyGroupInfo(obj)
	if groupName != "" {
		extras["groupName"] = groupName
		extras["group_name"] = groupName
	}
	if groupRate != nil {
		extras["groupRate"] = *groupRate
		extras["group_rate"] = *groupRate
	}
	if len(dailyRaw) > 0 {
		daily := unwrapData(dailyRaw)
		extras["dailyUsage"] = daily
		if today := sub2APITodayUsage(daily); len(today) > 0 {
			for key, value := range today {
				extras[key] = value
			}
		}
	}
	if len(batchUsage) > 0 {
		extras["batchUsage"] = batchUsage
		if value := firstFloatFromJSON(batchUsage, "today_actual_cost", "todayActualCost"); value != nil {
			extras["today_actual_cost"] = *value
		}
		if value := firstFloatFromJSON(batchUsage, "total_actual_cost", "totalActualCost"); value != nil {
			extras["total_actual_cost"] = *value
			extras["monthly_cost"] = *value
		}
	}
	extras["usageSummary"] = sub2APIKeyUsageSummary(dailyRaw, batchUsage)
	return mergeRaw(makeRaw(obj), extras)
}

func sub2APIKeyDailyUsageRaw(ctx context.Context, client *http.Client, root string, headers map[string]string, keyID string) json.RawMessage {
	if keyID == "" {
		return nil
	}
	path := "/api/v1/user/api-keys/" + url.PathEscape(keyID) + "/usage/daily?days=30"
	raw, _, err := requestJSON(ctx, client, http.MethodGet, joinURL(root, path), headers, nil)
	if err != nil {
		return nil
	}
	return raw
}

func sub2APITodayUsage(value any) map[string]any {
	root := objectFromAny(value)
	items := arrayFromAny(root["items"])
	if len(items) == 0 {
		items = arrayFromAny(value)
	}
	if len(items) == 0 {
		return nil
	}
	today := ""
	for _, item := range items {
		obj := objectFromAny(item)
		if date := stringFromJSON(obj, "date"); date > today {
			today = date
		}
	}
	if today == "" {
		return nil
	}
	var cost, actualCost, requests, tokens float64
	for _, item := range items {
		obj := objectFromAny(item)
		if stringFromJSON(obj, "date") != today {
			continue
		}
		if value := floatFromJSON(obj, "cost"); value != nil {
			cost += *value
		}
		if value := floatFromJSON(obj, "actual_cost", "actualCost"); value != nil {
			actualCost += *value
		}
		if value := floatFromJSON(obj, "requests"); value != nil {
			requests += *value
		}
		if value := floatFromJSON(obj, "total_tokens", "totalTokens", "tokens"); value != nil {
			tokens += *value
		}
	}
	return map[string]any{
		"today_date":        today,
		"today_cost":        cost,
		"today_actual_cost": actualCost,
		"today_requests":    requests,
		"today_tokens":      tokens,
	}
}

func sub2APIKeyObjectMatchesTarget(obj map[string]any, target domain.MonitorTarget) bool {
	id := sub2APIKeyID(obj)
	if id != "" && id == target.ExternalID {
		return true
	}
	key := stringFromJSON(obj, "key", "api_key", "apiKey", "token", "value")
	if key != "" && target.KeyFingerprint != "" && keyFingerprint(key) == target.KeyFingerprint {
		return true
	}
	name := stringFromJSON(obj, "name", "label", "title", "remark", "description")
	return name != "" && name == target.Name
}

func sub2APIKeyID(obj map[string]any) string {
	return stringFromJSON(obj, "id", "key_id", "keyId", "uuid")
}

func sub2APIKeyItems(raw json.RawMessage) []any {
	if items := arrayFromAny(unwrapData(raw)); len(items) > 0 {
		return items
	}
	root := rawObject(raw)
	if items := arrayFromObjectKeys(root, "items", "list", "data", "records", "api_keys", "apiKeys", "keys"); len(items) > 0 {
		return items
	}
	data := objectFromAny(root["data"])
	return arrayFromObjectKeys(data, "items", "list", "records", "api_keys", "apiKeys", "keys")
}

func arrayFromObjectKeys(object map[string]any, keys ...string) []any {
	for _, key := range keys {
		if items := arrayFromAny(object[key]); len(items) > 0 {
			return items
		}
		if arr, ok := object[key].([]any); ok {
			return arr
		}
	}
	return nil
}
