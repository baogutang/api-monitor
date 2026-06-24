package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	appcrypto "api-monitor/internal/crypto"
	"api-monitor/internal/domain"
)

type DashboardSummary struct {
	TotalTargets     int           `json:"totalTargets"`
	HealthyTargets   int           `json:"healthyTargets"`
	WarningTargets   int           `json:"warningTargets"`
	CriticalTargets  int           `json:"criticalTargets"`
	UnknownTargets   int           `json:"unknownTargets"`
	ScanSuccessRate  float64       `json:"scanSuccessRate"`
	MonthlyCost      *domain.Money `json:"monthlyCost,omitempty"`
	TodayCost        *domain.Money `json:"todayCost,omitempty"`
	TotalBalance     *domain.Money `json:"totalBalance,omitempty"`
	AtRiskBalance    *domain.Money `json:"atRiskBalance,omitempty"`
	ActiveChannels   int           `json:"activeChannels"`
	AlertingChannels int           `json:"alertingChannels"`
	Alerts24h        int           `json:"alerts24h"`
	OpenAlerts       int           `json:"openAlerts"`
	CriticalAlerts   int           `json:"criticalAlerts"`
	RiskTargets      int           `json:"riskTargets"`
}

type DashboardTrendPoint struct {
	CapturedAt time.Time     `json:"capturedAt"`
	Balance    *domain.Money `json:"balance,omitempty"`
	Cost       *domain.Money `json:"cost,omitempty"`
}

type InstanceUsageSummary struct {
	InstanceID string        `json:"instanceId"`
	Range      string        `json:"range"`
	Cost       *domain.Money `json:"cost,omitempty"`
	Requests   int64         `json:"requests"`
	StartedAt  time.Time     `json:"startedAt"`
	EndedAt    time.Time     `json:"endedAt"`
}

func (s *Store) CreateScanRun(ctx context.Context, targetID, instanceID string) (*domain.ScanRun, error) {
	run := domain.ScanRun{ID: appcrypto.NewID("run"), TargetID: targetID, InstanceID: instanceID, Status: "running"}
	row := s.db.QueryRow(ctx, `
		INSERT INTO scan_runs(id, target_id, instance_id, status)
		VALUES($1,$2,$3,$4)
		RETURNING id, target_id, instance_id, status, started_at, finished_at, error, raw
	`, run.ID, nullableText(targetID), nullableText(instanceID), run.Status)
	return scanRun(row)
}

func (s *Store) FinishScanRun(ctx context.Context, id, status, errText string, raw json.RawMessage) error {
	if raw == nil {
		raw = json.RawMessage(`{}`)
	}
	_, err := s.db.Exec(ctx, `
		UPDATE scan_runs SET status=$2, error=$3, raw=$4, finished_at=now()
		WHERE id=$1
	`, id, status, errText, raw)
	return err
}

