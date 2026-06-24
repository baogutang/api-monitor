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
}

type DashboardTrendPoint struct {
	CapturedAt time.Time     `json:"capturedAt"`
	Balance    *domain.Money `json:"balance,omitempty"`
	Cost       *domain.Money `json:"cost,omitempty"`
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
		summary.TotalBalance = &domain.Money{Amount: totalBalanceAmount.Float64, Currency: firstNonEmpty(totalBalanceCurrency.String, "USD")}
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
		summary.MonthlyCost = &domain.Money{Amount: monthlyCostAmount.Float64, Currency: firstNonEmpty(monthlyCostCurrency.String, "USD")}
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
				GREATEST(lt.monthly_cost_amount - COALESCE(p.monthly_cost_amount, et.monthly_cost_amount), 0) AS amount
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
		summary.TodayCost = &domain.Money{Amount: todayCostAmount.Float64, Currency: firstNonEmpty(todayCostCurrency.String, "USD")}
	}
	var atRisk sql.NullFloat64
	_ = s.db.QueryRow(ctx, `
		SELECT sum(balance_amount)
		FROM monitor_targets
		WHERE kind IN ('user','subscription')
			AND status IN ('warning','critical')
			AND balance_amount IS NOT NULL
	`).Scan(&atRisk)
	if atRisk.Valid {
		summary.AtRiskBalance = &domain.Money{Amount: atRisk.Float64, Currency: "USD"}
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM alert_events WHERE opened_at >= now() - interval '24 hours'`).Scan(&summary.Alerts24h)
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
