package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"api-monitor/internal/auth"
	"api-monitor/internal/cache"
	"api-monitor/internal/config"
	"api-monitor/internal/domain"
	"api-monitor/internal/notify"
	"api-monitor/internal/scanner"
	"api-monitor/internal/store"
	"api-monitor/internal/version"
)

type Server struct {
	store    *store.Store
	auth     auth.Service
	scanner  *scanner.Service
	cache    *cache.Cache
	notifier *notify.Service
	cfg      config.Config
	client   *http.Client
}

type contextKey string

const userContextKey contextKey = "user"

func New(st *store.Store, authSvc auth.Service, scannerSvc *scanner.Service, cacheSvc *cache.Cache, notifierSvc *notify.Service, cfg config.Config, client *http.Client) *Server {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Server{store: st, auth: authSvc, scanner: scannerSvc, cache: cacheSvc, notifier: notifierSvc, cfg: cfg, client: client}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/v1/version", s.versionInfo)
	mux.HandleFunc("POST /api/v1/version/check", s.versionCheck)
	mux.Handle("POST /api/v1/version/update", s.requireAuth(http.HandlerFunc(s.versionUpdate)))
	mux.HandleFunc("GET /api/v1/setup/status", s.setupStatus)
	mux.HandleFunc("POST /api/v1/setup", s.setup)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.Handle("GET /api/v1/auth/me", s.requireAuth(http.HandlerFunc(s.me)))

	mux.Handle("GET /api/v1/dashboard/summary", s.requireAuth(http.HandlerFunc(s.dashboardSummary)))
	mux.Handle("GET /api/v1/dashboard/risk-targets", s.requireAuth(http.HandlerFunc(s.riskTargets)))
	mux.Handle("GET /api/v1/dashboard/trends", s.requireAuth(http.HandlerFunc(s.dashboardTrends)))

	mux.Handle("GET /api/v1/instances", s.requireAuth(http.HandlerFunc(s.listInstances)))
	mux.Handle("POST /api/v1/instances", s.requireAuth(http.HandlerFunc(s.createInstance)))
	mux.Handle("POST /api/v1/instances/test-draft", s.requireAuth(http.HandlerFunc(s.testDraftInstance)))
	mux.Handle("GET /api/v1/instances/{id}", s.requireAuth(http.HandlerFunc(s.getInstance)))
	mux.Handle("PATCH /api/v1/instances/{id}", s.requireAuth(http.HandlerFunc(s.updateInstance)))
	mux.Handle("DELETE /api/v1/instances/{id}", s.requireAuth(http.HandlerFunc(s.deleteInstance)))
	mux.Handle("POST /api/v1/instances/{id}/test", s.requireAuth(http.HandlerFunc(s.testInstance)))
	mux.Handle("POST /api/v1/instances/{id}/discover", s.requireAuth(http.HandlerFunc(s.discoverInstance)))
	mux.Handle("POST /api/v1/account-oauth/{provider}/authorize", s.requireAuth(http.HandlerFunc(s.accountOAuthAuthorize)))
	mux.Handle("POST /api/v1/account-oauth/{provider}/exchange", s.requireAuth(http.HandlerFunc(s.accountOAuthExchange)))

	mux.Handle("GET /api/v1/targets", s.requireAuth(http.HandlerFunc(s.listTargets)))
	mux.Handle("GET /api/v1/targets/{id}", s.requireAuth(http.HandlerFunc(s.getTarget)))
	mux.Handle("PATCH /api/v1/targets/{id}", s.requireAuth(http.HandlerFunc(s.updateTarget)))
	mux.Handle("POST /api/v1/targets/{id}/scan", s.requireAuth(http.HandlerFunc(s.scanTarget)))
	mux.Handle("GET /api/v1/targets/{id}/snapshots", s.requireAuth(http.HandlerFunc(s.targetSnapshots)))
	mux.Handle("GET /api/v1/targets/{id}/alerts", s.requireAuth(http.HandlerFunc(s.targetAlerts)))

	mux.Handle("GET /api/v1/alert-rules", s.requireAuth(http.HandlerFunc(s.listAlertRules)))
	mux.Handle("POST /api/v1/alert-rules", s.requireAuth(http.HandlerFunc(s.upsertAlertRule)))
	mux.Handle("PATCH /api/v1/alert-rules/{id}", s.requireAuth(http.HandlerFunc(s.upsertAlertRule)))
	mux.Handle("DELETE /api/v1/alert-rules/{id}", s.requireAuth(http.HandlerFunc(s.deleteAlertRule)))

	mux.Handle("GET /api/v1/alerts", s.requireAuth(http.HandlerFunc(s.listAlerts)))
	mux.Handle("POST /api/v1/alerts/{id}/ack", s.requireAuth(http.HandlerFunc(s.ackAlert)))
	mux.Handle("POST /api/v1/alerts/{id}/silence", s.requireAuth(http.HandlerFunc(s.silenceAlert)))
	mux.Handle("POST /api/v1/alerts/{id}/resolve", s.requireAuth(http.HandlerFunc(s.resolveAlert)))

	mux.Handle("GET /api/v1/notification-channels", s.requireAuth(http.HandlerFunc(s.listNotificationChannels)))
	mux.Handle("POST /api/v1/notification-channels", s.requireAuth(http.HandlerFunc(s.upsertNotificationChannel)))
	mux.Handle("PATCH /api/v1/notification-channels/{id}", s.requireAuth(http.HandlerFunc(s.upsertNotificationChannel)))
	mux.Handle("DELETE /api/v1/notification-channels/{id}", s.requireAuth(http.HandlerFunc(s.deleteNotificationChannel)))
	mux.Handle("POST /api/v1/notification-channels/{id}/test", s.requireAuth(http.HandlerFunc(s.testNotificationChannel)))
	mux.Handle("POST /api/v1/notification-channels/test-draft", s.requireAuth(http.HandlerFunc(s.testDraftNotificationChannel)))

	mux.Handle("GET /api/v1/scan-runs", s.requireAuth(http.HandlerFunc(s.listScanRuns)))
	mux.Handle("GET /api/v1/settings", s.requireAuth(http.HandlerFunc(s.getSettings)))
	mux.Handle("PATCH /api/v1/settings", s.requireAuth(http.HandlerFunc(s.updateSettings)))

	return s.cors(s.withStatic(mux))
}

