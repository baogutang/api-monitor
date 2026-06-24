package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	appcrypto "api-monitor/internal/crypto"
	"api-monitor/internal/domain"
)

func (s *Store) ListAlertRules(ctx context.Context) ([]domain.AlertRule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, scope_type, scope_value, severity, condition_type, threshold_value, threshold_unit,
			sustain_count, cooldown_seconds, notification_channel_ids, enabled, created_at, updated_at
		FROM alert_rules
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []domain.AlertRule
	for rows.Next() {
		rule, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Store) UpsertAlertRule(ctx context.Context, rule domain.AlertRule) (*domain.AlertRule, error) {
	if rule.ID == "" {
		rule.ID = appcrypto.NewID("rul")
	}
	if rule.ScopeType == "" {
		rule.ScopeType = "global"
	}
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	if rule.SustainCount <= 0 {
		rule.SustainCount = 1
	}
	if rule.CooldownSeconds <= 0 {
		rule.CooldownSeconds = 1800
	}
	channels, err := json.Marshal(rule.NotificationChannelIDs)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO alert_rules(id, name, scope_type, scope_value, severity, condition_type, threshold_value,
			threshold_unit, sustain_count, cooldown_seconds, notification_channel_ids, enabled)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT(id) DO UPDATE SET
			name=EXCLUDED.name,
			scope_type=EXCLUDED.scope_type,
			scope_value=EXCLUDED.scope_value,
			severity=EXCLUDED.severity,
			condition_type=EXCLUDED.condition_type,
			threshold_value=EXCLUDED.threshold_value,
			threshold_unit=EXCLUDED.threshold_unit,
			sustain_count=EXCLUDED.sustain_count,
			cooldown_seconds=EXCLUDED.cooldown_seconds,
			notification_channel_ids=EXCLUDED.notification_channel_ids,
			enabled=EXCLUDED.enabled,
			updated_at=now()
		RETURNING id, name, scope_type, scope_value, severity, condition_type, threshold_value, threshold_unit,
			sustain_count, cooldown_seconds, notification_channel_ids, enabled, created_at, updated_at
	`, rule.ID, rule.Name, rule.ScopeType, rule.ScopeValue, rule.Severity, rule.ConditionType, rule.ThresholdValue,
		rule.ThresholdUnit, rule.SustainCount, rule.CooldownSeconds, channels, rule.Enabled)
	ruleOut, err := scanAlertRule(row)
	if err != nil {
		return nil, err
	}
	return &ruleOut, nil
}

func (s *Store) DeleteAlertRule(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM alert_rules WHERE id=$1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanAlertRule(row pgx.Row) (domain.AlertRule, error) {
	var rule domain.AlertRule
	var channels []byte
	err := row.Scan(&rule.ID, &rule.Name, &rule.ScopeType, &rule.ScopeValue, &rule.Severity, &rule.ConditionType,
		&rule.ThresholdValue, &rule.ThresholdUnit, &rule.SustainCount, &rule.CooldownSeconds, &channels,
		&rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return rule, ErrNotFound
	}
	if err != nil {
		return rule, err
	}
	_ = json.Unmarshal(channels, &rule.NotificationChannelIDs)
	return rule, nil
}

func (s *Store) CreateAlert(ctx context.Context, event domain.AlertEvent) (*domain.AlertEvent, error) {
	if event.ID == "" {
		event.ID = appcrypto.NewID("alt")
	}
	if event.Status == "" {
		event.Status = "open"
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO alert_events(id, target_id, rule_id, severity, status, title, message)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, target_id, rule_id, severity, status, title, message, opened_at, resolved_at,
			acknowledged_at, silence_until
	`, event.ID, nullableText(event.TargetID), nullableText(event.RuleID), event.Severity, event.Status, event.Title, event.Message)
	return scanAlertEvent(row)
}

func (s *Store) FindOpenAlert(ctx context.Context, targetID, ruleID string) (*domain.AlertEvent, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, target_id, rule_id, severity, status, title, message, opened_at, resolved_at,
			acknowledged_at, silence_until
		FROM alert_events
		WHERE target_id=$1 AND rule_id=$2 AND status IN ('open', 'acknowledged', 'silenced')
		ORDER BY opened_at DESC
		LIMIT 1
	`, targetID, ruleID)
	return scanAlertEvent(row)
}

func (s *Store) ListAlerts(ctx context.Context, status, severity string, limit, offset int) ([]domain.AlertEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `
		SELECT id, target_id, rule_id, severity, status, title, message, opened_at, resolved_at,
			acknowledged_at, silence_until
		FROM alert_events
		WHERE ($1='' OR status=$1) AND ($2='' OR severity=$2)
		ORDER BY opened_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := s.db.Query(ctx, query, status, severity, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var alerts []domain.AlertEvent
	for rows.Next() {
		alert, err := scanAlertEvent(rows)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, *alert)
	}
	return alerts, rows.Err()
}

