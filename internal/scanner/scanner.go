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
		if !rule.Enabled || !ruleMatchesTarget(rule, target) || !conditionMet(rule, target) {
			continue
		}
		if existing, err := s.store.FindOpenAlert(ctx, target.ID, rule.ID); err == nil && existing != nil {
			continue
		}
		alert, err := s.store.CreateAlert(ctx, domain.AlertEvent{
			TargetID: target.ID,
			RuleID:   rule.ID,
			Severity: rule.Severity,
			Status:   "open",
			Title:    fmt.Sprintf("%s: %s", rule.Severity, target.Name),
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

func conditionMet(rule domain.AlertRule, target domain.MonitorTarget) bool {
	switch rule.ConditionType {
	case "balance_below":
		return target.Balance != nil && target.Balance.Amount < rule.ThresholdValue
	case "remaining_quota_below":
		return target.Quota != nil && target.Quota.Remaining != nil && *target.Quota.Remaining < rule.ThresholdValue
	case "remaining_percent_below":
		if target.Quota == nil || target.Quota.Remaining == nil || target.Quota.Total == nil || *target.Quota.Total == 0 {
			return false
		}
		return (*target.Quota.Remaining / *target.Quota.Total * 100) < rule.ThresholdValue
	case "days_until_expiry_below":
		if target.Plan == nil || target.Plan.ExpireAt == nil {
			return false
		}
		return time.Until(*target.Plan.ExpireAt).Hours()/24 < rule.ThresholdValue
	case "health_not_healthy":
		return target.Status != domain.StatusHealthy
	case "monthly_cost_above":
		return target.MonthlyCost != nil && target.MonthlyCost.Amount > rule.ThresholdValue
	default:
		return false
	}
}

func alertMessage(rule domain.AlertRule, target domain.MonitorTarget) string {
	return fmt.Sprintf("Rule `%s` matched `%s`; condition `%s` threshold %.4f %s.",
		rule.Name, target.Name, rule.ConditionType, rule.ThresholdValue, rule.ThresholdUnit)
}