func (s *Server) withStatic(apiHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/healthz" {
			apiHandler.ServeHTTP(w, r)
			return
		}
		staticDir := s.cfg.StaticDir
		if staticDir == "" {
			staticDir = "web/dist"
		}
		path := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if path == "." || path == "/" {
			path = "index.html"
		}
		fullPath, ok := safeStaticPath(staticDir, path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, fullPath)
			return
		}
		indexPath := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
			return
		}
		apiHandler.ServeHTTP(w, r)
	})
}

func safeStaticPath(staticDir string, path string) (string, bool) {
	root, err := filepath.Abs(staticDir)
	if err != nil {
		return "", false
	}
	fullPath, err := filepath.Abs(filepath.Join(root, path))
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return fullPath, true
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token", nil)
			return
		}
		claims, err := s.auth.Parse(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token", nil)
			return
		}
		user, err := s.store.GetUserByID(r.Context(), claims.UserID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "user not found", nil)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentUser(r *http.Request) *store.User {
	user, _ := r.Context().Value(userContextKey).(*store.User)
	return user
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) setupStatus(w http.ResponseWriter, r *http.Request) {
	hasUsers, err := s.store.HasUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": hasUsers, "needsSetup": !hasUsers})
}

func (s *Server) versionInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Current(s.cfg.GitHubRepo, s.cfg.EnableSelfUpdate && s.cfg.UpdateCommand != ""))
}

func (s *Server) versionCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.CheckLatest(r.Context(), s.client, s.cfg.GitHubRepo, s.cfg.EnableSelfUpdate && s.cfg.UpdateCommand != ""))
}

func (s *Server) versionUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.EnableSelfUpdate || s.cfg.UpdateCommand == "" {
		writeError(w, http.StatusForbidden, "self_update_disabled", "self update is disabled; configure ENABLE_SELF_UPDATE=true and UPDATE_COMMAND to enable it", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", s.cfg.UpdateCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update_failed", err.Error(), string(output))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output": string(output)})
}

type setupRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	hasUsers, err := s.store.HasUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error(), nil)
		return
	}
	if hasUsers {
		writeError(w, http.StatusConflict, "already_configured", "setup has already completed", nil)
		return
	}
	var req setupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email and password are required", nil)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash_failed", err.Error(), nil)
		return
	}
	if req.Name == "" {
		req.Name = "Admin"
	}
	user, err := s.store.CreateUser(r.Context(), req.Email, req.Name, hash, "admin")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create_user_failed", err.Error(), nil)
		return
	}
	token, _ := s.auth.Issue(user.ID, user.Email, user.Role)
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "user": user})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.store.GetUserByEmail(r.Context(), req.Email)
	if err != nil || !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect", nil)
		return
	}
	token, err := s.auth.Issue(user.ID, user.Email, user.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

func (s *Server) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.DashboardSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "dashboard_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) riskTargets(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 10)
	targets, err := s.store.ListTargets(r.Context(), store.TargetFilter{Limit: limit})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "targets_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (s *Server) dashboardTrends(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargets(r.Context(), store.TargetFilter{Limit: 20})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "trends_failed", err.Error(), nil)
		return
	}
	points := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		point := map[string]any{
			"capturedAt":   time.Now().UTC().Format(time.RFC3339),
			"providerKind": target.ProviderKind,
		}
		if target.Balance != nil {
			point["balance"] = target.Balance
		}
		if target.MonthlyCost != nil {
			point["cost"] = target.MonthlyCost
		}
		points = append(points, point)
	}
	writeJSON(w, http.StatusOK, points)
}