func (s *Store) ListScanRuns(ctx context.Context, targetID, status string, limit, offset int) ([]domain.ScanRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, target_id, instance_id, status, started_at, finished_at, error, raw
		FROM scan_runs
		WHERE ($1='' OR target_id=$1) AND ($2='' OR status=$2)
		ORDER BY started_at DESC
		LIMIT $3 OFFSET $4
	`, targetID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []domain.ScanRun
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	return runs, rows.Err()
}

func scanRun(row pgx.Row) (*domain.ScanRun, error) {
	var run domain.ScanRun
	var targetID, instanceID sql.NullString
	var finishedAt sql.NullTime
	err := row.Scan(&run.ID, &targetID, &instanceID, &run.Status, &run.StartedAt, &finishedAt, &run.Error, &run.Raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	run.TargetID = targetID.String
	run.InstanceID = instanceID.String
	run.FinishedAt = nullTimePtr(finishedAt)
	return &run, nil
}

func (s *Store) DashboardSummary(ctx context.Context) (DashboardSummary, error) {
	var summary DashboardSummary
	err := s.db.QueryRow(ctx, `
		SELECT
			count(*),
			count(*) FILTER (WHERE status='healthy'),
			count(*) FILTER (WHERE status='warning'),
			count(*) FILTER (WHERE status='critical'),
			count(*) FILTER (WHERE status='unknown')
		FROM monitor_targets
	`).Scan(&summary.TotalTargets, &summary.HealthyTargets, &summary.WarningTargets, &summary.CriticalTargets, &summary.UnknownTargets)
	if err != nil {
		return summary, err
	}
	var success, total int
	_ = s.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status='success'), count(*)
		FROM scan_runs
		WHERE started_at >= now() - interval '24 hours'
	`).Scan(&success, &total)
	if total > 0 {
		summary.ScanSuccessRate = float64(success) / float64(total)
	}
	_ = s.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE enabled = TRUE AND kind IN ('user','subscription')),
			count(*) FILTER (WHERE enabled = TRUE AND kind IN ('user','subscription') AND status IN ('warning','critical'))
		FROM monitor_targets
	`).Scan(&summary.ActiveChannels, &summary.AlertingChannels)
	var totalBalanceAmount sql.NullFloat64
	var totalBalanceCurrency sql.NullString
	_ = s.db.QueryRow(ctx, `
		SELECT balance_currency, sum(balance_amount) AS amount
		FROM monitor_targets
		WHERE enabled = TRUE
			AND kind IN ('user','subscription')
			AND balance_amount IS NOT NULL
		GROUP BY balance_currency
		ORDER BY amount DESC
		LIMIT 1
	`).Scan(&totalBalanceCurrency, &totalBalanceAmount)
	if totalBalanceAmount.Valid {
		summary.TotalBalance = roundedMoney(totalBalanceAmount.Float64, firstNonEmpty(totalBalanceCurrency.String, "USD"))
	}
	var monthlyCostAmount sql.NullFloat64
	var monthlyCostCurrency sql.NullString
	_ = s.db.QueryRow(ctx, `
		WITH per_instance AS (
			SELECT
				instance_id,
				bool_or(kind IN ('user','subscription') AND monthly_cost_amount IS NOT NULL) AS has_parent_cost
			FROM monitor_targets
			WHERE enabled = TRUE
			GROUP BY instance_id
		),
		picked AS (
			SELECT mt.monthly_cost_currency AS currency, mt.monthly_cost_amount AS amount
			FROM monitor_targets mt
			JOIN per_instance pi ON pi.instance_id = mt.instance_id
			WHERE mt.enabled = TRUE
				AND mt.monthly_cost_amount IS NOT NULL
				AND (
					(pi.has_parent_cost AND mt.kind IN ('user','subscription'))
					OR (NOT pi.has_parent_cost AND mt.kind = 'api_key')
				)
		)
		SELECT currency, sum(amount)
		FROM picked
		GROUP BY currency
		ORDER BY sum(amount) DESC
		LIMIT 1
	`).Scan(&monthlyCostCurrency, &monthlyCostAmount)
	if monthlyCostAmount.Valid {
		summary.MonthlyCost = roundedMoney(monthlyCostAmount.Float64, firstNonEmpty(monthlyCostCurrency.String, "USD"))
	}
	var todayCostAmount sql.NullFloat64
	var todayCostCurrency sql.NullString
	_ = s.db.QueryRow(ctx, `
		WITH latest_today AS (
			SELECT DISTINCT ON (target_id)
				target_id, monthly_cost_amount, monthly_cost_currency
			FROM balance_snapshots
			WHERE captured_at >= date_trunc('day', now())
				AND monthly_cost_amount IS NOT NULL
			ORDER BY target_id, captured_at DESC
		),
		earliest_today AS (
			SELECT DISTINCT ON (target_id)
				target_id, monthly_cost_amount
			FROM balance_snapshots
			WHERE captured_at >= date_trunc('day', now())
				AND monthly_cost_amount IS NOT NULL
			ORDER BY target_id, captured_at ASC
		),
		previous AS (
			SELECT DISTINCT ON (target_id)
				target_id, monthly_cost_amount
			FROM balance_snapshots
			WHERE captured_at < date_trunc('day', now())
				AND monthly_cost_amount IS NOT NULL
			ORDER BY target_id, captured_at DESC
		),
		daily AS (
			SELECT
				mt.instance_id,
				mt.kind,
				lt.monthly_cost_currency AS currency,
				GREATEST(lt.monthly_cost_amount - COALESCE(p.monthly_cost_amount, 0), 0) AS amount
			FROM latest_today lt
			JOIN earliest_today et ON et.target_id = lt.target_id
			LEFT JOIN previous p ON p.target_id = lt.target_id
			JOIN monitor_targets mt ON mt.id = lt.target_id
			WHERE mt.enabled = TRUE
		),
		per_instance AS (
			SELECT
				instance_id,
				bool_or(kind IN ('user','subscription')) AS has_parent_cost
			FROM daily
			GROUP BY instance_id
		),
		picked AS (
			SELECT daily.currency, daily.amount
			FROM daily
			JOIN per_instance pi ON pi.instance_id = daily.instance_id
			WHERE
				(pi.has_parent_cost AND daily.kind IN ('user','subscription'))
				OR (NOT pi.has_parent_cost AND daily.kind = 'api_key')
		)
		SELECT currency, sum(amount)
		FROM picked
		GROUP BY currency
		ORDER BY sum(amount) DESC
		LIMIT 1
	`).Scan(&todayCostCurrency, &todayCostAmount)
	if todayCostAmount.Valid {
		summary.TodayCost = roundedMoney(todayCostAmount.Float64, firstNonEmpty(todayCostCurrency.String, "USD"))
	}
	var atRisk sql.NullFloat64
	var atRiskCurrency sql.NullString
	_ = s.db.QueryRow(ctx, `
		SELECT balance_currency, sum(balance_amount)
		FROM monitor_targets
		WHERE kind IN ('user','subscription')
			AND status IN ('warning','critical')
			AND balance_amount IS NOT NULL
		GROUP BY balance_currency
		ORDER BY sum(balance_amount) DESC
		LIMIT 1
	`).Scan(&atRiskCurrency, &atRisk)
	if atRisk.Valid {
		summary.AtRiskBalance = roundedMoney(atRisk.Float64, firstNonEmpty(atRiskCurrency.String, "USD"))
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM alert_events WHERE opened_at >= now() - interval '24 hours'`).Scan(&summary.Alerts24h)
	_ = s.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status IN ('open','acknowledged','silenced')),
			count(*) FILTER (WHERE status IN ('open','acknowledged','silenced') AND severity IN ('critical','phone'))
		FROM alert_events
	`).Scan(&summary.OpenAlerts, &summary.CriticalAlerts)
	_ = s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM monitor_targets
		WHERE enabled = TRUE
			AND kind IN ('user','subscription','api_key')
			AND status IN ('warning','critical')
	`).Scan(&summary.RiskTargets)
	return summary, nil
}

