package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	appcrypto "api-monitor/internal/crypto"
	"api-monitor/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db      *pgxpool.Pool
	secrets appcrypto.Service
}

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type ListOptions struct {
	Limit  int
	Offset int
}

type TargetFilter struct {
	ProviderKind string
	Status       string
	GroupName    string
	Query        string
	Limit        int
	Offset       int
}

func New(db *pgxpool.Pool, secrets appcrypto.Service) *Store {
	return &Store{db: db, secrets: secrets}
}

func (s *Store) DB() *pgxpool.Pool {
	return s.db
}

func (s *Store) HasUsers(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users)").Scan(&exists)
	return exists, err
}

func (s *Store) CreateUser(ctx context.Context, email, name, passwordHash, role string) (*User, error) {
	user := &User{
		ID:           appcrypto.NewID("usr"),
		Email:        strings.ToLower(strings.TrimSpace(email)),
		Name:         strings.TrimSpace(name),
		PasswordHash: passwordHash,
		Role:         role,
	}
	err := s.db.QueryRow(ctx, `
		INSERT INTO users(id, email, name, password_hash, role)
		VALUES($1, $2, $3, $4, $5)
		RETURNING created_at, updated_at
	`, user.ID, user.Email, user.Name, user.PasswordHash, user.Role).Scan(&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, email, name, password_hash, role, created_at, updated_at
		FROM users WHERE email=$1
	`, strings.ToLower(strings.TrimSpace(email)))
	return scanUser(row)
}

func (s *Store) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, email, name, password_hash, role, created_at, updated_at
		FROM users WHERE id=$1
	`, id)
	return scanUser(row)
}