func (s *Store) UpdateAlertStatus(ctx context.Context, id, status string, silenceUntil *time.Time) (*domain.AlertEvent, error) {
	now := time.Now().UTC()
	row := s.db.QueryRow(ctx, `
		UPDATE alert_events SET
			status=$2,
			acknowledged_at=CASE WHEN $2='acknowledged' THEN $3 ELSE acknowledged_at END,
			resolved_at=CASE WHEN $2='resolved' THEN $3 ELSE resolved_at END,
			silence_until=$4,
			updated_at=$3
		WHERE id=$1
		RETURNING id, target_id, rule_id, severity, status, title, message, opened_at, resolved_at,
			acknowledged_at, silence_until
	`, id, status, now, silenceUntil)
	return scanAlertEvent(row)
}

func scanAlertEvent(row pgx.Row) (*domain.AlertEvent, error) {
	var alert domain.AlertEvent
	var targetID, ruleID sql.NullString
	var resolvedAt, ackAt, silenceUntil sql.NullTime
	err := row.Scan(&alert.ID, &targetID, &ruleID, &alert.Severity, &alert.Status, &alert.Title,
		&alert.Message, &alert.OpenedAt, &resolvedAt, &ackAt, &silenceUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	alert.TargetID = targetID.String
	alert.RuleID = ruleID.String
	alert.ResolvedAt = nullTimePtr(resolvedAt)
	alert.AcknowledgedAt = nullTimePtr(ackAt)
	alert.SilenceUntil = nullTimePtr(silenceUntil)
	return &alert, nil
}

func (s *Store) ListNotificationChannels(ctx context.Context) ([]domain.NotificationChannel, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, type, enabled, settings, secret_fingerprint, created_at, updated_at
		FROM notification_channels
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []domain.NotificationChannel
	for rows.Next() {
		channel, err := scanNotificationChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (s *Store) GetNotificationChannel(ctx context.Context, id string, includeSecret bool) (*domain.NotificationChannel, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, name, type, enabled, settings, secret_fingerprint, secret_ciphertext, created_at, updated_at
		FROM notification_channels WHERE id=$1
	`, id)
	channel, ciphertext, err := scanNotificationChannelSecret(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if includeSecret && ciphertext != "" {
		plain, err := s.secrets.Decrypt(ciphertext)
		if err != nil {
			return nil, err
		}
		channel.SecretValue = string(plain)
	}
	return channel, nil
}

func (s *Store) UpsertNotificationChannel(ctx context.Context, channel domain.NotificationChannel) (*domain.NotificationChannel, error) {
	if channel.ID == "" {
		channel.ID = appcrypto.NewID("ntf")
	}
	if channel.Settings == nil {
		channel.Settings = json.RawMessage(`{}`)
	}
	ciphertext := ""
	fingerprint := channel.SecretFingerprint
	if channel.SecretValue != "" {
		var err error
		ciphertext, err = s.secrets.Encrypt([]byte(channel.SecretValue))
		if err != nil {
			return nil, err
		}
		fingerprint = appcrypto.Fingerprint(channel.SecretValue)
	}
	var row pgx.Row
	if channel.SecretValue != "" {
		row = s.db.QueryRow(ctx, `
			INSERT INTO notification_channels(id, name, type, enabled, settings, secret_ciphertext, secret_fingerprint)
			VALUES($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT(id) DO UPDATE SET
				name=EXCLUDED.name,
				type=EXCLUDED.type,
				enabled=EXCLUDED.enabled,
				settings=EXCLUDED.settings,
				secret_ciphertext=EXCLUDED.secret_ciphertext,
				secret_fingerprint=EXCLUDED.secret_fingerprint,
				updated_at=now()
			RETURNING id, name, type, enabled, settings, secret_fingerprint, created_at, updated_at
		`, channel.ID, channel.Name, channel.Type, channel.Enabled, channel.Settings, ciphertext, fingerprint)
	} else {
		row = s.db.QueryRow(ctx, `
			INSERT INTO notification_channels(id, name, type, enabled, settings, secret_fingerprint)
			VALUES($1,$2,$3,$4,$5,$6)
			ON CONFLICT(id) DO UPDATE SET
				name=EXCLUDED.name,
				type=EXCLUDED.type,
				enabled=EXCLUDED.enabled,
				settings=EXCLUDED.settings,
				updated_at=now()
			RETURNING id, name, type, enabled, settings, secret_fingerprint, created_at, updated_at
		`, channel.ID, channel.Name, channel.Type, channel.Enabled, channel.Settings, fingerprint)
	}
	out, err := scanNotificationChannel(row)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) DeleteNotificationChannel(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM notification_channels WHERE id=$1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanNotificationChannel(row pgx.Row) (domain.NotificationChannel, error) {
	var channel domain.NotificationChannel
	err := row.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Enabled, &channel.Settings,
		&channel.SecretFingerprint, &channel.CreatedAt, &channel.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return channel, ErrNotFound
	}
	return channel, err
}

func scanNotificationChannelSecret(row pgx.Row) (*domain.NotificationChannel, string, error) {
	var channel domain.NotificationChannel
	var ciphertext string
	err := row.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Enabled, &channel.Settings,
		&channel.SecretFingerprint, &ciphertext, &channel.CreatedAt, &channel.UpdatedAt)
	if err != nil {
		return nil, "", err
	}
	return &channel, ciphertext, nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