func (s *Store) DashboardTrends(ctx context.Context, since time.Time, bucket string) ([]DashboardTrendPoint, error) {
	bucketSQL := dashboardTrendBucket(bucket)
	rows, err := s.db.Query(ctx, `
		WITH bucketed AS (
			SELECT
				`+bucketSQL+` AS bucket,
				bs.target_id,
				mt.instance_id,
				mt.kind,
				bs.captured_at,
				bs.balance_amount,
				bs.balance_currency,
				bs.monthly_cost_amount,
				bs.monthly_cost_currency,
				row_number() OVER (
					PARTITION BY `+bucketSQL+`, bs.target_id
					ORDER BY bs.captured_at DESC
				) AS rn
			FROM balance_snapshots bs
			JOIN monitor_targets mt ON mt.id = bs.target_id
			WHERE bs.captured_at >= $1
				AND mt.enabled = TRUE
		),
		latest AS (
			SELECT *
			FROM bucketed
			WHERE rn = 1
		),
		per_instance AS (
			SELECT
				bucket,
				instance_id,
				bool_or(kind IN ('user','subscription') AND monthly_cost_amount IS NOT NULL) AS has_parent_cost
			FROM latest
			GROUP BY bucket, instance_id
		),
		picked AS (
			SELECT latest.*
			FROM latest
			JOIN per_instance pi
				ON pi.bucket = latest.bucket AND pi.instance_id = latest.instance_id
			WHERE
				latest.kind IN ('user','subscription')
				OR (
					latest.monthly_cost_amount IS NOT NULL
					AND NOT pi.has_parent_cost
					AND latest.kind = 'api_key'
				)
		)
		SELECT
			bucket,
			max(balance_currency) FILTER (WHERE kind IN ('user','subscription') AND balance_amount IS NOT NULL),
			sum(balance_amount) FILTER (WHERE kind IN ('user','subscription') AND balance_amount IS NOT NULL),
			max(monthly_cost_currency) FILTER (WHERE monthly_cost_amount IS NOT NULL),
			sum(monthly_cost_amount) FILTER (WHERE monthly_cost_amount IS NOT NULL)
		FROM picked
		GROUP BY bucket
		ORDER BY bucket
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := []DashboardTrendPoint{}
	for rows.Next() {
		var point DashboardTrendPoint
		var balanceCurrency, costCurrency sql.NullString
		var balanceAmount, costAmount sql.NullFloat64
		if err := rows.Scan(&point.CapturedAt, &balanceCurrency, &balanceAmount, &costCurrency, &costAmount); err != nil {
			return nil, err
		}
		if balanceAmount.Valid {
			point.Balance = &domain.Money{Amount: balanceAmount.Float64, Currency: firstNonEmpty(balanceCurrency.String, "USD")}
		}
		if costAmount.Valid {
			point.Cost = &domain.Money{Amount: costAmount.Float64, Currency: firstNonEmpty(costCurrency.String, "USD")}
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func dashboardTrendBucket(bucket string) string {
	switch bucket {
	case "minute":
		return "date_trunc('minute', bs.captured_at)"
	case "day":
		return "date_trunc('day', bs.captured_at)"
	default:
		return "date_trunc('hour', bs.captured_at)"
	}
}

func (s *Store) InstanceUsageSummaries(ctx context.Context, rangeID string) ([]InstanceUsageSummary, error) {
	rangeID, since, until := normalizeUsageRange(rangeID)
	rows, err := s.db.Query(ctx, `
		WITH previous AS (
			SELECT DISTINCT ON (target_id)
				target_id, monthly_cost_amount, raw
			FROM balance_snapshots
			WHERE captured_at < $1
			ORDER BY target_id, captured_at DESC
		)
		SELECT
			mt.instance_id,
			mt.kind,
			mt.monthly_cost_amount,
			mt.monthly_cost_currency,
			mt.raw,
			previous.monthly_cost_amount,
			previous.raw
		FROM monitor_targets mt
		LEFT JOIN previous ON previous.target_id = mt.id
		WHERE mt.enabled = TRUE
			AND mt.kind IN ('user','subscription','api_key')
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type bucket struct {
		hasParentCost     bool
		hasParentRequests bool
		parentCost        float64
		childCost         float64
		parentRequests    int64
		childRequests     int64
		currency          string
	}
	buckets := map[string]*bucket{}
	for rows.Next() {
		var instanceID string
		var kind domain.TargetKind
		var currentCost, previousCost sql.NullFloat64
		var currentCurrency sql.NullString
		var currentRaw, previousRaw json.RawMessage
		if err := rows.Scan(&instanceID, &kind, &currentCost, &currentCurrency, &currentRaw, &previousCost, &previousRaw); err != nil {
			return nil, err
		}
		currentCostValue, costOK := costValue(currentCost, currentRaw)
		previousCostValue := 0.0
		if previousCost.Valid {
			previousCostValue = previousCost.Float64
		} else if value, ok := rawNumber(previousRaw, usageCostKeys()...); ok {
			previousCostValue = value
		}
		costDelta := 0.0
		if costOK {
			costDelta = currentCostValue - previousCostValue
			if costDelta < 0 {
				costDelta = 0
			}
		}

		currentRequests, requestOK := rawNumber(currentRaw, requestCountKeys()...)
		previousRequests := 0.0
		if value, ok := rawNumber(previousRaw, requestCountKeys()...); ok {
			previousRequests = value
		}
		requestDelta := int64(0)
		if requestOK {
			delta := currentRequests - previousRequests
			if delta > 0 {
				requestDelta = int64(math.Round(delta))
			}
		}

		entry := buckets[instanceID]
		if entry == nil {
			entry = &bucket{}
			buckets[instanceID] = entry
		}
		if entry.currency == "" {
			entry.currency = firstNonEmpty(currentCurrency.String, rawCurrency(currentRaw), "USD")
		}
		if kind == domain.TargetUser || kind == domain.TargetSubscription {
			if costOK {
				entry.hasParentCost = true
				entry.parentCost += costDelta
			}
			if requestOK {
				entry.hasParentRequests = true
				entry.parentRequests += requestDelta
			}
			continue
		}
		entry.childCost += costDelta
		entry.childRequests += requestDelta
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]InstanceUsageSummary, 0, len(buckets))
	for instanceID, entry := range buckets {
		amount := entry.childCost
		if entry.hasParentCost {
			amount = entry.parentCost
		}
		requests := entry.childRequests
		if entry.hasParentRequests {
			requests = entry.parentRequests
		}
		summary := InstanceUsageSummary{
			InstanceID: instanceID,
			Range:      rangeID,
			Requests:   requests,
			StartedAt:  since,
			EndedAt:    until,
		}
		if amount > 0 {
			summary.Cost = roundedMoney(amount, firstNonEmpty(entry.currency, "USD"))
		}
		out = append(out, summary)
	}
	return out, nil
}