func scanUser(row pgx.Row) (*User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) ListInstances(ctx context.Context) ([]domain.Instance, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, provider_kind, base_url, group_name, enabled, scan_interval_seconds,
			capabilities, settings, credential_type, credential_fingerprint, created_at, updated_at
		FROM instances
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var instances []domain.Instance
	for rows.Next() {
		instance, err := scanInstancePublic(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func (s *Store) GetInstance(ctx context.Context, id string, includeCredential bool) (*domain.Instance, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, name, provider_kind, base_url, group_name, enabled, scan_interval_seconds,
			capabilities, settings, credential_type, credential_fingerprint, credential_ciphertext, created_at, updated_at
		FROM instances
		WHERE id=$1
	`, id)
	instance, ciphertext, err := scanInstanceWithSecret(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if includeCredential && ciphertext != "" {
		plain, err := s.secrets.Decrypt(ciphertext)
		if err != nil {
			return nil, err
		}
		var cred domain.Credential
		if err := json.Unmarshal(plain, &cred); err != nil {
			return nil, err
		}
		instance.Credential = &cred
	}
	return instance, nil
}

func (s *Store) UpsertInstance(ctx context.Context, in domain.Instance, credential *domain.Credential) (*domain.Instance, error) {
	now := time.Now().UTC()
	if in.ID == "" {
		in.ID = appcrypto.NewID("ins")
	}
	if in.ScanIntervalSeconds <= 0 {
		in.ScanIntervalSeconds = 60
	}
	if in.Settings == nil {
		in.Settings = json.RawMessage(`{}`)
	}
	capabilities, err := json.Marshal(in.Capabilities)
	if err != nil {
		return nil, err
	}
	credentialType := in.CredentialType
	ciphertext := ""
	fingerprint := in.CredentialFingerprint
	if credential != nil {
		credentialType = credential.Type
		plain, err := json.Marshal(credential)
		if err != nil {
			return nil, err
		}
		ciphertext, err = s.secrets.Encrypt(plain)
		if err != nil {
			return nil, err
		}
		fingerprint = credentialFingerprint(credential)
	}

	var row pgx.Row
	if credential != nil {
		row = s.db.QueryRow(ctx, `
			INSERT INTO instances(id, name, provider_kind, base_url, group_name, enabled, scan_interval_seconds,
				capabilities, settings, credential_type, credential_ciphertext, credential_fingerprint, created_at, updated_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$13)
			ON CONFLICT(id) DO UPDATE SET
				name=EXCLUDED.name,
				provider_kind=EXCLUDED.provider_kind,
				base_url=EXCLUDED.base_url,
				group_name=EXCLUDED.group_name,
				enabled=EXCLUDED.enabled,
				scan_interval_seconds=EXCLUDED.scan_interval_seconds,
				capabilities=EXCLUDED.capabilities,
				settings=EXCLUDED.settings,
				credential_type=EXCLUDED.credential_type,
				credential_ciphertext=EXCLUDED.credential_ciphertext,
				credential_fingerprint=EXCLUDED.credential_fingerprint,
				updated_at=EXCLUDED.updated_at
			RETURNING id, name, provider_kind, base_url, group_name, enabled, scan_interval_seconds,
				capabilities, settings, credential_type, credential_fingerprint, created_at, updated_at
		`, in.ID, in.Name, in.ProviderKind, in.BaseURL, in.GroupName, in.Enabled, in.ScanIntervalSeconds,
			capabilities, in.Settings, credentialType, ciphertext, fingerprint, now)
	} else {
		row = s.db.QueryRow(ctx, `
			INSERT INTO instances(id, name, provider_kind, base_url, group_name, enabled, scan_interval_seconds,
				capabilities, settings, credential_type, credential_fingerprint, created_at, updated_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
			ON CONFLICT(id) DO UPDATE SET
				name=EXCLUDED.name,
				provider_kind=EXCLUDED.provider_kind,
				base_url=EXCLUDED.base_url,
				group_name=EXCLUDED.group_name,
				enabled=EXCLUDED.enabled,
				scan_interval_seconds=EXCLUDED.scan_interval_seconds,
				capabilities=EXCLUDED.capabilities,
				settings=EXCLUDED.settings,
				updated_at=EXCLUDED.updated_at
			RETURNING id, name, provider_kind, base_url, group_name, enabled, scan_interval_seconds,
				capabilities, settings, credential_type, credential_fingerprint, created_at, updated_at
		`, in.ID, in.Name, in.ProviderKind, in.BaseURL, in.GroupName, in.Enabled, in.ScanIntervalSeconds,
			capabilities, in.Settings, credentialType, fingerprint, now)
	}
	result, err := scanInstancePublic(row)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Store) DeleteInstance(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM instances WHERE id=$1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func credentialFingerprint(cred *domain.Credential) string {
	parts := []string{cred.Type, cred.Value, cred.Username, cred.Password}
	if len(cred.JSON) > 0 {
		data, _ := json.Marshal(cred.JSON)
		parts = append(parts, string(data))
	}
	return appcrypto.Fingerprint(strings.Join(parts, "|"))
}

func scanInstancePublic(row pgx.Row) (domain.Instance, error) {
	var instance domain.Instance
	var caps []byte
	err := row.Scan(&instance.ID, &instance.Name, &instance.ProviderKind, &instance.BaseURL, &instance.GroupName,
		&instance.Enabled, &instance.ScanIntervalSeconds, &caps, &instance.Settings, &instance.CredentialType,
		&instance.CredentialFingerprint, &instance.CreatedAt, &instance.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return instance, ErrNotFound
	}
	if err != nil {
		return instance, err
	}
	_ = json.Unmarshal(caps, &instance.Capabilities)
	return instance, nil
}

func scanInstanceWithSecret(row pgx.Row) (*domain.Instance, string, error) {
	var instance domain.Instance
	var caps []byte
	var ciphertext string
	err := row.Scan(&instance.ID, &instance.Name, &instance.ProviderKind, &instance.BaseURL, &instance.GroupName,
		&instance.Enabled, &instance.ScanIntervalSeconds, &caps, &instance.Settings, &instance.CredentialType,
		&instance.CredentialFingerprint, &ciphertext, &instance.CreatedAt, &instance.UpdatedAt)
	if err != nil {
		return nil, "", err
	}
	_ = json.Unmarshal(caps, &instance.Capabilities)
	return &instance, ciphertext, nil
}

func (s *Store) UpsertTarget(ctx context.Context, target domain.MonitorTarget) (*domain.MonitorTarget, error) {
	if target.ID == "" {
		target.ID = appcrypto.NewID("tgt")
	}
	if target.Status == "" {
		target.Status = domain.StatusUnknown
	}
	caps, err := json.Marshal(target.Capabilities)
	if err != nil {
		return nil, err
	}
	if target.Raw == nil {
		target.Raw = json.RawMessage(`{}`)
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO monitor_targets(id, instance_id, provider_kind, kind, name, external_id, group_name,
			key_fingerprint, capabilities, status, balance_amount, balance_currency, quota_used, quota_total,
			quota_remaining, quota_unit, plan_name, plan_renew_at, plan_expire_at, monthly_cost_amount,
			monthly_cost_currency, raw, last_scan_at, next_scan_at, risk_score, enabled)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
		ON CONFLICT(instance_id, kind, external_id) DO UPDATE SET
			name=EXCLUDED.name,
			group_name=EXCLUDED.group_name,
			key_fingerprint=EXCLUDED.key_fingerprint,
			capabilities=EXCLUDED.capabilities,
			status=CASE WHEN EXCLUDED.status='unknown' THEN monitor_targets.status ELSE EXCLUDED.status END,
			balance_amount=EXCLUDED.balance_amount,
			balance_currency=EXCLUDED.balance_currency,
			quota_used=EXCLUDED.quota_used,
			quota_total=EXCLUDED.quota_total,
			quota_remaining=EXCLUDED.quota_remaining,
			quota_unit=EXCLUDED.quota_unit,
			plan_name=EXCLUDED.plan_name,
			plan_renew_at=EXCLUDED.plan_renew_at,
			plan_expire_at=EXCLUDED.plan_expire_at,
			monthly_cost_amount=EXCLUDED.monthly_cost_amount,
			monthly_cost_currency=EXCLUDED.monthly_cost_currency,
			raw=EXCLUDED.raw,
			last_scan_at=COALESCE(EXCLUDED.last_scan_at, monitor_targets.last_scan_at),
			next_scan_at=EXCLUDED.next_scan_at,
			risk_score=CASE WHEN EXCLUDED.status='unknown' THEN monitor_targets.risk_score ELSE EXCLUDED.risk_score END,
			enabled=EXCLUDED.enabled,
			updated_at=now()
		RETURNING id, instance_id, provider_kind, kind, name, external_id, group_name, key_fingerprint,
			capabilities, status, balance_amount, balance_currency, quota_used, quota_total, quota_remaining,
			quota_unit, plan_name, plan_renew_at, plan_expire_at, monthly_cost_amount, monthly_cost_currency,
			raw, last_scan_at, next_scan_at, risk_score, enabled, created_at, updated_at
	`, target.ID, target.InstanceID, target.ProviderKind, target.Kind, target.Name, target.ExternalID, target.GroupName,
		target.KeyFingerprint, caps, target.Status, moneyAmount(target.Balance), moneyCurrency(target.Balance),
		quotaUsed(target.Quota), quotaTotal(target.Quota), quotaRemaining(target.Quota), quotaUnit(target.Quota),
		planName(target.Plan), planRenew(target.Plan), planExpire(target.Plan), moneyAmount(target.MonthlyCost),
		moneyCurrency(target.MonthlyCost), target.Raw, target.LastScanAt, target.NextScanAt, target.RiskScore, target.Enabled)
	return scanTarget(row)
}

func (s *Store) GetTarget(ctx context.Context, id string) (*domain.MonitorTarget, error) {
	row := s.db.QueryRow(ctx, targetSelectSQL("WHERE id=$1"), id)
	return scanTarget(row)
}

func (s *Store) UpdateTargetEditable(ctx context.Context, id string, name *string, groupName *string, enabled *bool) (*domain.MonitorTarget, error) {
	current, err := s.GetTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	if name != nil {
		current.Name = *name
	}
	if groupName != nil {
		current.GroupName = *groupName
	}
	if enabled != nil {
		current.Enabled = *enabled
	}
	row := s.db.QueryRow(ctx, `
		UPDATE monitor_targets
		SET name=$2, group_name=$3, enabled=$4, updated_at=now()
		WHERE id=$1
		RETURNING id, instance_id, provider_kind, kind, name, external_id, group_name, key_fingerprint,
			capabilities, status, balance_amount, balance_currency, quota_used, quota_total, quota_remaining,
			quota_unit, plan_name, plan_renew_at, plan_expire_at, monthly_cost_amount, monthly_cost_currency,
			raw, last_scan_at, next_scan_at, risk_score, enabled, created_at, updated_at
	`, current.ID, current.Name, current.GroupName, current.Enabled)
	return scanTarget(row)
}

func (s *Store) ListTargets(ctx context.Context, filter TargetFilter) ([]domain.MonitorTarget, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	var clauses []string
	var args []any
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if filter.ProviderKind != "" {
		add("provider_kind=$%d", filter.ProviderKind)
	}
	if filter.Status != "" {
		add("status=$%d", filter.Status)
	}
	if filter.GroupName != "" {
		add("group_name=$%d", filter.GroupName)
	}
	if filter.Query != "" {
		args = append(args, "%"+strings.ToLower(filter.Query)+"%")
		idx := len(args)
		clauses = append(clauses, fmt.Sprintf("(lower(name) LIKE $%d OR lower(external_id) LIKE $%d OR lower(key_fingerprint) LIKE $%d)", idx, idx, idx))
	}
	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	args = append(args, filter.Limit, filter.Offset)
	sqlText := targetSelectSQL(where) + fmt.Sprintf(" ORDER BY risk_score DESC, updated_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))
	rows, err := s.db.Query(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []domain.MonitorTarget
	for rows.Next() {
		target, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *target)
	}
	return targets, rows.Err()
}

func (s *Store) DueTargets(ctx context.Context, limit int) ([]domain.MonitorTarget, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, targetSelectSQL(`
		WHERE enabled=TRUE AND (next_scan_at IS NULL OR next_scan_at <= now())
	`)+" ORDER BY COALESCE(next_scan_at, created_at) ASC LIMIT $1", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []domain.MonitorTarget
	for rows.Next() {
		target, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *target)
	}
	return targets, rows.Err()
}

func (s *Store) UpdateTargetScanResult(ctx context.Context, target domain.MonitorTarget, result domain.ScanResult, intervalSeconds int) (*domain.MonitorTarget, error) {
	now := time.Now().UTC()
	next := now.Add(time.Duration(intervalSeconds) * time.Second)
	target.Status = result.Status
	if result.Balance != nil {
		target.Balance = result.Balance
	}
	if result.Quota != nil {
		target.Quota = result.Quota
	}
	if result.Plan != nil {
		target.Plan = result.Plan
	}
	if result.MonthlyCost != nil {
		target.MonthlyCost = result.MonthlyCost
	}
	if len(result.Capabilities) > 0 {
		target.Capabilities = result.Capabilities
	}
	if len(result.Raw) > 0 && result.Error == "" {
		target.Raw = result.Raw
	}
	target.LastScanAt = &now
	target.NextScanAt = &next
	target.RiskScore = RiskScore(target)
	updated, err := s.UpsertTarget(ctx, target)
	if err != nil {
		return nil, err
	}
	if err := s.CreateSnapshot(ctx, *updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func RiskScore(target domain.MonitorTarget) int {
	score := 0
	switch target.Status {
	case domain.StatusCritical:
		score += 80
	case domain.StatusWarning:
		score += 50
	case domain.StatusUnknown:
		score += 20
	}
	if target.Balance != nil {
		if target.Balance.Amount <= 0 {
			score += 30
		} else if target.Balance.Amount < 10 {
			score += 20
		} else if target.Balance.Amount < 50 {
			score += 10
		}
	}
	if target.Quota != nil && target.Quota.Remaining != nil {
		if *target.Quota.Remaining <= 0 {
			score += 30
		} else if *target.Quota.Remaining < 100 {
			score += 15
		}
	}
	if score > 100 {
		return 100
	}
	return score
}

func (s *Store) CreateSnapshot(ctx context.Context, target domain.MonitorTarget) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO balance_snapshots(id, target_id, status, balance_amount, balance_currency, quota_used,
			quota_total, quota_remaining, quota_unit, monthly_cost_amount, monthly_cost_currency, raw)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, appcrypto.NewID("snp"), target.ID, target.Status, moneyAmount(target.Balance), moneyCurrency(target.Balance),
		quotaUsed(target.Quota), quotaTotal(target.Quota), quotaRemaining(target.Quota), quotaUnit(target.Quota),
		moneyAmount(target.MonthlyCost), moneyCurrency(target.MonthlyCost), target.Raw)
	return err
}

func (s *Store) ListSnapshots(ctx context.Context, targetID string, since time.Time) ([]domain.Snapshot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, target_id, captured_at, status, balance_amount, balance_currency, quota_used, quota_total,
			quota_remaining, quota_unit, monthly_cost_amount, monthly_cost_currency, raw
		FROM balance_snapshots
		WHERE target_id=$1 AND captured_at >= $2
		ORDER BY captured_at ASC
	`, targetID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []domain.Snapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func (s *Store) ListRecentSnapshots(ctx context.Context, targetID string, limit int) ([]domain.Snapshot, error) {
	if limit <= 0 || limit > 20 {
		limit = 2
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, target_id, captured_at, status, balance_amount, balance_currency, quota_used, quota_total,
			quota_remaining, quota_unit, monthly_cost_amount, monthly_cost_currency, raw
		FROM balance_snapshots
		WHERE target_id=$1
		ORDER BY captured_at DESC
		LIMIT $2
	`, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []domain.Snapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func targetSelectSQL(where string) string {
	return `SELECT id, instance_id, provider_kind, kind, name, external_id, group_name, key_fingerprint,
		capabilities, status, balance_amount, balance_currency, quota_used, quota_total, quota_remaining,
		quota_unit, plan_name, plan_renew_at, plan_expire_at, monthly_cost_amount, monthly_cost_currency,
		raw, last_scan_at, next_scan_at, risk_score, enabled, created_at, updated_at
		FROM monitor_targets ` + where
}

func scanTarget(row pgx.Row) (*domain.MonitorTarget, error) {
	var target domain.MonitorTarget
	var caps []byte
	var balanceAmount, quotaUsedValue, quotaTotalValue, quotaRemainingValue, monthlyCostAmount sql.NullFloat64
	var balanceCurrency, quotaUnitValue, planNameValue, monthlyCostCurrency sql.NullString
	var renewAt, expireAt, lastScanAt, nextScanAt sql.NullTime
	err := row.Scan(&target.ID, &target.InstanceID, &target.ProviderKind, &target.Kind, &target.Name, &target.ExternalID,
		&target.GroupName, &target.KeyFingerprint, &caps, &target.Status, &balanceAmount, &balanceCurrency,
		&quotaUsedValue, &quotaTotalValue, &quotaRemainingValue, &quotaUnitValue, &planNameValue, &renewAt, &expireAt,
		&monthlyCostAmount, &monthlyCostCurrency, &target.Raw, &lastScanAt, &nextScanAt, &target.RiskScore,
		&target.Enabled, &target.CreatedAt, &target.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(caps, &target.Capabilities)
	if balanceAmount.Valid {
		target.Balance = &domain.Money{Amount: balanceAmount.Float64, Currency: balanceCurrency.String}
	}
	if quotaUsedValue.Valid || quotaTotalValue.Valid || quotaRemainingValue.Valid || quotaUnitValue.Valid {
		target.Quota = &domain.Quota{
			Used:      nullFloatPtr(quotaUsedValue),
			Total:     nullFloatPtr(quotaTotalValue),
			Remaining: nullFloatPtr(quotaRemainingValue),
			Unit:      quotaUnitValue.String,
		}
	}
	if planNameValue.Valid || renewAt.Valid || expireAt.Valid {
		target.Plan = &domain.PlanInfo{Name: planNameValue.String, RenewAt: nullTimePtr(renewAt), ExpireAt: nullTimePtr(expireAt)}
	}
	if monthlyCostAmount.Valid {
		target.MonthlyCost = &domain.Money{Amount: monthlyCostAmount.Float64, Currency: monthlyCostCurrency.String}
	}
	target.LastScanAt = nullTimePtr(lastScanAt)
	target.NextScanAt = nullTimePtr(nextScanAt)
	return &target, nil
}

func scanSnapshot(row pgx.Row) (domain.Snapshot, error) {
	var snapshot domain.Snapshot
	var balanceAmount, quotaUsedValue, quotaTotalValue, quotaRemainingValue, monthlyCostAmount sql.NullFloat64
	var balanceCurrency, quotaUnitValue, monthlyCostCurrency sql.NullString
	err := row.Scan(&snapshot.ID, &snapshot.TargetID, &snapshot.CapturedAt, &snapshot.Status, &balanceAmount,
		&balanceCurrency, &quotaUsedValue, &quotaTotalValue, &quotaRemainingValue, &quotaUnitValue,
		&monthlyCostAmount, &monthlyCostCurrency, &snapshot.Raw)
	if err != nil {
		return snapshot, err
	}
	if balanceAmount.Valid {
		snapshot.Balance = &domain.Money{Amount: balanceAmount.Float64, Currency: balanceCurrency.String}
	}
	if quotaUsedValue.Valid || quotaTotalValue.Valid || quotaRemainingValue.Valid || quotaUnitValue.Valid {
		snapshot.Quota = &domain.Quota{
			Used:      nullFloatPtr(quotaUsedValue),
			Total:     nullFloatPtr(quotaTotalValue),
			Remaining: nullFloatPtr(quotaRemainingValue),
			Unit:      quotaUnitValue.String,
		}
	}
	if monthlyCostAmount.Valid {
		snapshot.MonthlyCost = &domain.Money{Amount: monthlyCostAmount.Float64, Currency: monthlyCostCurrency.String}
	}
	return snapshot, nil
}

func moneyAmount(money *domain.Money) any {
	if money == nil {
		return nil
	}
	return money.Amount
}

func moneyCurrency(money *domain.Money) any {
	if money == nil {
		return nil
	}
	return money.Currency
}

func quotaUsed(quota *domain.Quota) any {
	if quota == nil || quota.Used == nil {
		return nil
	}
	return *quota.Used
}

func quotaTotal(quota *domain.Quota) any {
	if quota == nil || quota.Total == nil {
		return nil
	}
	return *quota.Total
}

func quotaRemaining(quota *domain.Quota) any {
	if quota == nil || quota.Remaining == nil {
		return nil
	}
	return *quota.Remaining
}

func quotaUnit(quota *domain.Quota) any {
	if quota == nil {
		return nil
	}
	return quota.Unit
}

func planName(plan *domain.PlanInfo) any {
	if plan == nil || plan.Name == "" {
		return nil
	}
	return plan.Name
}

func planRenew(plan *domain.PlanInfo) any {
	if plan == nil || plan.RenewAt == nil {
		return nil
	}
	return *plan.RenewAt
}

func planExpire(plan *domain.PlanInfo) any {
	if plan == nil || plan.ExpireAt == nil {
		return nil
	}
	return *plan.ExpireAt
}

func nullFloatPtr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
