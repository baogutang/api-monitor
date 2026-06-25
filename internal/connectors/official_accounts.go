package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"api-monitor/internal/domain"
)

type openAIAccountConnector struct {
	client *http.Client
}

func (c *openAIAccountConnector) Kind() domain.ProviderKind { return domain.ProviderOpenAIAccount }

func (c *openAIAccountConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	return officialAccountTest(ctx, c.client, instance, "OpenAI official account", c.probe)
}

func (c *openAIAccountConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	return officialAccountTargets(instance, "openai", domain.ProviderOpenAIAccount, nil), nil
}

func (c *openAIAccountConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	if isWatchTarget(target.Kind) {
		return scanOfficialWatch(ctx, c.client, instance, "openai", target)
	}
	return officialAccountScan(ctx, c.client, instance, "openai", nil, c.probe)
}

func (c *openAIAccountConnector) probe(ctx context.Context, client *http.Client, instance domain.Instance) (json.RawMessage, error) {
	if token := officialAccessToken(instance); token != "" {
		return probeOpenAICodexUsage(ctx, client, instance, token)
	}
	key := officialAPIKey(instance)
	if key == "" {
		return nil, errors.New("OpenAI account saved; no OAuth access token or API key health probe configured")
	}
	raw, _, err := requestJSON(ctx, client, http.MethodGet, joinURL(baseURL(instance, "https://api.openai.com"), "/v1/models"), map[string]string{"Authorization": "Bearer " + key}, nil)
	return raw, err
}

type geminiAccountConnector struct {
	client *http.Client
}

func (c *geminiAccountConnector) Kind() domain.ProviderKind { return domain.ProviderGeminiAccount }

func (c *geminiAccountConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	return officialAccountTest(ctx, c.client, instance, "Gemini official account", c.probe)
}

func (c *geminiAccountConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	return officialAccountTargets(instance, "gemini", domain.ProviderGeminiAccount, nil), nil
}

func (c *geminiAccountConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	if isWatchTarget(target.Kind) {
		return scanOfficialWatch(ctx, c.client, instance, "gemini", target)
	}
	return officialAccountScan(ctx, c.client, instance, "gemini", nil, c.probe)
}

func (c *geminiAccountConnector) probe(ctx context.Context, client *http.Client, instance domain.Instance) (json.RawMessage, error) {
	root := baseURL(instance, "https://generativelanguage.googleapis.com")
	if key := officialAPIKey(instance); key != "" {
		raw, _, err := requestJSON(ctx, client, http.MethodGet, appendQuery(joinURL(root, "/v1beta/models"), map[string]string{"key": key}), map[string]string{}, nil)
		return raw, err
	}
	if token := officialAccessToken(instance); token != "" {
		raw, _, err := requestJSON(ctx, client, http.MethodGet, joinURL(root, "/v1beta/models"), map[string]string{"Authorization": "Bearer " + token}, nil)
		return raw, err
	}
	return nil, errors.New("Gemini account saved; no API key or OAuth access token health probe configured")
}

type anthropicAccountConnector struct {
	client *http.Client
}

func (c *anthropicAccountConnector) Kind() domain.ProviderKind {
	return domain.ProviderAnthropicAccount
}

func (c *anthropicAccountConnector) Test(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	return officialAccountTest(ctx, c.client, instance, "Claude official account", c.probe)
}

func (c *anthropicAccountConnector) Discover(ctx context.Context, instance domain.Instance) ([]domain.MonitorTarget, error) {
	return officialAccountTargets(instance, "anthropic", domain.ProviderAnthropicAccount, nil), nil
}

func (c *anthropicAccountConnector) Scan(ctx context.Context, instance domain.Instance, target domain.MonitorTarget) (*domain.ScanResult, error) {
	if isWatchTarget(target.Kind) {
		return scanOfficialWatch(ctx, c.client, instance, "anthropic", target)
	}
	return officialAccountScan(ctx, c.client, instance, "anthropic", nil, c.probe)
}

