package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-monitor/internal/domain"
)

func TestNewAPIUserHeadersUsesLoginTokenAndUserID(t *testing.T) {
	var sawLogin bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/login" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		sawLogin = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"data": map[string]any{
				"token": "user-token",
				"user": map[string]any{
					"id":       42,
					"username": "alice",
				},
			},
		})
	}))
	defer server.Close()

	headers, raw, err := newAPIUserHeaders(context.Background(), server.Client(), domain.Instance{
		BaseURL: server.URL,
		Credential: &domain.Credential{
			Type:     "basic",
			Username: "alice",
			Password: "secret",
		},
	})
	if err != nil {
		t.Fatalf("newAPIUserHeaders returned error: %v; raw=%s", err, string(raw))
	}
	if !sawLogin {
		t.Fatal("login endpoint was not called")
	}
	if got := headers["Authorization"]; got != "Bearer user-token" {
		t.Fatalf("Authorization header = %q", got)
	}
	if got := headers["New-Api-User"]; got != "42" {
		t.Fatalf("New-Api-User header = %q", got)
	}
}

func TestSub2APIUserHeadersUsesAccessToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"access_token": "sub2-token",
			},
		})
	}))
	defer server.Close()

	headers, raw, err := sub2APIUserHeaders(context.Background(), server.Client(), domain.Instance{
		BaseURL: server.URL,
		Credential: &domain.Credential{
			Type:     "basic",
			Username: "ops@example.com",
			Password: "secret",
		},
	})
	if err != nil {
		t.Fatalf("sub2APIUserHeaders returned error: %v; raw=%s", err, string(raw))
	}
	if got := headers["Authorization"]; got != "Bearer sub2-token" {
		t.Fatalf("Authorization header = %q", got)
	}
}
