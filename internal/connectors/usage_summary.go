package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"api-monitor/internal/domain"
)

const newAPIQuotaUnitScale = 500000.0

type usageRangeSpec struct {
	id    string
	start time.Time
	end   time.Time
}

func usageRangeSpecs(now time.Time) []usageRangeSpec {
	now = now.UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return []usageRangeSpec{
		{id: "today", start: today, end: now},
		{id: "24h", start: now.Add(-24 * time.Hour), end: now},
		{id: "7d", start: now.Add(-7 * 24 * time.Hour), end: now},
		{id: "30d", start: now.Add(-30 * 24 * time.Hour), end: now},
	}
}

func newUsageSummary(source string) map[string]any {
	return map[string]any{
		"source":   source,
		"currency": "USD",
		"ranges":   map[string]any{},
	}
}

func usageSummaryRanges(summary map[string]any) map[string]any {
	ranges, _ := summary["ranges"].(map[string]any)
	if ranges == nil {
		ranges = map[string]any{}
		summary["ranges"] = ranges
	}
	return ranges
}

func usageSummarySetRange(summary map[string]any, id string, value map[string]any) {
	if len(value) == 0 {
		return
	}
	value["range"] = id
	usageSummaryRanges(summary)[id] = value
}

func usageSummaryErrors(summary map[string]any) map[string]string {
	errorsMap, _ := summary["errors"].(map[string]string)
	if errorsMap == nil {
		errorsMap = map[string]string{}
		summary["errors"] = errorsMap
	}
	return errorsMap
}

func usageSummaryCost(summary map[string]any, rangeID string) *domain.Money {
	ranges := usageSummaryRanges(summary)
	obj := objectFromAny(ranges[rangeID])
	if len(obj) == 0 {
		return nil
	}
	value := firstFloatFromJSON(obj, "actualCost", "actual_cost", "cost", "totalActualCost", "total_actual_cost", "totalCost", "total_cost")
	if value == nil {
		return nil
	}
	currency := firstNonEmpty(stringFromJSON(obj, "currency"), stringFromJSON(summary, "currency"), "USD")
	return &domain.Money{Amount: *value, Currency: currency}
}

func usageSummaryRangeFromUsageObject(obj map[string]any, currency string) map[string]any {
	if len(obj) == 0 {
		return nil
	}
	out := map[string]any{"currency": firstNonEmpty(currency, "USD")}
	copyUsageNumber(out, obj, "cost", "cost", "total_cost", "totalCost")
	copyUsageNumber(out, obj, "actualCost", "actual_cost", "actualCost", "total_actual_cost", "totalActualCost")
	copyUsageNumber(out, obj, "accountCost", "total_account_cost", "totalAccountCost", "account_cost", "accountCost")
	copyUsageNumber(out, obj, "requests", "requests", "total_requests", "totalRequests", "request_count", "requestCount", "api_requests", "apiRequests")
	copyUsageNumber(out, obj, "tokens", "total_tokens", "totalTokens", "tokens")
	copyUsageNumber(out, obj, "inputTokens", "input_tokens", "inputTokens", "total_input_tokens", "totalInputTokens")
	copyUsageNumber(out, obj, "outputTokens", "output_tokens", "outputTokens", "total_output_tokens", "totalOutputTokens")
	copyUsageNumber(out, obj, "rpm", "rpm")
	copyUsageNumber(out, obj, "tpm", "tpm")
	if len(out) == 1 {
		return nil
	}
	return out
}

func copyUsageNumber(out map[string]any, obj map[string]any, outKey string, keys ...string) {
	if value := firstFloatFromJSON(obj, keys...); value != nil {
		out[outKey] = *value
	}
}

func firstFloatFromJSON(obj map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		if value, ok := numberFromAny(obj[key]); ok {
			return &value
		}
	}
	return nil
}