func (c *anthropicAccountConnector) probe(ctx context.Context, client *http.Client, instance domain.Instance) (json.RawMessage, error) {
	if token := officialAccessToken(instance); token != "" {
		return probeClaudeUsage(ctx, client, token)
	}
	key := officialAPIKey(instance)
	if key == "" {
		return nil, errors.New("Claude account saved; no OAuth access token or Anthropic API key health probe configured")
	}
	headers := map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"}
	raw, _, err := requestJSON(ctx, client, http.MethodGet, joinURL(baseURL(instance, "https://api.anthropic.com"), "/v1/models"), headers, nil)
	return raw, err
}

type officialProbeFunc func(context.Context, *http.Client, domain.Instance) (json.RawMessage, error)

type officialHeaderWindow struct {
	label string
	key   string
	used  *float64
	reset *int
	mins  *int
}

func officialAccountTest(ctx context.Context, client *http.Client, instance domain.Instance, label string, probe officialProbeFunc) (*domain.ProbeResult, error) {
	windows := configuredUsageWindows(instance, nil)
	if !hasOfficialCredential(instance) && len(windows) == 0 {
		err := errMissingCredential()
		return &domain.ProbeResult{OK: false, Status: domain.StatusCritical, Message: err.Error(), Capabilities: officialAccountCapabilities(), Raw: officialRaw(instance, "", windows, nil, err)}, err
	}
	raw, err := probe(ctx, client, instance)
	windows = mergeProbeUsageWindows(windows, raw)
	status := statusFromUsageWindows(windows, domain.StatusHealthy)
	ok := true
	message := label + " is configured"
	if err != nil {
		message = err.Error()
		if len(windows) == 0 {
			status = flexibleStatus(err)
			ok = false
		} else if status == domain.StatusHealthy {
			status = domain.StatusWarning
		}
	}
	return &domain.ProbeResult{OK: ok, Status: status, Message: message, Capabilities: officialAccountCapabilities(), Raw: officialRaw(instance, "", windows, raw, err)}, err
}

func officialAccountTargets(instance domain.Instance, provider string, kind domain.ProviderKind, defaults []map[string]any) []domain.MonitorTarget {
	windows := configuredUsageWindows(instance, defaults)
	meta := officialCredentialMeta(instance)
	name := firstNonEmpty(stringFromJSON(meta, "email", "name", "project_id", "org_uuid"), instance.Name)
	targets := []domain.MonitorTarget{{
		InstanceID:   instance.ID,
		ProviderKind: kind,
		Kind:         domain.TargetUser,
		Name:         name,
		ExternalID:   firstNonEmpty(stringFromJSON(meta, "account_id", "chatgpt_account_id", "project_id", "org_uuid", "email"), provider+"-account"),
		GroupName:    instance.GroupName,
		Capabilities: officialAccountCapabilities(),
		Status:       domain.StatusUnknown,
		Quota:        quotaFromUsageWindows(windows),
		Plan:         officialPlan(meta),
		Raw:          officialRaw(instance, provider, windows, nil, nil),
		Enabled:      true,
	}}
	targets = append(targets, officialWatchTargets(instance, provider, kind)...)
	return targets
}

func officialAccountScan(ctx context.Context, client *http.Client, instance domain.Instance, provider string, defaults []map[string]any, probe officialProbeFunc) (*domain.ScanResult, error) {
	windows := configuredUsageWindows(instance, defaults)
	raw, err := probe(ctx, client, instance)
	windows = mergeProbeUsageWindows(windows, raw)
	status := statusFromUsageWindows(windows, domain.StatusHealthy)
	if err != nil {
		if len(windows) == 0 {
			status = flexibleStatus(err)
		} else if status == domain.StatusHealthy {
			status = domain.StatusWarning
		}
	}
	return &domain.ScanResult{
		Status:       status,
		Quota:        quotaFromUsageWindows(windows),
		Plan:         officialPlan(officialCredentialMeta(instance)),
		Capabilities: officialAccountCapabilities(),
		Raw:          officialRaw(instance, provider, windows, raw, err),
		Error:        errorText(err),
	}, err
}

func officialAccountCapabilities() []domain.Capability {
	return capabilities(domain.CapabilityHealth, domain.CapabilityUsage, domain.CapabilityWindowQuota, domain.CapabilityManualPlan)
}

