package connectors

import (
	"encoding/json"
	"strings"

	"api-monitor/internal/domain"
)

func mergeRaw(raw json.RawMessage, extras map[string]any) json.RawMessage {
	obj := rawObject(raw)
	for key, value := range extras {
		if value != nil {
			obj[key] = value
		}
	}
	return makeRaw(obj)
}

func firstQuota(values ...*domain.Quota) *domain.Quota {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func configuredUsageWindows(instance domain.Instance, defaults []map[string]any) []map[string]any {
	var windows []map[string]any
	settings := map[string]any{}
	if len(instance.Settings) > 0 {
		_ = json.Unmarshal(instance.Settings, &settings)
	}
	windows = append(windows, windowsFromAny(settings["usageWindows"])...)
	windows = append(windows, windowsFromAny(settings["usage_windows"])...)
	windows = append(windows, windowsFromAny(settings["windows"])...)
	if instance.Credential != nil {
		windows = append(windows, windowsFromAny(instance.Credential.JSON["usageWindows"])...)
		windows = append(windows, windowsFromAny(instance.Credential.JSON["usage_windows"])...)
	}
	if len(windows) == 0 {
		windows = defaults
	}
	return compactWindows(windows)
}

func windowsFromAny(value any) []map[string]any {
	items := arrayFromAny(value)
	if len(items) == 0 {
		if obj := objectFromAny(value); len(obj) > 0 {
			items = []any{obj}
		}
	}
	windows := make([]map[string]any, 0, len(items))
	for _, item := range items {
		obj := objectFromAny(item)
		if len(obj) == 0 {
			continue
		}
		key := firstNonEmpty(stringFromJSON(obj, "key", "id", "window"), "window")
		label := firstNonEmpty(stringFromJSON(obj, "label", "name"), key)
		windows = append(windows, normalizeWindow(obj, key, label))
	}
	return windows
}

func quotaWindowFromObject(object map[string]any, key, label, usedKeyA, usedKeyB, totalKeyA, totalKeyB, resetKeyA, resetKeyB string) map[string]any {
	used := floatFromJSON(object, usedKeyA, usedKeyB)
	total := floatFromJSON(object, totalKeyA, totalKeyB)
	remaining := floatFromJSON(object, strings.Replace(totalKeyA, "limit", "remaining", 1), strings.Replace(totalKeyB, "limit", "remaining", 1))
	if used == nil && total == nil && remaining == nil {
		return nil
	}
	raw := map[string]any{
		"key":        key,
		"label":      label,
		"unit":       firstNonEmpty(stringFromJSON(object, "quota_unit", "unit"), "USD"),
		"source":     "sub2api",
		"window":     strings.TrimPrefix(strings.TrimPrefix(key, "openai_"), "anthropic_"),
		"resetAt":    firstNonEmpty(stringFromJSON(object, resetKeyA, resetKeyB), ""),
		"windowFrom": stringFromJSON(object, resetKeyB),
	}
	if used != nil {
		raw["used"] = *used
	}
	if total != nil {
		raw["total"] = *total
	}
	if remaining != nil {
		raw["remaining"] = *remaining
	}
	return normalizeWindow(raw, key, label)
}

func normalizeWindow(raw map[string]any, key, label string) map[string]any {
	out := map[string]any{
		"key":    firstNonEmpty(stringFromJSON(raw, "key", "id", "window"), key),
		"label":  firstNonEmpty(stringFromJSON(raw, "label", "name"), label),
		"unit":   firstNonEmpty(stringFromJSON(raw, "unit", "quotaUnit"), "quota"),
		"source": firstNonEmpty(stringFromJSON(raw, "source"), "configured"),
	}
	copyString(out, raw, "window", "window")
	copyString(out, raw, "resetAt", "resetAt", "reset_at", "expires_at")
	copyString(out, raw, "windowFrom", "windowFrom", "window_start")
	copyNumber(out, raw, "used", "used", "usage", "usedAmount")
	copyNumber(out, raw, "total", "total", "limit", "quota")
	copyNumber(out, raw, "remaining", "remaining", "available", "remain")
	copyNumber(out, raw, "warnPercent", "warnPercent", "warn_percent")
	copyNumber(out, raw, "criticalPercent", "criticalPercent", "critical_percent")
	if _, ok := out["remaining"]; !ok {
		used, usedOK := numberFromAny(out["used"])
		total, totalOK := numberFromAny(out["total"])
		if usedOK && totalOK {
			out["remaining"] = total - used
		}
	}
	used, usedOK := numberFromAny(out["used"])
	total, totalOK := numberFromAny(out["total"])
	remaining, remainingOK := numberFromAny(out["remaining"])
	if totalOK && total > 0 {
		if !usedOK && remainingOK {
			used = total - remaining
			out["used"] = used
		}
		if remainingOK {
			out["utilization"] = (total - remaining) / total * 100
		} else if usedOK {
			out["utilization"] = used / total * 100
		}
	}
	out["status"] = statusForWindow(out)
	return out
}

func compactWindows(windows []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(windows))
	seen := map[string]bool{}
	for _, window := range windows {
		if len(window) == 0 {
			continue
		}
		key := stringFromJSON(window, "key")
		if key == "" {
			key = stringFromJSON(window, "label")
		}
		if key != "" && seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, window)
	}
	return out
}

