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
		case "/api/usage/token/":
			t.Fatalf("New API user child key scan should not call token usage with a masked key")
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