func (s *Server) listInstances(w http.ResponseWriter, r *http.Request) {
	instances, err := s.store.ListInstances(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "instances_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, instances)
}

type upsertInstanceRequest struct {
	Name                string              `json:"name"`
	ProviderKind        domain.ProviderKind `json:"providerKind"`
	BaseURL             string              `json:"baseUrl"`
	GroupName           string              `json:"groupName"`
	Enabled             *bool               `json:"enabled"`
	ScanIntervalSeconds int                 `json:"scanIntervalSeconds"`
	Capabilities        []domain.Capability `json:"capabilities"`
	Settings            map[string]any      `json:"settings"`
	Credential          *domain.Credential  `json:"credential"`
}

func (s *Server) createInstance(w http.ResponseWriter, r *http.Request) {
	s.upsertInstance(w, r, "")
}

func (s *Server) updateInstance(w http.ResponseWriter, r *http.Request) {
	s.upsertInstance(w, r, r.PathValue("id"))
}

func (s *Server) upsertInstance(w http.ResponseWriter, r *http.Request, id string) {
	var req upsertInstanceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	settings := json.RawMessage(`{}`)
	if req.Settings != nil {
		settings, _ = json.Marshal(req.Settings)
	}
	if len(req.Capabilities) == 0 {
		req.Capabilities = defaultCapabilities(req.ProviderKind)
	}
	instance := domain.Instance{
		ID:                  id,
		Name:                req.Name,
		ProviderKind:        req.ProviderKind,
		BaseURL:             req.BaseURL,
		GroupName:           req.GroupName,
		Enabled:             enabled,
		ScanIntervalSeconds: req.ScanIntervalSeconds,
		Capabilities:        req.Capabilities,
		Settings:            settings,
	}
	if instance.Name == "" || instance.ProviderKind == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name and providerKind are required", nil)
		return
	}
	saved, err := s.store.UpsertInstance(r.Context(), instance, req.Credential)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "instance_save_failed", err.Error(), nil)
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidateConfig(r.Context())
	}
	_ = s.store.Audit(r.Context(), currentUser(r).ID, "upsert_instance", "instance", saved.ID, map[string]any{"providerKind": saved.ProviderKind})
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) getInstance(w http.ResponseWriter, r *http.Request) {
	instance, err := s.store.GetInstance(r.Context(), r.PathValue("id"), false)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, instance)
}