func normalizeUsageRange(rangeID string) (string, time.Time, time.Time) {
	now := time.Now().UTC()
	switch rangeID {
	case "today":
		return "today", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), now
	case "7d":
		return "7d", now.Add(-7 * 24 * time.Hour), now
	case "30d":
		return "30d", now.Add(-30 * 24 * time.Hour), now
	default:
		return "24h", now.Add(-24 * time.Hour), now
	}
}

func costValue(column sql.NullFloat64, raw json.RawMessage) (float64, bool) {
	if column.Valid {
		return column.Float64, true
	}
	if value, ok := rawNumber(raw, usageCostKeys()...); ok {
		return value, true
	}
	return 0, false
}

func usageCostKeys() []string {
	return []string{
		"monthly_cost",
		"monthlyCost",
		"month_cost",
		"monthCost",
		"usage_cost",
		"usageCost",
		"total_cost",
		"totalCost",
		"used_amount",
		"usedAmount",
		"used_usd",
		"usedUSD",
	}
}

func requestCountKeys() []string {
	return []string{
		"request_count",
		"requestCount",
		"requests",
		"total_requests",
		"totalRequests",
		"api_requests",
		"apiRequests",
		"request_num",
		"requestNum",
	}
}

func rawCurrency(raw json.RawMessage) string {
	if value, ok := rawStringValue(raw, "currency", "balance_currency", "monthly_cost_currency"); ok {
		return value
	}
	return ""
}

