package scanner

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"api-monitor/internal/cache"
	"api-monitor/internal/connectors"
	"api-monitor/internal/domain"
	"api-monitor/internal/notify"
	"api-monitor/internal/store"
)

type Service struct {
	store     *store.Store
	registry  *connectors.Registry
	notifier  *notify.Service
	cache     *cache.Cache
	logger    *slog.Logger
	batchSize int
}

func New(st *store.Store, registry *connectors.Registry, notifier *notify.Service, cache *cache.Cache, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: st, registry: registry, notifier: notifier, cache: cache, logger: logger, batchSize: 50}
}

func (s *Service) DiscoverInstance(ctx context.Context, instanceID string) ([]domain.MonitorTarget, error) {
	instance, err := s.store.GetInstance(ctx, instanceID, true)
	if err != nil {
		return nil, err
	}
	connector, ok := s.registry.Get(instance.ProviderKind)
	if !ok {
		return nil, fmt.Errorf("unsupported provider kind %s", instance.ProviderKind)
	}
	targets, err := connector.Discover(ctx, *instance)
	if err != nil {
		return nil, err
	}
	var saved []domain.MonitorTarget
	for _, target := range targets {
		if target.GroupName == "" {
			target.GroupName = instance.GroupName
		}
		if target.ProviderKind == "" {
			target.ProviderKind = instance.ProviderKind
		}
		if target.InstanceID == "" {
			target.InstanceID = instance.ID
		}
		target.Enabled = true
		target.RiskScore = store.RiskScore(target)
		next := time.Now().UTC().Add(time.Duration(instance.ScanIntervalSeconds) * time.Second)
		target.NextScanAt = &next
		out, err := s.store.UpsertTarget(ctx, target)
		if err != nil {
			return nil, err
		}
		saved = append(saved, *out)
	}
	if s.cache != nil {
		_ = s.cache.InvalidateConfig(ctx)
	}
	return saved, nil
}

func (s *Service) TestInstance(ctx context.Context, instanceID string) (*domain.ProbeResult, error) {
	instance, err := s.store.GetInstance(ctx, instanceID, true)
	if err != nil {
		return nil, err
	}
	connector, ok := s.registry.Get(instance.ProviderKind)
	if !ok {
		return nil, fmt.Errorf("unsupported provider kind %s", instance.ProviderKind)
	}
	return connector.Test(ctx, *instance)
}

func (s *Service) TestDraftInstance(ctx context.Context, instance domain.Instance) (*domain.ProbeResult, error) {
	connector, ok := s.registry.Get(instance.ProviderKind)
	if !ok {
		return nil, fmt.Errorf("unsupported provider kind %s", instance.ProviderKind)
	}
	return connector.Test(ctx, instance)
}

func (s *Service) ScanTarget(ctx context.Context, targetID string) (*domain.MonitorTarget, error) {
	target, err := s.store.GetTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	instance, err := s.store.GetInstance(ctx, target.InstanceID, true)
	if err != nil {
		return nil, err
	}
	connector, ok := s.registry.Get(instance.ProviderKind)
	if !ok {
		return nil, fmt.Errorf("unsupported provider kind %s", instance.ProviderKind)
	}
	run, err := s.store.CreateScanRun(ctx, target.ID, instance.ID)
	if err != nil {
		return nil, err
	}
	result, scanErr := connector.Scan(ctx, *instance, *target)
	if result == nil {
		result = &domain.ScanResult{Status: domain.StatusWarning}
	}
	updated, updateErr := s.store.UpdateTargetScanResult(ctx, *target, *result, instance.ScanIntervalSeconds)
	status := "success"
	errText := ""
	if scanErr != nil || updateErr != nil {
		status = "failed"
		if scanErr != nil {
			errText = scanErr.Error()
		} else {
			errText = updateErr.Error()
		}
	}
	raw := result.Raw
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	_ = s.store.FinishScanRun(ctx, run.ID, status, errText, raw)
	if updateErr != nil {
		return nil, updateErr
	}
	if updated != nil {
		_ = s.EvaluateRules(ctx, *updated)
	}
	return updated, scanErr
}