func (s *Server) deleteInstance(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteInstance(r.Context(), r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidateConfig(r.Context())
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testInstance(w http.ResponseWriter, r *http.Request) {
	result, err := s.scanner.TestInstance(r.Context(), r.PathValue("id"))
	if err != nil && result == nil {
		writeError(w, http.StatusBadGateway, "test_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) testDraftInstance(w http.ResponseWriter, r *http.Request) {
	var req upsertInstanceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	settings := json.RawMessage(`{}`)
	if req.Settings != nil {
		settings, _ = json.Marshal(req.Settings)
	}
	if len(req.Capabilities) == 0 {
		req.Capabilities = defaultCapabilities(req.ProviderKind)
	}
	instance := domain.Instance{
		ID:                  "draft",
		Name:                firstNonEmpty(req.Name, "draft"),
		ProviderKind:        req.ProviderKind,
		BaseURL:             req.BaseURL,
		GroupName:           req.GroupName,
		Enabled:             enabled,
		ScanIntervalSeconds: req.ScanIntervalSeconds,
		Capabilities:        req.Capabilities,
		Settings:            settings,
		Credential:          req.Credential,
	}
	if instance.ProviderKind == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "providerKind is required", nil)
		return
	}
	result, err := s.scanner.TestDraftInstance(r.Context(), instance)
	if err != nil && result == nil {
		writeError(w, http.StatusBadGateway, "test_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) discoverInstance(w http.ResponseWriter, r *http.Request) {
	targets, err := s.scanner.DiscoverInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "discover_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"created": len(targets), "items": targets})
}

func (s *Server) listTargets(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)
	targets, err := s.store.ListTargets(r.Context(), store.TargetFilter{
		ProviderKind: r.URL.Query().Get("providerKind"),
		Status:       r.URL.Query().Get("status"),
		GroupName:    r.URL.Query().Get("groupName"),
		Query:        r.URL.Query().Get("q"),
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "targets_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, paginated(targets, len(targets), limit, offset))
}

func (s *Server) getTarget(w http.ResponseWriter, r *http.Request) {
	target, err := s.store.GetTarget(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (s *Server) updateTarget(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      *string `json:"name"`
		GroupName *string `json:"groupName"`
		Enabled   *bool   `json:"enabled"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	target, err := s.store.UpdateTargetEditable(r.Context(), r.PathValue("id"), req.Name, req.GroupName, req.Enabled)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (s *Server) scanTarget(w http.ResponseWriter, r *http.Request) {
	target, err := s.scanner.ScanTarget(r.Context(), r.PathValue("id"))
	if err != nil && target == nil {
		writeError(w, http.StatusBadGateway, "scan_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, target)
}

func (s *Server) targetSnapshots(w http.ResponseWriter, r *http.Request) {
	since := sinceFromRange(r.URL.Query().Get("range"))
	snapshots, err := s.store.ListSnapshots(r.Context(), r.PathValue("id"), since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "snapshots_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, snapshots)
}

func (s *Server) targetAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := s.store.ListAlerts(r.Context(), "", "", 100, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "alerts_failed", err.Error(), nil)
		return
	}
	targetID := r.PathValue("id")
	filtered := make([]domain.AlertEvent, 0)
	for _, alert := range alerts {
		if alert.TargetID == targetID {
			filtered = append(filtered, alert)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (s *Server) listAlertRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.store.ListAlertRules(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rules_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) upsertAlertRule(w http.ResponseWriter, r *http.Request) {
	var rule domain.AlertRule
	if !decodeJSON(w, r, &rule) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		rule.ID = id
	}
	out, err := s.store.UpsertAlertRule(r.Context(), rule)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rule_save_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteAlertRule(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteAlertRule(r.Context(), r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)
	alerts, err := s.store.ListAlerts(r.Context(), r.URL.Query().Get("status"), r.URL.Query().Get("severity"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "alerts_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, paginated(alerts, len(alerts), limit, offset))
}

func (s *Server) ackAlert(w http.ResponseWriter, r *http.Request) {
	alert, err := s.store.UpdateAlertStatus(r.Context(), r.PathValue("id"), "acknowledged", nil)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alert)
}

func (s *Server) silenceAlert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Minutes int    `json:"minutes"`
		Until   string `json:"until"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	var until time.Time
	if req.Until != "" {
		parsed, err := time.Parse(time.RFC3339, req.Until)
		if err == nil {
			until = parsed
		}
	}
	if until.IsZero() && req.Minutes <= 0 {
		req.Minutes = 60
	}
	if until.IsZero() {
		until = time.Now().UTC().Add(time.Duration(req.Minutes) * time.Minute)
	}
	alert, err := s.store.UpdateAlertStatus(r.Context(), r.PathValue("id"), "silenced", &until)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alert)
}

func (s *Server) resolveAlert(w http.ResponseWriter, r *http.Request) {
	alert, err := s.store.UpdateAlertStatus(r.Context(), r.PathValue("id"), "resolved", nil)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alert)
}

func (s *Server) listNotificationChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := s.store.ListNotificationChannels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "channels_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (s *Server) upsertNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID          string          `json:"id"`
		Name        string          `json:"name"`
		Type        string          `json:"type"`
		Enabled     *bool           `json:"enabled"`
		Settings    json.RawMessage `json:"settings"`
		SecretValue string          `json:"secretValue"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		req.ID = id
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if len(req.Settings) == 0 {
		req.Settings = json.RawMessage(`{}`)
	}
	channel, err := s.store.UpsertNotificationChannel(r.Context(), domain.NotificationChannel{
		ID:          req.ID,
		Name:        req.Name,
		Type:        req.Type,
		Enabled:     enabled,
		Settings:    req.Settings,
		SecretValue: req.SecretValue,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "channel_save_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, channel)
}

func (s *Server) deleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteNotificationChannel(r.Context(), r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testNotificationChannel(w http.ResponseWriter, r *http.Request) {
	channel, err := s.store.GetNotificationChannel(r.Context(), r.PathValue("id"), true)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	response, err := s.notifier.Send(r.Context(), *channel, domain.AlertEvent{
		ID:       "test",
		Severity: "info",
		Status:   "open",
		Title:    "API Monitor test alert",
		Message:  "This is a notification channel test from API Monitor.",
		OpenedAt: time.Now().UTC(),
	}, draftTarget())
	if err != nil {
		writeError(w, http.StatusBadGateway, "notification_test_failed", err.Error(), response)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "channel": channel.ID, "response": response})
}

func (s *Server) testDraftNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string          `json:"name"`
		Type        string          `json:"type"`
		Enabled     *bool           `json:"enabled"`
		Settings    json.RawMessage `json:"settings"`
		SecretValue string          `json:"secretValue"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if len(req.Settings) == 0 {
		req.Settings = json.RawMessage(`{}`)
	}
	response, err := s.notifier.Send(r.Context(), domain.NotificationChannel{
		ID:          "draft",
		Name:        req.Name,
		Type:        req.Type,
		Enabled:     enabled,
		Settings:    req.Settings,
		SecretValue: req.SecretValue,
	}, draftAlert(), draftTarget())
	if err != nil {
		writeError(w, http.StatusBadGateway, "notification_test_failed", err.Error(), response)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "response": response})
}

func draftAlert() domain.AlertEvent {
	return domain.AlertEvent{
		ID:       "draft-test",
		Severity: "critical",
		Status:   "open",
		Title:    "[CRITICAL] openai-relay-prod 余额严重不足",
		Message:  "openai-relay-prod 余额严重不足，当前余额 $8.42，剩余额度 2.1%，已低于阈值 $10.00，需立即处理。",
		OpenedAt: time.Now().UTC(),
	}
}

func draftTarget() *domain.MonitorTarget {
	remaining := 2.1
	return &domain.MonitorTarget{
		ID:           "draft-target",
		ProviderKind: domain.ProviderOpenAIAccount,
		Kind:         domain.TargetUser,
		Name:         "openai-relay-prod",
		GroupName:    "production",
		Status:       domain.StatusCritical,
		Balance:      &domain.Money{Amount: 8.42, Currency: "USD"},
		Quota:        &domain.Quota{Remaining: &remaining, Unit: "%"},
		Enabled:      true,
	}
}

func (s *Server) listScanRuns(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)
	runs, err := s.store.ListScanRuns(r.Context(), r.URL.Query().Get("targetId"), r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scan_runs_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, paginated(runs, len(runs), limit, offset))
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var values map[string]json.RawMessage
	if !decodeJSON(w, r, &values) {
		return
	}
	if err := s.store.UpsertSettings(r.Context(), values); err != nil {
		writeError(w, http.StatusInternalServerError, "settings_save_failed", err.Error(), nil)
		return
	}
	if s.cache != nil {
		_ = s.cache.InvalidateConfig(r.Context())
	}
	settings, err := s.store.GetSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_failed", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func defaultCapabilities(kind domain.ProviderKind) []domain.Capability {
	switch kind {
	case domain.ProviderOpenAIAdmin:
		return []domain.Capability{domain.CapabilityCost, domain.CapabilityUsage, domain.CapabilityHealth}
	case domain.ProviderOpenAIAccount, domain.ProviderGeminiAccount, domain.ProviderAnthropicAccount:
		return []domain.Capability{domain.CapabilityUsage, domain.CapabilityHealth, domain.CapabilityWindowQuota, domain.CapabilityManualPlan}
	case domain.ProviderOpenAIKey, domain.ProviderAnthropicKey, domain.ProviderSub2APIToken:
		return []domain.Capability{domain.CapabilityHealth}
	case domain.ProviderManualSub:
		return []domain.Capability{domain.CapabilityManualPlan, domain.CapabilityHealth}
	case domain.ProviderGenericHTTP:
		return []domain.Capability{domain.CapabilityBalance, domain.CapabilityUsage, domain.CapabilityHealth}
	default:
		return []domain.Capability{domain.CapabilityUsage, domain.CapabilityHealth}
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "details": details}})
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "resource not found", nil)
		return
	}
	writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
}

func paginated[T any](items []T, total, limit, offset int) map[string]any {
	return map[string]any{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}
}

func intQuery(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func sinceFromRange(value string) time.Time {
	now := time.Now().UTC()
	switch value {
	case "24h":
		return now.Add(-24 * time.Hour)
	case "30d":
		return now.AddDate(0, 0, -30)
	default:
		return now.AddDate(0, 0, -7)
	}
}

func debugString(value any) string {
	return fmt.Sprintf("%v", value)
}