func rawNumber(raw json.RawMessage, keys ...string) (float64, bool) {
	values := rawLookupMaps(raw)
	for _, object := range values {
		for _, key := range keys {
			value, ok := object[key]
			if !ok {
				continue
			}
			switch typed := value.(type) {
			case float64:
				if math.IsNaN(typed) || math.IsInf(typed, 0) {
					continue
				}
				return typed, true
			case int:
				return float64(typed), true
			case int64:
				return float64(typed), true
			case json.Number:
				if parsed, err := typed.Float64(); err == nil {
					return parsed, true
				}
			case string:
				if parsed, err := strconvParseFloat(typed); err == nil {
					return parsed, true
				}
			}
		}
	}
	return 0, false
}

func rawStringValue(raw json.RawMessage, keys ...string) (string, bool) {
	values := rawLookupMaps(raw)
	for _, object := range values {
		for _, key := range keys {
			value, ok := object[key]
			if !ok {
				continue
			}
			if text, ok := value.(string); ok && text != "" {
				return text, true
			}
		}
	}
	return "", false
}

func rawLookupMaps(raw json.RawMessage) []map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	out := []map[string]any{root}
	if data, ok := root["data"].(map[string]any); ok {
		out = append(out, data)
	}
	if payload, ok := root["payload"].(map[string]any); ok {
		out = append(out, payload)
	}
	return out
}

func strconvParseFloat(value string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(value), 64)
}

func roundedMoney(amount float64, currency string) *domain.Money {
	return &domain.Money{Amount: math.Round(amount*100) / 100, Currency: firstNonEmpty(currency, "USD")}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Store) GetSettings(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.db.Query(ctx, `SELECT key, value FROM system_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var key string
		var value json.RawMessage
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}

func (s *Store) UpsertSettings(ctx context.Context, values map[string]json.RawMessage) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for key, value := range values {
		if len(value) == 0 {
			value = json.RawMessage(`null`)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO system_settings(key, value)
			VALUES($1,$2)
			ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value, updated_at=now()
		`, key, value); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Audit(ctx context.Context, actorID, action, resourceType, resourceID string, detail any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		raw = []byte(`{}`)
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO audit_logs(id, actor_user_id, action, resource_type, resource_id, detail)
		VALUES($1,$2,$3,$4,$5,$6)
	`, appcrypto.NewID("aud"), nullableText(actorID), action, resourceType, resourceID, raw)
	return err
}