func (s *Service) RunDueOnce(ctx context.Context) error {
	targets, err := s.store.DueTargets(ctx, s.batchSize)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if _, err := s.ScanTarget(ctx, target.ID); err != nil {
			s.logger.Warn("scan target failed", "target_id", target.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) RunLoop(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := s.RunDueOnce(ctx); err != nil {
			s.logger.Warn("due scan failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *Service) EvaluateRules(ctx context.Context, target domain.MonitorTarget) error {
	rules, err := s.store.ListAlertRules(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !rule.Enabled || !ruleMatchesTarget(rule, target) {
			continue
		}
		matched, err := s.conditionMet(ctx, rule, target)
		if err != nil {
			s.logger.Warn("evaluate rule condition failed", "rule_id", rule.ID, "target_id", target.ID, "error", err)
			continue
		}
		if !matched {
			continue
		}
		if !isChangeCondition(rule.ConditionType) {
			if existing, err := s.store.FindOpenAlert(ctx, target.ID, rule.ID); err == nil && existing != nil {
				continue
			}
		}
		if isChangeCondition(rule.ConditionType) && !s.changeCooldownElapsed(ctx, target.ID, rule.ID, rule.CooldownSeconds) {
			continue
		}
		alert, err := s.store.CreateAlert(ctx, domain.AlertEvent{
			TargetID: target.ID,
			RuleID:   rule.ID,
			Severity: rule.Severity,
			Status:   "open",
			Title:    alertTitle(rule, target),
			Message:  alertMessage(rule, target),
		})
		if err != nil {
			return err
		}
		_ = s.deliverAlert(ctx, *alert, target, rule.NotificationChannelIDs)
	}
	return nil
}

func (s *Service) deliverAlert(ctx context.Context, alert domain.AlertEvent, target domain.MonitorTarget, channelIDs []string) error {
	if len(channelIDs) == 0 {
		return nil
	}
	channels, err := s.store.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}
	byID := map[string]domain.NotificationChannel{}
	for _, channel := range channels {
		byID[channel.ID] = channel
	}
	for _, id := range channelIDs {
		channel, ok := byID[id]
		if !ok {
			continue
		}
		full, err := s.store.GetNotificationChannel(ctx, channel.ID, true)
		if err != nil {
			continue
		}
		_, _ = s.notifier.Send(ctx, *full, alert, &target)
	}
	return nil
}

func ruleMatchesTarget(rule domain.AlertRule, target domain.MonitorTarget) bool {
	switch rule.ScopeType {
	case "", "global":
		return true
	case "provider":
		return string(target.ProviderKind) == rule.ScopeValue
	case "group":
		return target.GroupName == rule.ScopeValue
	case "instance":
		return target.InstanceID == rule.ScopeValue
	case "asset", "target":
		return target.ID == rule.ScopeValue
	default:
		return false
	}
}

func (s *Service) conditionMet(ctx context.Context, rule domain.AlertRule, target domain.MonitorTarget) (bool, error) {
	switch rule.ConditionType {
	case "balance_below":
		return target.Balance != nil && target.Balance.Amount < rule.ThresholdValue, nil
	case "remaining_quota_below":
		return target.Quota != nil && target.Quota.Remaining != nil && *target.Quota.Remaining < rule.ThresholdValue, nil
	case "remaining_percent_below":
		if target.Quota == nil || target.Quota.Remaining == nil || target.Quota.Total == nil || *target.Quota.Total == 0 {
			return false, nil
		}
		return (*target.Quota.Remaining / *target.Quota.Total * 100) < rule.ThresholdValue, nil
	case "days_until_expiry_below":
		if target.Plan == nil || target.Plan.ExpireAt == nil {
			return false, nil
		}
		return time.Until(*target.Plan.ExpireAt).Hours()/24 < rule.ThresholdValue, nil
	case "health_not_healthy":
		return target.Status != domain.StatusHealthy, nil
	case "monthly_cost_above":
		return target.MonthlyCost != nil && target.MonthlyCost.Amount > rule.ThresholdValue, nil
	case "announcement_changed":
		return s.watchFingerprintChanged(ctx, target, domain.CapabilityAnnouncement)
	case "news_changed":
		return s.watchFingerprintChanged(ctx, target, domain.CapabilityNews)
	case "deprecation_changed":
		return s.watchFingerprintChanged(ctx, target, domain.CapabilityDeprecation)
	case "group_catalog_changed":
		return s.watchFingerprintChanged(ctx, target, domain.CapabilityGroupCatalog)
	case "model_catalog_changed":
		return s.watchFingerprintChanged(ctx, target, domain.CapabilityModelCatalog)
	case "pricing_changed":
		return s.watchFingerprintChanged(ctx, target, domain.CapabilityPricing)
	case "source_changed":
		return s.watchFingerprintChanged(ctx, target, domain.CapabilityChangeWatch)
	default:
		return false, nil
	}
}

func (s *Service) watchFingerprintChanged(ctx context.Context, target domain.MonitorTarget, capability domain.Capability) (bool, error) {
	if !targetHasCapability(target, capability) && !targetHasCapability(target, domain.CapabilityChangeWatch) {
		return false, nil
	}
	current := rawString(target.Raw, "fingerprint")
	if current == "" {
		return false, nil
	}
	snapshots, err := s.store.ListRecentSnapshots(ctx, target.ID, 2)
	if err != nil {
		return false, err
	}
	if len(snapshots) < 2 {
		return false, nil
	}
	previous := rawString(snapshots[1].Raw, "fingerprint")
	return previous != "" && previous != current, nil
}

func (s *Service) changeCooldownElapsed(ctx context.Context, targetID, ruleID string, cooldownSeconds int) bool {
	if cooldownSeconds <= 0 {
		return true
	}
	alerts, err := s.store.ListAlertsForTarget(ctx, targetID, 50)
	if err != nil {
		return true
	}
	cutoff := time.Now().UTC().Add(-time.Duration(cooldownSeconds) * time.Second)
	for _, alert := range alerts {
		if alert.RuleID == ruleID && alert.OpenedAt.After(cutoff) {
			return false
		}
	}
	return true
}

func targetHasCapability(target domain.MonitorTarget, capability domain.Capability) bool {
	for _, current := range target.Capabilities {
		if current == capability {
			return true
		}
	}
	return false
}

func isChangeCondition(condition string) bool {
	switch condition {
	case "announcement_changed", "news_changed", "deprecation_changed", "group_catalog_changed", "model_catalog_changed", "pricing_changed", "source_changed":
		return true
	default:
		return false
	}
}

func rawString(raw json.RawMessage, key string) string {
	var object map[string]any
	if len(raw) == 0 {
		return ""
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return ""
	}
	if value, ok := object[key].(string); ok {
		return value
	}
	return ""
}

func alertTitle(rule domain.AlertRule, target domain.MonitorTarget) string {
	if isChangeCondition(rule.ConditionType) {
		if latest := latestWatchTitle(target.Raw); latest != "" {
			return fmt.Sprintf("%s: %s updated", rule.Severity, latest)
		}
		return fmt.Sprintf("%s: %s changed", rule.Severity, target.Name)
	}
	return fmt.Sprintf("%s: %s", rule.Severity, target.Name)
}

func alertMessage(rule domain.AlertRule, target domain.MonitorTarget) string {
	if isChangeCondition(rule.ConditionType) {
		title, summary, sourceURL, fingerprint := latestWatchInfo(target.Raw)
		return fmt.Sprintf("Rule `%s` detected `%s` on `%s`.\nLatest: %s\nSummary: %s\nSource: %s\nFingerprint: %s",
			rule.Name, rule.ConditionType, target.Name, fallback(title, target.Name), fallback(summary, "-"), fallback(sourceURL, "-"), fallback(fingerprint, "-"))
	}
	return fmt.Sprintf("Rule `%s` matched `%s`; condition `%s` threshold %.4f %s.",
		rule.Name, target.Name, rule.ConditionType, rule.ThresholdValue, rule.ThresholdUnit)
}

func latestWatchInfo(raw json.RawMessage) (title, summary, sourceURL, fingerprint string) {
	var object map[string]any
	if len(raw) == 0 {
		return "", "", "", ""
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return "", "", "", ""
	}
	sourceURL, _ = object["sourceUrl"].(string)
	fingerprint, _ = object["fingerprint"].(string)
	summary, _ = object["summary"].(string)
	if items, ok := object["items"].([]any); ok && len(items) > 0 {
		if first, ok := items[0].(map[string]any); ok {
			title, _ = first["title"].(string)
			if itemSummary, ok := first["summary"].(string); ok && itemSummary != "" {
				summary = itemSummary
			}
			if itemURL, ok := first["url"].(string); ok && itemURL != "" {
				sourceURL = itemURL
			}
		}
	}
	return title, summary, sourceURL, fingerprint
}

func latestWatchTitle(raw json.RawMessage) string {
	title, _, _, _ := latestWatchInfo(raw)
	return title
}

func fallback(value, alt string) string {
	if value != "" {
		return value
	}
	return alt
}
