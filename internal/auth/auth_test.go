package auth

import (
	"testing"
	"time"
)

func TestPasswordHashAndJWT(t *testing.T) {
	hash, err := HashPassword("password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !CheckPassword(hash, "password") {
		t.Fatalf("expected password to match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatalf("wrong password should not match")
	}

	service := New("jwt-secret", "api-monitor-test", time.Hour)
	token, err := service.Issue("usr_1", "admin@example.com", "admin")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	claims, err := service.Parse(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.UserID != "usr_1" || claims.Email != "admin@example.com" || claims.Role != "admin" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}