func officialRaw(instance domain.Instance, provider string, windows []map[string]any, probeRaw json.RawMessage, err error) json.RawMessage {
	raw := map[string]any{
		"provider":        provider,
		"account":         officialCredentialMeta(instance),
		"usageWindows":    windows,
		"usageSummary":    usageSummaryFromWindows(provider+"_official_account", windows),
		"credentialHints": officialCredentialHints(instance),
	}
	if len(probeRaw) > 0 {
		raw["probe"] = rawObject(probeRaw)
	}
	if err != nil {
		raw["probeError"] = err.Error()
	}
	return makeRaw(raw)
}

func officialCredentialMeta(instance domain.Instance) map[string]any {
	meta := map[string]any{}
	if instance.Credential == nil {
		return meta
	}
	for _, key := range []string{"email", "name", "plan_type", "plan", "project_id", "oauth_type", "tier_id", "org_uuid", "account_uuid", "chatgpt_account_id", "chatgpt_user_id", "organization_id", "expires_at"} {
		if value := stringFromJSON(instance.Credential.JSON, key); value != "" {
			meta[key] = value
		}
	}
	if instance.Credential.Username != "" {
		meta["email"] = instance.Credential.Username
	}
	return meta
}

func officialCredentialHints(instance domain.Instance) map[string]any {
	hints := map[string]any{"type": ""}
	if instance.Credential == nil {
		return hints
	}
	hints["type"] = instance.Credential.Type
	hints["hasAPIKey"] = officialAPIKey(instance) != ""
	hints["hasAccessToken"] = officialAccessToken(instance) != ""
	hints["hasRefreshToken"] = stringFromJSON(instance.Credential.JSON, "refresh_token", "refreshToken", "rt") != ""
	hints["hasSession"] = stringFromJSON(instance.Credential.JSON, "session_token", "sessionToken", "cookie") != ""
	return hints
}

func officialAPIKey(instance domain.Instance) string {
	if instance.Credential == nil {
		return ""
	}
	return firstNonEmpty(stringFromJSON(instance.Credential.JSON, "api_key", "apiKey", "key"), func() string {
		if instance.Credential.Type == "api_key" {
			return instance.Credential.Value
		}
		return ""
	}())
}

func officialAccessToken(instance domain.Instance) string {
	if instance.Credential == nil {
		return ""
	}
	return firstNonEmpty(stringFromJSON(instance.Credential.JSON, "access_token", "accessToken"), func() string {
		if instance.Credential.Type == "bearer" || instance.Credential.Type == "oauth" {
			return instance.Credential.Value
		}
		return ""
	}())
}

func hasOfficialCredential(instance domain.Instance) bool {
	if instance.Credential == nil {
		return false
	}
	return instance.Credential.Value != "" || instance.Credential.Username != "" || len(instance.Credential.JSON) > 0
}

func officialPlan(meta map[string]any) *domain.PlanInfo {
	plan := parsePlan(meta)
	if plan != nil {
		return plan
	}
	if name := firstNonEmpty(stringFromJSON(meta, "plan_type", "tier_id")); name != "" {
		return &domain.PlanInfo{Name: name}
	}
	return nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func probeOpenAICodexUsage(ctx context.Context, client *http.Client, instance domain.Instance, accessToken string) (json.RawMessage, error) {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	payload := map[string]any{
		"model": "gpt-5.4",
		"input": []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text",
				"text": "hi",
			}},
		}},
		"stream":       true,
		"store":        false,
		"instructions": "You are a helpful assistant.",
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Host = "chatgpt.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("Version", "0.125.0")
	req.Header.Set("User-Agent", "codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color")
	if accountID := stringFromJSON(officialCredentialMeta(instance), "chatgpt_account_id"); accountID != "" {
		req.Header.Set("chatgpt-account-id", accountID)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
	windows := codexWindowsFromHeaders(res.Header)
	raw := map[string]any{"usageWindows": windows, "source": "chatgpt-codex-headers", "status": res.StatusCode}
	if len(windows) == 0 && (res.StatusCode < 200 || res.StatusCode >= 300) {
		return makeRaw(raw), fmt.Errorf("OpenAI Codex probe returned status %d", res.StatusCode)
	}
	return makeRaw(raw), nil
}