func newAPIUserUsageSummary(ctx context.Context, client *http.Client, root string, headers map[string]string) map[string]any {
	summary := newUsageSummary("newapi_user")
	now := time.Now().UTC()
	for _, spec := range usageRangeSpecs(now) {
		endpoint := appendQuery(joinURL(root, "/api/log/self/stat"), map[string]string{
			"type":            "2",
			"start_timestamp": strconv.FormatInt(spec.start.Unix(), 10),
			"end_timestamp":   strconv.FormatInt(spec.end.Unix(), 10),
		})
		raw, _, err := requestJSON(ctx, client, http.MethodGet, endpoint, headers, nil)
		if err != nil {
			usageSummaryErrors(summary)[spec.id] = err.Error()
			continue
		}
		stat := objectFromAny(unwrapData(raw))
		rangeValue := map[string]any{"currency": "USD"}
		if quota := firstFloatFromJSON(stat, "quota", "used_quota", "usedQuota"); quota != nil {
			rangeValue["quota"] = *quota
			rangeValue["cost"] = *quota / newAPIQuotaUnitScale
			rangeValue["actualCost"] = *quota / newAPIQuotaUnitScale
		}
		copyUsageNumber(rangeValue, stat, "rpm", "rpm")
		copyUsageNumber(rangeValue, stat, "tpm", "tpm")
		usageSummarySetRange(summary, spec.id, rangeValue)
	}

	start30 := now.Add(-30 * 24 * time.Hour)
	if raw, _, err := requestJSON(ctx, client, http.MethodGet, appendQuery(joinURL(root, "/api/log/self"), map[string]string{
		"p":               "1",
		"page_size":       "20",
		"type":            "2",
		"start_timestamp": strconv.FormatInt(start30.Unix(), 10),
		"end_timestamp":   strconv.FormatInt(now.Unix(), 10),
	}), headers, nil); err == nil {
		summary["recentLogs"] = rawObject(raw)
	} else {
		usageSummaryErrors(summary)["recentLogs"] = err.Error()
	}
	if raw, _, err := requestJSON(ctx, client, http.MethodGet, appendQuery(joinURL(root, "/api/data/self"), map[string]string{
		"start_timestamp": strconv.FormatInt(start30.Unix(), 10),
		"end_timestamp":   strconv.FormatInt(now.Unix(), 10),
	}), headers, nil); err == nil {
		summary["data"] = rawObject(raw)
	} else {
		usageSummaryErrors(summary)["data"] = err.Error()
	}
	if raw, _, err := requestJSON(ctx, client, http.MethodGet, appendQuery(joinURL(root, "/api/data/flow/self"), map[string]string{
		"start_timestamp": strconv.FormatInt(start30.Unix(), 10),
		"end_timestamp":   strconv.FormatInt(now.Unix(), 10),
	}), headers, nil); err == nil {
		summary["flow"] = rawObject(raw)
	} else {
		usageSummaryErrors(summary)["flow"] = err.Error()
	}
	return summary
}

func newAPITokenRaw(ctx context.Context, client *http.Client, root string, headers map[string]string, obj map[string]any) json.RawMessage {
	extras := map[string]any{"source": "newapi_api_key"}
	tokenID := stringFromJSON(obj, "id")
	if tokenID != "" {
		fullKeyRaw, _, err := requestJSON(ctx, client, http.MethodPost, joinURL(root, "/api/token/"+url.PathEscape(tokenID)+"/key"), headers, nil)
		if err == nil {
			fullKey := stringFromJSON(objectFromAny(unwrapData(fullKeyRaw)), "key", "token", "api_key", "apiKey")
			if fullKey != "" {
				extras["fullKeyFingerprint"] = keyFingerprint(fullKey)
				extras["fullKeyHash"] = shortHash(fullKey)
				usageRaw, _, usageErr := requestJSON(ctx, client, http.MethodGet, joinURL(root, "/api/usage/token/"), map[string]string{"Authorization": "Bearer " + fullKey}, nil)
				if usageErr == nil {
					usage := objectFromAny(unwrapData(usageRaw))
					extras["tokenUsage"] = usage
					if totalUsed := firstFloatFromJSON(usage, "total_used", "totalUsed"); totalUsed != nil {
						extras["total_used_quota"] = *totalUsed
					}
				} else {
					extras["tokenUsageError"] = usageErr.Error()
				}
			}
		} else {
			extras["fullKeyError"] = err.Error()
		}
	}
	extras["usageSummary"] = newAPITokenUsageSummary(ctx, client, root, headers, obj, objectFromAny(extras["tokenUsage"]))
	return mergeRaw(makeRaw(obj), extras)
}

