package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	appcrypto "api-monitor/internal/crypto"
	"api-monitor/internal/domain"
)

type DashboardSummary struct {
	TotalTargets    int           `json:"totalTargets"`
	HealthyTargets  int           `json:"healthyTargets"`
	WarningTargets  int           `json:"warningTargets"`
	CriticalTargets int           `json:"criticalTargets"`
	UnknownTargets  int           `json:"unknownTargets"`
	ScanSuccessRate float64       `json:"scanSuccessRate"`
	MonthlyCost     *domain.Money `json:"monthlyCost,omitempty"`
	AtRiskBalance   *domain.Money `json:"atRiskBalance,omitempty"`
	Alerts24h       int           `json:"alerts24h"`
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
	var monthlyCost sql.NullFloat64
	_ = s.db.QueryRow(ctx, `SELECT sum(monthly_cost_amount) FROM monitor_targets WHERE monthly_cost_amount IS NOT NULL`).Scan(&monthlyCost)
	if monthlyCost.Valid {
		summary.MonthlyCost = &domain.Money{Amount: monthlyCost.Float64, Currency: "USD"}
	}
	var atRisk sql.NullFloat64
	_ = s.db.QueryRow(ctx, `SELECT sum(balance_amount) FROM monitor_targets WHERE status IN ('warning','critical') AND balance_amount IS NOT NULL`).Scan(&atRisk)
	if atRisk.Valid {
		summary.AtRiskBalance = &domain.Money{Amount: atRisk.Float64, Currency: "USD"}
	}
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM alert_events WHERE opened_at >= now() - interval '24 hours'`).Scan(&summary.Alerts24h)
	return summary, nil
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