func probeClaudeUsage(ctx context.Context, client *http.Client, accessToken string) (json.RawMessage, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "claude-code/2.1.7")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	obj := rawObject(json.RawMessage(data))
	windows := claudeWindowsFromUsage(obj)
	raw := mergeRaw(json.RawMessage(data), map[string]any{"usageWindows": windows, "source": "anthropic-oauth-usage"})
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return raw, fmt.Errorf("Claude usage endpoint returned status %d: %s", res.StatusCode, string(data))
	}
	return raw, nil
}

func codexWindowsFromHeaders(headers http.Header) []map[string]any {
	primary := officialHeaderWindow{
		label: "7D window",
		key:   "openai_7d",
		used:  headerFloat(headers, "x-codex-primary-used-percent"),
		reset: headerInt(headers, "x-codex-primary-reset-after-seconds"),
		mins:  headerInt(headers, "x-codex-primary-window-minutes"),
	}
	secondary := officialHeaderWindow{
		label: "5H window",
		key:   "openai_5h",
		used:  headerFloat(headers, "x-codex-secondary-used-percent"),
		reset: headerInt(headers, "x-codex-secondary-reset-after-seconds"),
		mins:  headerInt(headers, "x-codex-secondary-window-minutes"),
	}
	if primary.mins != nil && secondary.mins != nil && *primary.mins < *secondary.mins {
		primary.label, secondary.label = secondary.label, primary.label
		primary.key, secondary.key = secondary.key, primary.key
	}
	now := time.Now().UTC()
	return compactWindows([]map[string]any{
		codexWindow(primary, now),
		codexWindow(secondary, now),
	})
}

func codexWindow(window officialHeaderWindow, now time.Time) map[string]any {
	if window.used == nil && window.reset == nil && window.mins == nil {
		return nil
	}
	raw := map[string]any{
		"key":    window.key,
		"label":  window.label,
		"unit":   "%",
		"source": "openai-codex-headers",
	}
	if window.key == "openai_5h" {
		raw["window"] = "5h"
	} else {
		raw["window"] = "7d"
	}
	if window.used != nil {
		raw["used"] = *window.used
		raw["total"] = 100
		raw["remaining"] = 100 - *window.used
	}
	if window.reset != nil {
		raw["resetAt"] = now.Add(time.Duration(*window.reset) * time.Second).Format(time.RFC3339)
	}
	if window.mins != nil {
		raw["windowMinutes"] = *window.mins
	}
	return normalizeWindow(raw, window.key, window.label)
}

func claudeWindowsFromUsage(obj map[string]any) []map[string]any {
	return compactWindows([]map[string]any{
		claudeWindow(obj, "five_hour", "anthropic_5h", "5H window", "5h"),
		claudeWindow(obj, "seven_day", "anthropic_7d", "7D window", "7d"),
		claudeWindow(obj, "seven_day_sonnet", "anthropic_7d_sonnet", "7D Sonnet window", "7d"),
	})
}

func claudeWindow(obj map[string]any, field, key, label, window string) map[string]any {
	nested := objectFromAny(obj[field])
	if len(nested) == 0 {
		return nil
	}
	raw := map[string]any{
		"key":    key,
		"label":  label,
		"window": window,
		"unit":   "%",
		"source": "anthropic-oauth-usage",
	}
	if used := floatFromJSON(nested, "utilization"); used != nil {
		raw["used"] = *used
		raw["total"] = 100
		raw["remaining"] = 100 - *used
	}
	if resetAt := stringFromJSON(nested, "resets_at"); resetAt != "" {
		raw["resetAt"] = resetAt
	}
	return normalizeWindow(raw, key, label)
}

func mergeProbeUsageWindows(current []map[string]any, raw json.RawMessage) []map[string]any {
	obj := rawObject(raw)
	probed := windowsFromAny(obj["usageWindows"])
	if len(probed) == 0 {
		probed = windowsFromAny(obj["usage_windows"])
	}
	if len(probed) == 0 {
		return current
	}
	return compactWindows(probed)
}

func headerFloat(headers http.Header, key string) *float64 {
	if raw := headers.Get(key); raw != "" {
		if value, err := strconv.ParseFloat(raw, 64); err == nil {
			return &value
		}
	}
	return nil
}

func headerInt(headers http.Header, key string) *int {
	if raw := headers.Get(key); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			return &value
		}
	}
	return nil
}
