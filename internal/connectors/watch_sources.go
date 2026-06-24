package connectors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"api-monitor/internal/domain"
)

type watchTargetSpec struct {
	Kind         domain.TargetKind
	Name         string
	ExternalID   string
	Capabilities []domain.Capability
}

func newAPIWatchTargets(instance domain.Instance) []domain.MonitorTarget {
	return watchTargets(instance, instance.ProviderKind, []watchTargetSpec{
		{Kind: domain.TargetAnnouncement, Name: "Upstream announcements", ExternalID: "newapi:announcements", Capabilities: capabilities(domain.CapabilityAnnouncement, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetGroupCatalog, Name: "Groups and ratios", ExternalID: "newapi:groups", Capabilities: capabilities(domain.CapabilityGroupCatalog, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetModelCatalog, Name: "Upstream models", ExternalID: "newapi:models", Capabilities: capabilities(domain.CapabilityModelCatalog, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetPricing, Name: "Pricing and ratios", ExternalID: "newapi:pricing", Capabilities: capabilities(domain.CapabilityPricing, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
	})
}

func sub2APIWatchTargets(instance domain.Instance) []domain.MonitorTarget {
	return watchTargets(instance, instance.ProviderKind, []watchTargetSpec{
		{Kind: domain.TargetAnnouncement, Name: "Upstream announcements", ExternalID: "sub2api:announcements", Capabilities: capabilities(domain.CapabilityAnnouncement, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetGroupCatalog, Name: "Groups and channel rates", ExternalID: "sub2api:groups", Capabilities: capabilities(domain.CapabilityGroupCatalog, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetModelCatalog, Name: "Upstream models", ExternalID: "sub2api:models", Capabilities: capabilities(domain.CapabilityModelCatalog, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetPricing, Name: "Model pricing", ExternalID: "sub2api:pricing", Capabilities: capabilities(domain.CapabilityPricing, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
	})
}

func officialWatchTargets(instance domain.Instance, provider string, kind domain.ProviderKind) []domain.MonitorTarget {
	return watchTargets(instance, kind, []watchTargetSpec{
		{Kind: domain.TargetNewsFeed, Name: officialProviderName(provider) + " release notes", ExternalID: provider + ":news", Capabilities: capabilities(domain.CapabilityNews, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetDeprecation, Name: officialProviderName(provider) + " model deprecations", ExternalID: provider + ":deprecations", Capabilities: capabilities(domain.CapabilityDeprecation, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetModelCatalog, Name: officialProviderName(provider) + " model catalog", ExternalID: provider + ":models", Capabilities: capabilities(domain.CapabilityModelCatalog, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
		{Kind: domain.TargetPricing, Name: officialProviderName(provider) + " pricing", ExternalID: provider + ":pricing", Capabilities: capabilities(domain.CapabilityPricing, domain.CapabilityChangeWatch, domain.CapabilityHealth)},
	})
}

func watchTargets(instance domain.Instance, providerKind domain.ProviderKind, specs []watchTargetSpec) []domain.MonitorTarget {
	out := make([]domain.MonitorTarget, 0, len(specs))
	for _, spec := range specs {
		out = append(out, domain.MonitorTarget{
			InstanceID:   instance.ID,
			ProviderKind: providerKind,
			Kind:         spec.Kind,
			Name:         spec.Name,
			ExternalID:   spec.ExternalID,
			GroupName:    instance.GroupName,
			Capabilities: spec.Capabilities,
			Status:       domain.StatusUnknown,
			Raw: watchRaw(watchPayload{
				Source:    spec.ExternalID,
				WatchKind: string(spec.Kind),
				Summary:   "Waiting for first scan",
			}),
			Enabled: true,
		})
	}
	return out
}

func isWatchTarget(kind domain.TargetKind) bool {
	switch kind {
	case domain.TargetAnnouncement, domain.TargetNewsFeed, domain.TargetDeprecation, domain.TargetGroupCatalog, domain.TargetModelCatalog, domain.TargetPricing:
		return true
	default:
		return false
	}
}

func scanNewAPIWatch(ctx context.Context, client *http.Client, instance domain.Instance, target domain.MonitorTarget, headers map[string]string) (*domain.ScanResult, error) {
	root := baseURL(instance, "")
	switch target.Kind {
	case domain.TargetAnnouncement:
		raw, path, err := requestFirstJSON(ctx, client, http.MethodGet, root, []string{"/api/notice"}, map[string]string{}, nil)
		return watchJSONResult("newapi.announcements", string(target.Kind), joinURL(root, path), raw, err), nil
	case domain.TargetGroupCatalog:
		payload := map[string]any{}
		raw, path, err := requestFirstJSON(ctx, client, http.MethodGet, root, []string{"/api/user/self/groups", "/api/user/groups"}, headers, nil)
		if err != nil {
			return watchJSONResult("newapi.groups", string(target.Kind), joinURL(root, path), raw, err), nil
		}
		payload["groups"] = rawObject(raw)
		if ratioRaw, _, ratioErr := requestJSON(ctx, client, http.MethodGet, joinURL(root, "/api/ratio_config"), map[string]string{}, nil); ratioErr == nil {
			payload["ratioConfig"] = rawObject(ratioRaw)
		}
		return watchPayloadResult("newapi.groups", string(target.Kind), joinURL(root, path), payload, nil), nil
	case domain.TargetModelCatalog:
		raw, path, err := requestFirstJSON(ctx, client, http.MethodGet, root, []string{"/api/user/models", "/api/models"}, headers, nil)
		return watchJSONResult("newapi.models", string(target.Kind), joinURL(root, path), raw, err), nil
	case domain.TargetPricing:
		raw, path, err := requestFirstJSON(ctx, client, http.MethodGet, root, []string{"/api/pricing", "/api/ratio_config"}, headers, nil)
		return watchJSONResult("newapi.pricing", string(target.Kind), joinURL(root, path), raw, err), nil
	default:
		return nil, fmt.Errorf("unsupported watch target kind %s", target.Kind)
	}
}

func scanSub2APIWatch(ctx context.Context, client *http.Client, instance domain.Instance, target domain.MonitorTarget, headers map[string]string) (*domain.ScanResult, error) {
	root := baseURL(instance, "")
	switch target.Kind {
	case domain.TargetAnnouncement:
		raw, path, err := requestFirstJSON(ctx, client, http.MethodGet, root, []string{"/api/v1/announcements"}, headers, nil)
		return watchJSONResult("sub2api.announcements", string(target.Kind), joinURL(root, path), raw, err), nil
	case domain.TargetGroupCatalog:
		payload := map[string]any{}
		var mainErr error
		if raw, _, err := requestJSON(ctx, client, http.MethodGet, joinURL(root, "/api/v1/groups/available"), headers, nil); err == nil {
			payload["availableGroups"] = rawObject(raw)
		} else {
			mainErr = err
			payload["availableGroupsError"] = err.Error()
		}
		if raw, _, err := requestJSON(ctx, client, http.MethodGet, joinURL(root, "/api/v1/groups/rates"), headers, nil); err == nil {
			payload["groupRates"] = rawObject(raw)
		} else if mainErr == nil {
			mainErr = err
			payload["groupRatesError"] = err.Error()
		}
		if raw, _, err := requestJSON(ctx, client, http.MethodGet, joinURL(root, "/api/v1/channels/available"), headers, nil); err == nil {
			payload["channels"] = rawObject(raw)
		}
		return watchPayloadResult("sub2api.groups", string(target.Kind), joinURL(root, "/api/v1/groups/available"), payload, mainErr), nil
	case domain.TargetModelCatalog:
		raw, path, err := requestFirstJSON(ctx, client, http.MethodGet, root, []string{"/api/v1/channels/available", "/api/v1/models"}, headers, nil)
		return watchJSONResult("sub2api.models", string(target.Kind), joinURL(root, path), raw, err), nil
	case domain.TargetPricing:
		raw, path, err := requestFirstJSON(ctx, client, http.MethodGet, root, []string{"/api/v1/channels/available", "/api/v1/groups/rates"}, headers, nil)
		return watchJSONResult("sub2api.pricing", string(target.Kind), joinURL(root, path), raw, err), nil
	default:
		return nil, fmt.Errorf("unsupported watch target kind %s", target.Kind)
	}
}

func scanOfficialWatch(ctx context.Context, client *http.Client, instance domain.Instance, provider string, target domain.MonitorTarget) (*domain.ScanResult, error) {
	if target.Kind == domain.TargetModelCatalog {
		if raw, sourceURL, err := officialModelCatalog(ctx, client, instance, provider); err == nil {
			return watchJSONResult(provider+".models", string(target.Kind), sourceURL, raw, nil), nil
		}
	}
	sourceURL := officialWatchURL(provider, target.Kind)
	text, err := requestText(ctx, client, sourceURL)
	return watchHTMLResult(provider+"."+string(target.Kind), string(target.Kind), sourceURL, text, err), nil
}

func officialModelCatalog(ctx context.Context, client *http.Client, instance domain.Instance, provider string) (json.RawMessage, string, error) {
	switch provider {
	case "openai":
		key := officialAPIKey(instance)
		if key == "" {
			return nil, "", errMissingCredential()
		}
		endpoint := joinURL(baseURL(instance, "https://api.openai.com"), "/v1/models")
		raw, _, err := requestJSON(ctx, client, http.MethodGet, endpoint, map[string]string{"Authorization": "Bearer " + key}, nil)
		return raw, endpoint, err
	case "anthropic":
		key := officialAPIKey(instance)
		if key == "" {
			return nil, "", errMissingCredential()
		}
		endpoint := joinURL(baseURL(instance, "https://api.anthropic.com"), "/v1/models")
		raw, _, err := requestJSON(ctx, client, http.MethodGet, endpoint, map[string]string{"x-api-key": key, "anthropic-version": "2023-06-01"}, nil)
		return raw, endpoint, err
	case "gemini":
		root := baseURL(instance, "https://generativelanguage.googleapis.com")
		if key := officialAPIKey(instance); key != "" {
			endpoint := appendQuery(joinURL(root, "/v1beta/models"), map[string]string{"key": key})
			raw, _, err := requestJSON(ctx, client, http.MethodGet, endpoint, map[string]string{}, nil)
			return raw, endpoint, err
		}
		if token := officialAccessToken(instance); token != "" {
			endpoint := joinURL(root, "/v1beta/models")
			raw, _, err := requestJSON(ctx, client, http.MethodGet, endpoint, map[string]string{"Authorization": "Bearer " + token}, nil)
			return raw, endpoint, err
		}
	}
	return nil, "", errMissingCredential()
}

func officialWatchURL(provider string, kind domain.TargetKind) string {
	switch provider {
	case "openai":
		switch kind {
		case domain.TargetModelCatalog:
			return "https://developers.openai.com/api/docs/models"
		case domain.TargetPricing:
			return "https://developers.openai.com/api/docs/pricing"
		case domain.TargetDeprecation:
			return "https://developers.openai.com/api/docs/deprecations"
		default:
			return "https://developers.openai.com/api/docs/changelog"
		}
	case "anthropic":
		switch kind {
		case domain.TargetModelCatalog:
			return "https://docs.anthropic.com/en/docs/about-claude/models/overview"
		case domain.TargetPricing:
			return "https://docs.anthropic.com/en/docs/about-claude/pricing"
		case domain.TargetDeprecation:
			return "https://docs.anthropic.com/en/docs/about-claude/model-deprecations"
		default:
			return "https://docs.anthropic.com/en/release-notes/api"
		}
	case "gemini":
		switch kind {
		case domain.TargetModelCatalog:
			return "https://ai.google.dev/gemini-api/docs/models"
		case domain.TargetPricing:
			return "https://ai.google.dev/gemini-api/docs/pricing"
		case domain.TargetDeprecation:
			return "https://ai.google.dev/gemini-api/docs/deprecations"
		default:
			return "https://ai.google.dev/gemini-api/docs/changelog"
		}
	default:
		return ""
	}
}

func officialProviderName(provider string) string {
	switch provider {
	case "openai":
		return "OpenAI"
	case "anthropic":
		return "Anthropic"
	case "gemini":
		return "Gemini"
	default:
		return provider
	}
}

type watchPayload struct {
	Source      string           `json:"source"`
	WatchKind   string           `json:"watchKind"`
	SourceURL   string           `json:"sourceUrl,omitempty"`
	CheckedAt   string           `json:"checkedAt,omitempty"`
	Fingerprint string           `json:"fingerprint,omitempty"`
	Summary     string           `json:"summary,omitempty"`
	Count       int              `json:"count"`
	Items       []map[string]any `json:"items,omitempty"`
	Payload     any              `json:"payload,omitempty"`
	Error       string           `json:"error,omitempty"`
}

func watchJSONResult(source, watchKind, sourceURL string, raw json.RawMessage, err error) *domain.ScanResult {
	var payload any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	return watchPayloadResult(source, watchKind, sourceURL, payload, err)
}

func watchPayloadResult(source, watchKind, sourceURL string, payload any, err error) *domain.ScanResult {
	items := watchItems(payload)
	status := domain.StatusHealthy
	if err != nil {
		status = domain.StatusWarning
	}
	return &domain.ScanResult{
		Status:       status,
		Capabilities: watchCapabilities(watchKind),
		Raw: watchRaw(watchPayload{
			Source:    source,
			WatchKind: watchKind,
			SourceURL: sourceURL,
			Items:     items,
			Payload:   payload,
			Error:     errorText(err),
		}),
	}
}

func watchHTMLResult(source, watchKind, sourceURL string, text string, err error) *domain.ScanResult {
	items := htmlItems(text, sourceURL)
	status := domain.StatusHealthy
	if err != nil {
		status = domain.StatusWarning
	}
	payload := map[string]any{
		"title": titleFromHTML(text),
		"text":  compactText(stripHTML(text), 5000),
	}
	return &domain.ScanResult{
		Status:       status,
		Capabilities: watchCapabilities(watchKind),
		Raw: watchRaw(watchPayload{
			Source:    source,
			WatchKind: watchKind,
			SourceURL: sourceURL,
			Items:     items,
			Payload:   payload,
			Error:     errorText(err),
		}),
	}
}

func watchCapabilities(watchKind string) []domain.Capability {
	switch domain.TargetKind(watchKind) {
	case domain.TargetAnnouncement:
		return capabilities(domain.CapabilityAnnouncement, domain.CapabilityChangeWatch, domain.CapabilityHealth)
	case domain.TargetNewsFeed:
		return capabilities(domain.CapabilityNews, domain.CapabilityChangeWatch, domain.CapabilityHealth)
	case domain.TargetDeprecation:
		return capabilities(domain.CapabilityDeprecation, domain.CapabilityChangeWatch, domain.CapabilityHealth)
	case domain.TargetGroupCatalog:
		return capabilities(domain.CapabilityGroupCatalog, domain.CapabilityChangeWatch, domain.CapabilityHealth)
	case domain.TargetModelCatalog:
		return capabilities(domain.CapabilityModelCatalog, domain.CapabilityChangeWatch, domain.CapabilityHealth)
	case domain.TargetPricing:
		return capabilities(domain.CapabilityPricing, domain.CapabilityChangeWatch, domain.CapabilityHealth)
	default:
		return capabilities(domain.CapabilityChangeWatch, domain.CapabilityHealth)
	}
}

func watchRaw(payload watchPayload) json.RawMessage {
	payload.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	payload.Count = len(payload.Items)
	if payload.Summary == "" {
		payload.Summary = watchSummary(payload.Items, payload.Error)
	}
	payload.Fingerprint = stableFingerprint(map[string]any{
		"source":    payload.Source,
		"watchKind": payload.WatchKind,
		"items":     payload.Items,
		"payload":   payload.Payload,
		"error":     payload.Error,
	})
	return makeRaw(payload)
}

func watchItems(payload any) []map[string]any {
	if payload == nil {
		return nil
	}
	switch typed := payload.(type) {
	case []any:
		return itemsFromArray(typed)
	case map[string]any:
		for _, key := range []string{"announcements", "notices", "items", "list", "models", "groups", "data", "result"} {
			if arr := arrayFromAny(typed[key]); len(arr) > 0 {
				return itemsFromArray(arr)
			}
		}
		if nested := objectFromAny(typed["payload"]); len(nested) > 0 {
			return watchItems(nested)
		}
		return []map[string]any{itemFromObject(typed)}
	default:
		return []map[string]any{{"title": fmt.Sprint(typed), "summary": compactText(fmt.Sprint(typed), 240)}}
	}
}

func itemsFromArray(values []any) []map[string]any {
	out := make([]map[string]any, 0, len(values))
	for _, value := range values {
		obj := objectFromAny(value)
		if len(obj) == 0 {
			out = append(out, map[string]any{"title": fmt.Sprint(value), "summary": compactText(fmt.Sprint(value), 240)})
			continue
		}
		out = append(out, itemFromObject(obj))
	}
	return out
}

func itemFromObject(obj map[string]any) map[string]any {
	title := firstNonEmpty(
		stringFromJSON(obj, "title", "name", "model", "id", "group", "channel_name", "platform"),
		compactText(stringFromJSON(obj, "content", "message", "description", "desc"), 80),
		"Item",
	)
	summary := firstNonEmpty(
		compactText(stringFromJSON(obj, "summary", "description", "desc", "content", "message"), 260),
		title,
	)
	item := map[string]any{
		"id":      firstNonEmpty(stringFromJSON(obj, "id", "uuid", "key", "model", "name"), title),
		"title":   title,
		"summary": summary,
	}
	if url := stringFromJSON(obj, "url", "link", "href"); url != "" {
		item["url"] = url
	}
	if published := stringFromJSON(obj, "published_at", "publishedAt", "created_at", "createdAt", "updated_at", "updatedAt", "date"); published != "" {
		item["publishedAt"] = published
	}
	return item
}

func htmlItems(text, sourceURL string) []map[string]any {
	clean := strings.ReplaceAll(text, "\n", " ")
	re := regexp.MustCompile(`(?is)<h[1-3][^>]*>(.*?)</h[1-3]>`)
	matches := re.FindAllStringSubmatch(clean, 12)
	items := make([]map[string]any, 0, len(matches))
	for _, match := range matches {
		title := compactText(stripHTML(match[1]), 120)
		if title == "" {
			continue
		}
		items = append(items, map[string]any{
			"id":      stableFingerprint(title),
			"title":   title,
			"summary": title,
			"url":     sourceURL,
		})
	}
	if len(items) == 0 {
		title := titleFromHTML(text)
		if title != "" {
			items = append(items, map[string]any{"id": stableFingerprint(title), "title": title, "summary": title, "url": sourceURL})
		}
	}
	return items
}

func titleFromHTML(text string) string {
	re := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	match := re.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return compactText(stripHTML(match[1]), 160)
}

func stripHTML(text string) string {
	text = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(text, " ")
	return html.UnescapeString(text)
}

func compactText(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if limit > 0 && len(text) > limit {
		return strings.TrimSpace(text[:limit]) + "..."
	}
	return text
}

func watchSummary(items []map[string]any, errText string) string {
	if errText != "" {
		return errText
	}
	if len(items) == 0 {
		return "No items returned"
	}
	title := ""
	if value, ok := items[0]["title"].(string); ok {
		title = value
	}
	if len(items) == 1 {
		return title
	}
	return fmt.Sprintf("%s and %d more item(s)", title, len(items)-1)
}

func stableFingerprint(value any) string {
	normalized := normalizeForFingerprint(value)
	data, _ := json.Marshal(normalized)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func normalizeForFingerprint(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			if key == "checkedAt" || key == "lastScanAt" || key == "updatedAt" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(keys))
		for _, key := range keys {
			out[key] = normalizeForFingerprint(typed[key])
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeForFingerprint(item))
		}
		return out
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeForFingerprint(item))
		}
		return out
	default:
		return typed
	}
}

func requestText(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	if endpoint == "" {
		return "", fmt.Errorf("empty source URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "api-monitor/1.0 (+https://github.com/baogutang/api-monitor)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if readErr != nil {
		return "", readErr
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return string(data), fmt.Errorf("source status %d", res.StatusCode)
	}
	return string(data), nil
}
