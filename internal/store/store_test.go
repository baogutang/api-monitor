package store

import (
	"testing"

	"api-monitor/internal/domain"
)

func TestRiskScore(t *testing.T) {
	target := domain.MonitorTarget{
		Status:  domain.StatusCritical,
		Balance: &domain.Money{Amount: 5, Currency: "USD"},
	}
	if got := RiskScore(target); got < 90 {
		t.Fatalf("expected high risk score, got %d", got)
	}

	target = domain.MonitorTarget{
		Status:  domain.StatusHealthy,
		Balance: &domain.Money{Amount: 200, Currency: "USD"},
	}
	if got := RiskScore(target); got != 0 {
		t.Fatalf("expected zero risk score, got %d", got)
	}
}