func quotaFromUsageWindows(windows []map[string]any) *domain.Quota {
	var chosen map[string]any
	bestRatio := 2.0
	bestRemaining := 0.0
	hasRemaining := false
	for _, window := range windows {
		remaining, remainingOK := numberFromAny(window["remaining"])
		total, totalOK := numberFromAny(window["total"])
		if totalOK && total > 0 && remainingOK {
			ratio := remaining / total
			if chosen == nil || ratio < bestRatio {
				chosen = window
				bestRatio = ratio
			}
			continue
		}
		if remainingOK && (!hasRemaining || remaining < bestRemaining) {
			chosen = window
			bestRemaining = remaining
			hasRemaining = true
		}
	}
	if chosen == nil {
		return nil
	}
	var quota domain.Quota
	quota.Unit = firstNonEmpty(stringFromJSON(chosen, "unit"), "quota")
	if used, ok := numberFromAny(chosen["used"]); ok {
		quota.Used = &used
	}
	if total, ok := numberFromAny(chosen["total"]); ok {
		quota.Total = &total
	}
	if remaining, ok := numberFromAny(chosen["remaining"]); ok {
		quota.Remaining = &remaining
	}
	return &quota
}

func statusFromUsageWindows(windows []map[string]any, fallback domain.HealthStatus) domain.HealthStatus {
	status := fallback
	for _, window := range windows {
		switch domain.HealthStatus(stringFromJSON(window, "status")) {
		case domain.StatusCritical:
			return domain.StatusCritical
		case domain.StatusWarning:
			status = domain.StatusWarning
		case domain.StatusHealthy:
			if status == "" || status == domain.StatusUnknown {
				status = domain.StatusHealthy
			}
		}
	}
	if status == "" {
		return domain.StatusUnknown
	}
	return status
}

func statusForWindow(window map[string]any) string {
	remaining, remainingOK := numberFromAny(window["remaining"])
	total, totalOK := numberFromAny(window["total"])
	if !remainingOK || !totalOK || total <= 0 {
		return string(domain.StatusUnknown)
	}
	percent := remaining / total * 100
	warn := 20.0
	critical := 5.0
	if value, ok := numberFromAny(window["warnPercent"]); ok {
		warn = value
	}
	if value, ok := numberFromAny(window["criticalPercent"]); ok {
		critical = value
	}
	if percent <= critical {
		return string(domain.StatusCritical)
	}
	if percent <= warn {
		return string(domain.StatusWarning)
	}
	return string(domain.StatusHealthy)
}

func copyString(out map[string]any, raw map[string]any, outKey string, keys ...string) {
	if value := stringFromJSON(raw, keys...); value != "" {
		out[outKey] = value
	}
}

func copyNumber(out map[string]any, raw map[string]any, outKey string, keys ...string) {
	for _, key := range keys {
		if value, ok := numberFromAny(raw[key]); ok {
			out[outKey] = value
			return
		}
	}
}

func numberFromAny(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		if parsed := floatFromJSON(map[string]any{"v": typed}, "v"); parsed != nil {
			return *parsed, true
		}
	}
	return 0, false
}
