package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
		case "/api/v1/keys":
			if got := r.Header.Get("Authorization"); got != "Bearer sub2-user-token" {
				t.Fatalf("Authorization header = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{
					"id":         12,
					"name":       "Gemini",
					"key":        "sk-sub2-test-secret",
					"group_name": "gemini-dedicated",
					"used_quota": 2.5,
					"quota":      100,
				}},
				"total": 1,
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
}