func newAPITokenUsageSummary(ctx context.Context, client *http.Client, root string, headers map[string]string, token map[string]any, usage map[string]any) map[string]any {
	summary := newUsageSummary("newapi_token")
	tokenName := firstNonEmpty(stringFromJSON(token, "name", "token_name", "tokenName"), stringFromJSON(usage, "name"))
	now := time.Now().UTC()
	if tokenName != "" {
		for _, spec := range usageRangeSpecs(now) {
			endpoint := appendQuery(joinURL(root, "/api/log/self/stat"), map[string]string{
				"type":            "2",
				"token_name":      tokenName,
				"start_timestamp": strconv.FormatInt(spec.start.Unix(), 10),
				"end_timestamp":   strconv.FormatInt(spec.end.Unix(), 10),
			})
			raw, _, err := requestJSON(ctx, client, http.MethodGet, endpoint, headers, nil)
			if err != nil {
				usageSummaryErrors(summary)[spec.id] = err.Error()
				continue
			}
			stat := objectFromAny(unwrapData(raw))
			rangeValue := map[string]any{"currency": "USD"}
			if quota := firstFloatFromJSON(stat, "quota", "used_quota", "usedQuota"); quota != nil {
				rangeValue["quota"] = *quota
				rangeValue["cost"] = *quota / newAPIQuotaUnitScale
				rangeValue["actualCost"] = *quota / newAPIQuotaUnitScale
			}
			copyUsageNumber(rangeValue, stat, "rpm", "rpm")
			copyUsageNumber(rangeValue, stat, "tpm", "tpm")
			usageSummarySetRange(summary, spec.id, rangeValue)
		}
	}
	if used := firstFloatFromJSON(usage, "total_used", "totalUsed", "used_quota", "usedQuota"); used != nil {
		total := map[string]any{
			"currency":   "USD",
			"quota":      *used,
			"cost":       *used / newAPIQuotaUnitScale,
			"actualCost": *used / newAPIQuotaUnitScale,
		}
		usageSummarySetRange(summary, "total", total)
	}
	return summary
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func sub2APIUserUsageSummary(ctx context.Context, client *http.Client, root string, headers map[string]string) map[string]any {
	summary := newUsageSummary("sub2api_user")
	now := time.Now().UTC()
	if raw, _, err := requestJSON(ctx, client, http.MethodGet, joinURL(root, "/api/v1/usage/dashboard/stats"), headers, nil); err == nil {
		stats := objectFromAny(unwrapData(raw))
		summary["dashboard"] = stats
		today := map[string]any{"currency": "USD"}
		copyUsageNumber(today, stats, "cost", "today_cost", "todayCost")
		copyUsageNumber(today, stats, "actualCost", "today_actual_cost", "todayActualCost")
		copyUsageNumber(today, stats, "requests", "today_requests", "todayRequests")
		copyUsageNumber(today, stats, "tokens", "today_tokens", "todayTokens")
		copyUsageNumber(today, stats, "rpm", "rpm")
		copyUsageNumber(today, stats, "tpm", "tpm")
		usageSummarySetRange(summary, "today", today)
		total := map[string]any{"currency": "USD"}
		copyUsageNumber(total, stats, "cost", "total_cost", "totalCost")
		copyUsageNumber(total, stats, "actualCost", "total_actual_cost", "totalActualCost")
		copyUsageNumber(total, stats, "requests", "total_requests", "totalRequests")
		copyUsageNumber(total, stats, "tokens", "total_tokens", "totalTokens")
		usageSummarySetRange(summary, "total", total)
	} else {
		usageSummaryErrors(summary)["dashboard"] = err.Error()
	}
	for _, item := range []struct {
		rangeID string
		period  string
	}{
		{rangeID: "today", period: "today"},
		{rangeID: "7d", period: "week"},
		{rangeID: "30d", period: "month"},
	} {
		raw, _, err := requestJSON(ctx, client, http.MethodGet, appendQuery(joinURL(root, "/api/v1/usage/stats"), map[string]string{
			"period":   item.period,
			"timezone": "UTC",
		}), headers, nil)
		if err != nil {
			usageSummaryErrors(summary)["stats_"+item.rangeID] = err.Error()
			continue
		}
		if value := usageSummaryRangeFromUsageObject(objectFromAny(unwrapData(raw)), "USD"); value != nil {
			usageSummarySetRange(summary, item.rangeID, value)
		}
	}
	if ranges := usageSummaryRanges(summary); ranges["24h"] == nil {
		if today := objectFromAny(ranges["today"]); len(today) > 0 {
			ranges["24h"] = today
		}
	}
	start30 := now.Add(-30 * 24 * time.Hour)
	query := map[string]string{
		"start_date":  start30.Format("2006-01-02"),
		"end_date":    now.Add(24 * time.Hour).Format("2006-01-02"),
		"granularity": "day",
	}
	if raw, _, err := requestJSON(ctx, client, http.MethodGet, appendQuery(joinURL(root, "/api/v1/usage/dashboard/trend"), query), headers, nil); err == nil {
		summary["trend"] = rawObject(raw)
	} else {
		usageSummaryErrors(summary)["trend"] = err.Error()
	}
	if raw, _, err := requestJSON(ctx, client, http.MethodGet, appendQuery(joinURL(root, "/api/v1/usage/dashboard/models"), map[string]string{
		"start_date": start30.Format("2006-01-02"),
		"end_date":   now.Add(24 * time.Hour).Format("2006-01-02"),
	}), headers, nil); err == nil {
		summary["models"] = rawObject(raw)
	} else {
		usageSummaryErrors(summary)["models"] = err.Error()
	}
	return summary
}

func sub2APIBatchAPIKeyUsageRaw(ctx context.Context, client *http.Client, root string, headers map[string]string, keyIDs []string) json.RawMessage {
	ids := make([]int64, 0, len(keyIDs))
	for _, id := range keyIDs {
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err == nil && parsed > 0 {
			ids = append(ids, parsed)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"api_key_ids": ids})
	raw, _, err := requestJSON(ctx, client, http.MethodPost, joinURL(root, "/api/v1/usage/dashboard/api-keys-usage"), headers, body)
	if err != nil {
		return nil
	}
	return raw
}

func sub2APIBatchUsageByID(raw json.RawMessage) map[string]map[string]any {
	out := map[string]map[string]any{}
	if len(raw) == 0 {
		return out
	}
	root := objectFromAny(unwrapData(raw))
	stats := objectFromAny(root["stats"])
	if len(stats) == 0 {
		stats = objectFromAny(rawObject(raw)["stats"])
	}
	for key, value := range stats {
		obj := objectFromAny(value)
		if len(obj) == 0 {
			continue
		}
		id := firstNonEmpty(stringFromJSON(obj, "api_key_id", "apiKeyId", "id"), key)
		out[id] = obj
	}
	return out
}

func sub2APIKeyUsageSummary(dailyRaw json.RawMessage, batchUsage map[string]any) map[string]any {
	summary := newUsageSummary("sub2api_api_key")
	if len(batchUsage) > 0 {
		total := usageSummaryRangeFromUsageObject(batchUsage, "USD")
		if total == nil {
			total = map[string]any{"currency": "USD"}
		}
		if value := firstFloatFromJSON(batchUsage, "total_actual_cost", "totalActualCost"); value != nil {
			total["actualCost"] = *value
			total["cost"] = *value
		}
		usageSummarySetRange(summary, "total", total)
		today := map[string]any{"currency": "USD"}
		if value := firstFloatFromJSON(batchUsage, "today_actual_cost", "todayActualCost"); value != nil {
			today["actualCost"] = *value
			today["cost"] = *value
		}
		usageSummarySetRange(summary, "today", today)
	}
	if len(dailyRaw) > 0 {
		daily := unwrapData(dailyRaw)
		summary["daily"] = daily
		ranges := sub2APIDailyRanges(daily)
		for id, value := range ranges {
			usageSummarySetRange(summary, id, value)
		}
	}
	if ranges := usageSummaryRanges(summary); ranges["24h"] == nil {
		if today := objectFromAny(ranges["today"]); len(today) > 0 {
			ranges["24h"] = today
		}
	}
	return summary
}

func sub2APIDailyRanges(value any) map[string]map[string]any {
	items := arrayFromAny(objectFromAny(value)["items"])
	if len(items) == 0 {
		items = arrayFromAny(value)
	}
	out := map[string]map[string]any{}
	if len(items) == 0 {
		return out
	}
	now := time.Now().UTC()
	for _, spec := range usageRangeSpecs(now) {
		out[spec.id] = map[string]any{"currency": "USD"}
	}
	for _, item := range items {
		obj := objectFromAny(item)
		dateText := stringFromJSON(obj, "date")
		if dateText == "" {
			continue
		}
		day, err := time.Parse("2006-01-02", dateText)
		if err != nil {
			continue
		}
		for _, spec := range usageRangeSpecs(now) {
			if day.Before(time.Date(spec.start.Year(), spec.start.Month(), spec.start.Day(), 0, 0, 0, 0, time.UTC)) || day.After(spec.end) {
				continue
			}
			accumulateUsageRange(out[spec.id], obj)
		}
	}
	for key, value := range out {
		if len(value) <= 1 {
			delete(out, key)
		}
	}
	return out
}

func accumulateUsageRange(out map[string]any, obj map[string]any) {
	addUsageNumber(out, "cost", obj, "cost", "total_cost", "totalCost")
	addUsageNumber(out, "actualCost", obj, "actual_cost", "actualCost")
	addUsageNumber(out, "requests", obj, "requests", "total_requests", "totalRequests")
	addUsageNumber(out, "tokens", obj, "total_tokens", "totalTokens", "tokens")
	addUsageNumber(out, "inputTokens", obj, "input_tokens", "inputTokens")
	addUsageNumber(out, "outputTokens", obj, "output_tokens", "outputTokens")
}

func addUsageNumber(out map[string]any, outKey string, obj map[string]any, keys ...string) {
	value := firstFloatFromJSON(obj, keys...)
	if value == nil {
		return
	}
	current, _ := numberFromAny(out[outKey])
	out[outKey] = current + *value
}

func usageSummaryFromWindows(source string, windows []map[string]any) map[string]any {
	summary := newUsageSummary(source)
	for _, window := range windows {
		key := firstNonEmpty(stringFromJSON(window, "window"), stringFromJSON(window, "key"))
		rangeID := normalizeUsageRangeID(key)
		if rangeID == "" {
			continue
		}
		value := map[string]any{"currency": firstNonEmpty(stringFromJSON(window, "unit"), "%")}
		if used, ok := numberFromAny(window["used"]); ok {
			value["usage"] = used
		}
		if total, ok := numberFromAny(window["total"]); ok {
			value["total"] = total
		}
		if remaining, ok := numberFromAny(window["remaining"]); ok {
			value["remaining"] = remaining
		}
		usageSummarySetRange(summary, rangeID, value)
	}
	return summary
}

func normalizeUsageRangeID(value string) string {
	switch value {
	case "today":
		return "today"
	case "24h", "1d":
		return "24h"
	case "5h":
		return "5h"
	case "7d":
		return "7d"
	case "30d", "month", "monthly":
		return "30d"
	default:
		return ""
	}
}
