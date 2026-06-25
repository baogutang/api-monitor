package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-monitor/internal/domain"
)

func TestNewAPIUserScanAPIKeyMatchesTokenList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/login":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"token": "user-token",
					"user":  map[string]any{"id": 42},
				},
			})
		case "/api/token/":
			if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
				t.Fatalf("Authorization header = %q", got)
			}
			if got := r.Header.Get("New-Api-User"); got != "42" {
				t.Fatalf("New-Api-User header = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": []map[string]any{{
					"id":         980,
					"key":        "htCZ**********ElHh",
					"name":       "work-key",
					"used_quota": 25,
					"quota":      100,
					"unit":       "quota",
				}},
			})
		case "/api/token/980/key":
			if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
				t.Fatalf("Authorization header for full key = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"key": "sk-full-secret-980",
				},
			})
		case "/api/usage/token/":
			if got := r.Header.Get("Authorization"); got != "Bearer sk-full-secret-980" {
				t.Fatalf("token usage should use full key, got %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"object":          "token_usage",
					"name":            "work-key",
					"total_granted":   100,
					"total_used":      25,
					"total_available": 75,
				},
			})
		case "/api/log/self/stat":
			if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
				t.Fatalf("Authorization header for token stat = %q", got)
			}
			if got := r.URL.Query().Get("token_name"); got != "work-key" {
				t.Fatalf("token_name query = %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"quota": 15,
					"rpm":   0,
					"tpm":   0,
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	connector := &newAPIUserConnector{client: server.Client()}
	result, err := connector.Scan(context.Background(), domain.Instance{
		ID:           "ins_1",
		ProviderKind: domain.ProviderNewAPIUser,
		BaseURL:      server.URL,
		Credential: &domain.Credential{
			Type:     "basic",
			Username: "alice",
			Password: "secret",
		},
	}, domain.MonitorTarget{
		InstanceID:   "ins_1",
		ProviderKind: domain.ProviderNewAPIUser,
		Kind:         domain.TargetAPIKey,
		ExternalID:   "980",
		Name:         "work-key",
		Raw:          json.RawMessage(`{"id":980,"key":"htCZ**********ElHh","name":"work-key"}`),
	})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if result.Quota == nil || result.Quota.Total == nil || *result.Quota.Total != 100 {
		t.Fatalf("quota was not parsed: %#v", result.Quota)
	}
	if got := string(result.Raw); got == "" || got == "{}" {
		t.Fatalf("target raw metadata was not updated: %s", got)
	}
}
