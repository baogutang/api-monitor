package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api-monitor/internal/domain"
)

func TestSub2APIUserDiscoversAndScansUserKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"access_token": "sub2-user-token",
				},
			})
		case "/api/v1/auth/me":
			if got := r.Header.Get("Authorization"); got != "Bearer sub2-user-token" {
				t.Fatalf("Authorization header = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"id":      7,
					"email":   "ops@example.com",
					"balance": 119.87,
				},
			})
		case "/api/v1/user/platform-quotas":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"platform_quotas": []any{}},
			})
		case "/api/v1/usage/dashboard/stats":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"today_requests":      7,
					"today_actual_cost":   0.31,
					"total_requests":      15,
					"total_actual_cost":   1.23,
					"total_api_keys":      1,
					"active_api_keys":     1,
					"average_duration_ms": 120,
				},
			})
		case "/api/v1/usage/stats":
			period := r.URL.Query().Get("period")
			actualCost := map[string]float64{"today": 0.31, "week": 0.62, "month": 1.23}[period]
			if actualCost == 0 {
				actualCost = 0.31
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"total_requests":    7,
					"total_actual_cost": actualCost,
					"total_tokens":      1024,
				},
			})
		case "/api/v1/usage/dashboard/trend":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"trend": []any{}},
			})
		case "/api/v1/usage/dashboard/models":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"models": []any{}},
			})
		case "/api/v1/usage/dashboard/api-keys-usage":
			if got := r.Header.Get("Authorization"); got != "Bearer sub2-user-token" {
				t.Fatalf("Authorization header = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"stats": map[string]any{
						"12": map[string]any{
							"api_key_id":        12,
							"today_actual_cost": 0.31,
							"total_actual_cost": 1.23,
						},
					},
				},
			})
		case "/api/v1/keys":
			if got := r.Header.Get("Authorization"); got != "Bearer sub2-user-token" {
				t.Fatalf("Authorization header = %q", got)
			}
			if got := r.URL.Query().Get("page_size"); got != "" && got != "100" {
				t.Fatalf("page_size query = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{
					"id":         12,
					"name":       "Gemini",
					"key":        "sk-sub2-test-secret",
					"quota_used": 2.5,
					"quota":      100,
					"group": map[string]any{
						"id":              9,
						"name":            "gemini-dedicated",
						"rate_multiplier": 1.3,
					},
				}},
				"total": 1,
			})
		case "/api/v1/user/api-keys/12/usage/daily":
			if got := r.Header.Get("Authorization"); got != "Bearer sub2-user-token" {
				t.Fatalf("Authorization header = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"items": []map[string]any{{
						"date":         time.Now().UTC().Format("2006-01-02"),
						"requests":     7,
						"total_tokens": 1024,
						"cost":         0.42,
						"actual_cost":  0.31,
					}},
				},
			})
		case "/api/v1/subscriptions", "/api/v1/subscriptions/active", "/api/v1/user/subscriptions":
			http.Error(w, `{"message":"not enabled"}`, http.StatusNotFound)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	instance := domain.Instance{
		ID:           "ins_sub2",
		Name:         "sub2 relay",
		ProviderKind: domain.ProviderSub2APIUser,
		BaseURL:      server.URL,
		Credential: &domain.Credential{
			Type:     "basic",
			Username: "ops@example.com",
			Password: "secret",
		},
	}

	connector := &sub2APIUserConnector{client: server.Client()}
	targets, err := connector.Discover(context.Background(), instance)
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(targets) < 2 {
		t.Fatalf("expected user and API key targets, got %#v", targets)
	}
	keyTarget := targets[1]
	if keyTarget.Kind != domain.TargetAPIKey || keyTarget.Name != "Gemini" {
		t.Fatalf("unexpected key target: %#v", keyTarget)
	}
	if keyTarget.ExternalID != "12" || keyTarget.GroupName != "gemini-dedicated" || keyTarget.KeyFingerprint == "" {
		t.Fatalf("key metadata was not parsed: %#v", keyTarget)
	}

	result, err := connector.Scan(context.Background(), instance, keyTarget)
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if result.Quota == nil || result.Quota.Total == nil || *result.Quota.Total != 100 {
		t.Fatalf("quota was not parsed: %#v", result.Quota)
	}
	if result.Quota.Unit != "USD" {
		t.Fatalf("quota unit was not normalized: %#v", result.Quota)
	}
	var raw map[string]any
	if err := json.Unmarshal(result.Raw, &raw); err != nil {
		t.Fatalf("raw was not json: %v", err)
	}
	if raw["groupName"] != "gemini-dedicated" || raw["today_cost"] == nil || raw["today_requests"] == nil {
		t.Fatalf("raw key context was not merged: %#v", raw)
	}
}
