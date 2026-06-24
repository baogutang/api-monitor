package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	openAIOAuthClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAIAuthorizeURL      = "https://auth.openai.com/oauth/authorize"
	openAITokenURL          = "https://auth.openai.com/oauth/token"
	defaultLocalRedirectURI = "http://localhost:1455/auth/callback"
	claudeOAuthClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	claudeAuthorizeURL      = "https://claude.ai/oauth/authorize"
	claudeTokenURL          = "https://platform.claude.com/v1/oauth/token"
	claudeRedirectURI       = "https://platform.claude.com/oauth/code/callback"
	claudeScope             = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	geminiAuthorizeURL      = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiTokenURL          = "https://oauth2.googleapis.com/token"
	geminiCLIRedirectURI    = "https://codeassist.google.com/authcode"
	geminiCLIOAuthClientID  = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	geminiCLIOAuthSecretEnv = "GEMINI_CLI_OAUTH_CLIENT_SECRET"
)

var accountOAuthSessions sync.Map

type accountOAuthSession struct {
	Provider     string
	State        string
	CodeVerifier string
	RedirectURI  string
	ClientID     string
	ClientSecret string
	OAuthType    string
	ProjectID    string
	CreatedAt    time.Time
}

func (s *Server) accountOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RedirectURI  string `json:"redirectUri"`
		OAuthType    string `json:"oauthType"`
		ProjectID    string `json:"projectId"`
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	provider := normalizeAccountOAuthProvider(r.PathValue("provider"))
	if provider == "" {
		writeError(w, http.StatusBadRequest, "unsupported_provider", "unsupported OAuth provider", nil)
		return
	}
	session, err := newAccountOAuthSession(provider, req.RedirectURI, req.OAuthType, req.ProjectID, req.ClientID, req.ClientSecret)
	if err != nil {
		writeError(w, http.StatusBadRequest, "oauth_config_error", err.Error(), nil)
		return
	}
	sessionID, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "oauth_session_failed", err.Error(), nil)
		return
	}
	accountOAuthSessions.Store(sessionID, session)
	writeJSON(w, http.StatusOK, map[string]any{
		"authUrl":     buildAccountAuthURL(session),
		"sessionId":   sessionID,
		"state":       session.State,
		"redirectUri": session.RedirectURI,
		"expiresAt":   session.CreatedAt.Add(30 * time.Minute),
	})
}

func (s *Server) accountOAuthExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID   string `json:"sessionId"`
		Code        string `json:"code"`
		CallbackURL string `json:"callbackUrl"`
		State       string `json:"state"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	raw, state := extractOAuthCode(firstNonEmpty(req.CallbackURL, req.Code))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "oauth_code_required", "authorization callback URL or code is required", nil)
		return
	}
	value, ok := accountOAuthSessions.Load(req.SessionID)
	if !ok {
		writeError(w, http.StatusBadRequest, "oauth_session_not_found", "authorization session expired, generate a new link and try again", nil)
		return
	}
	session := value.(*accountOAuthSession)
	if time.Since(session.CreatedAt) > 30*time.Minute {
		accountOAuthSessions.Delete(req.SessionID)
		writeError(w, http.StatusBadRequest, "oauth_session_expired", "authorization session expired, generate a new link and try again", nil)
		return
	}
	if req.State != "" {
		state = req.State
	}
	if state != "" && state != session.State {
		writeError(w, http.StatusBadRequest, "oauth_state_mismatch", "authorization state does not match this session", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	credential, err := exchangeAccountOAuthCode(ctx, s.client, session, raw)
	if err != nil {
		writeError(w, http.StatusBadGateway, "oauth_exchange_failed", err.Error(), nil)
		return
	}
	accountOAuthSessions.Delete(req.SessionID)
	writeJSON(w, http.StatusOK, map[string]any{
		"credential": credential,
		"account":    safeAccountMeta(credential),
	})
}

func normalizeAccountOAuthProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "openai_account":
		return "openai"
	case "anthropic", "claude", "anthropic_account":
		return "anthropic"
	case "gemini", "gemini_account":
		return "gemini"
	default:
		return ""
	}
}

func newAccountOAuthSession(provider, redirectURI, oauthType, projectID, clientID, clientSecret string) (*accountOAuthSession, error) {
	state, err := randomState()
	if err != nil {
		return nil, err
	}
	verifier, err := randomCodeVerifier(provider)
	if err != nil {
		return nil, err
	}
	session := &accountOAuthSession{
		Provider:     provider,
		State:        state,
		CodeVerifier: verifier,
		RedirectURI:  strings.TrimSpace(redirectURI),
		OAuthType:    strings.TrimSpace(oauthType),
		ProjectID:    strings.TrimSpace(projectID),
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		CreatedAt:    time.Now(),
	}
	switch provider {
	case "openai":
		if session.RedirectURI == "" {
			session.RedirectURI = defaultLocalRedirectURI
		}
		session.ClientID = openAIOAuthClientID
	case "anthropic":
		session.RedirectURI = claudeRedirectURI
		session.ClientID = claudeOAuthClientID
	case "gemini":
		if session.OAuthType == "" {
			session.OAuthType = "code_assist"
		}
		if session.OAuthType == "ai_studio" {
			if session.RedirectURI == "" {
				session.RedirectURI = defaultLocalRedirectURI
			}
			session.ClientID = firstNonEmpty(session.ClientID, strings.TrimSpace(os.Getenv("GEMINI_OAUTH_CLIENT_ID")))
			session.ClientSecret = firstNonEmpty(session.ClientSecret, strings.TrimSpace(os.Getenv("GEMINI_OAUTH_CLIENT_SECRET")))
			if session.ClientID == "" || session.ClientSecret == "" {
				return nil, fmt.Errorf("Gemini AI Studio OAuth requires GEMINI_OAUTH_CLIENT_ID and GEMINI_OAUTH_CLIENT_SECRET, or a one-time client id/secret in this request")
			}
		} else {
			session.RedirectURI = geminiCLIRedirectURI
			session.ClientID = geminiCLIOAuthClientID
			session.ClientSecret = strings.TrimSpace(os.Getenv(geminiCLIOAuthSecretEnv))
			if session.ClientSecret == "" {
				return nil, fmt.Errorf("Gemini Code Assist OAuth requires %s in the backend environment", geminiCLIOAuthSecretEnv)
			}
		}
	}
	return session, nil
}

func buildAccountAuthURL(session *accountOAuthSession) string {
	challenge := codeChallenge(session.CodeVerifier)
	params := url.Values{}
	switch session.Provider {
	case "openai":
		params.Set("response_type", "code")
		params.Set("client_id", session.ClientID)
		params.Set("redirect_uri", session.RedirectURI)
		params.Set("scope", "openid profile email offline_access")
		params.Set("state", session.State)
		params.Set("code_challenge", challenge)
		params.Set("code_challenge_method", "S256")
		params.Set("id_token_add_organizations", "true")
		params.Set("codex_cli_simplified_flow", "true")
		return openAIAuthorizeURL + "?" + params.Encode()
	case "anthropic":
		params.Set("code", "true")
		params.Set("client_id", session.ClientID)
		params.Set("response_type", "code")
		params.Set("redirect_uri", session.RedirectURI)
		params.Set("scope", claudeScope)
		params.Set("code_challenge", challenge)
		params.Set("code_challenge_method", "S256")
		params.Set("state", session.State)
		return claudeAuthorizeURL + "?" + params.Encode()
	default:
		params.Set("response_type", "code")
		params.Set("client_id", session.ClientID)
		params.Set("redirect_uri", session.RedirectURI)
		params.Set("scope", geminiScopes(session.OAuthType))
		params.Set("state", session.State)
		params.Set("code_challenge", challenge)
		params.Set("code_challenge_method", "S256")
		params.Set("access_type", "offline")
		params.Set("prompt", "consent")
		params.Set("include_granted_scopes", "true")
		if session.ProjectID != "" {
			params.Set("project_id", session.ProjectID)
		}
		return geminiAuthorizeURL + "?" + params.Encode()
	}
}

func exchangeAccountOAuthCode(ctx context.Context, client *http.Client, session *accountOAuthSession, code string) (map[string]any, error) {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	switch session.Provider {
	case "openai":
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {session.ClientID},
			"code":          {code},
			"redirect_uri":  {session.RedirectURI},
			"code_verifier": {session.CodeVerifier},
		}
		token, err := postOAuthForm(ctx, client, openAITokenURL, form, map[string]string{"User-Agent": "codex-cli/0.91.0"})
		if err != nil {
			return nil, err
		}
		enrichOpenAIClaims(token)
		token["auth_method"] = "oauth"
		token["client_id"] = session.ClientID
		return map[string]any{"type": "json", "json": token}, nil
	case "anthropic":
		authCode := code
		codeState := ""
		if idx := strings.Index(code, "#"); idx != -1 {
			authCode = code[:idx]
			codeState = code[idx+1:]
		}
		body := map[string]any{
			"grant_type":    "authorization_code",
			"client_id":     session.ClientID,
			"code":          authCode,
			"redirect_uri":  session.RedirectURI,
			"code_verifier": session.CodeVerifier,
		}
		if codeState != "" {
			body["state"] = codeState
		}
		token, err := postOAuthJSON(ctx, client, claudeTokenURL, body)
		if err != nil {
			return nil, err
		}
		token["auth_method"] = "oauth"
		if account, ok := token["account"].(map[string]any); ok {
			token["account_uuid"] = account["uuid"]
			token["email"] = account["email_address"]
		}
		if organization, ok := token["organization"].(map[string]any); ok {
			token["org_uuid"] = organization["uuid"]
		}
		return map[string]any{"type": "json", "json": token}, nil
	default:
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {session.ClientID},
			"client_secret": {session.ClientSecret},
			"code":          {code},
			"code_verifier": {session.CodeVerifier},
			"redirect_uri":  {session.RedirectURI},
		}
		token, err := postOAuthForm(ctx, client, geminiTokenURL, form, nil)
		if err != nil {
			return nil, err
		}
		token["auth_method"] = "oauth"
		token["oauth_type"] = session.OAuthType
		token["project_id"] = session.ProjectID
		if email := googleUserEmail(ctx, client, stringFromAny(token["access_token"])); email != "" {
			token["email"] = email
		}
		return map[string]any{"type": "json", "json": token}, nil
	}
}

func postOAuthForm(ctx context.Context, client *http.Client, endpoint string, form url.Values, extraHeaders map[string]string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}
	return decodeOAuthResponse(client.Do(req))
}

func postOAuthJSON(ctx context.Context, client *http.Client, endpoint string, body map[string]any) (map[string]any, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "axios/1.13.6")
	return decodeOAuthResponse(client.Do(req))
}

func decodeOAuthResponse(resp *http.Response, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(data))
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if expiresIn, ok := out["expires_in"].(float64); ok {
		out["expires_at"] = time.Now().Add(time.Duration(expiresIn) * time.Second).Unix()
	}
	return out, nil
}

func extractOAuthCode(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		code := parsed.Query().Get("code")
		state := parsed.Query().Get("state")
		if code == "" && parsed.Fragment != "" {
			fragment, _ := url.ParseQuery(parsed.Fragment)
			code = fragment.Get("code")
			state = firstNonEmpty(state, fragment.Get("state"))
		}
		if code != "" {
			return code, state
		}
	}
	if idx := strings.Index(value, "#"); idx != -1 {
		return value[:idx], value[idx+1:]
	}
	return value, ""
}

func safeAccountMeta(credential map[string]any) map[string]any {
	jsonValue, _ := credential["json"].(map[string]any)
	meta := map[string]any{}
	for _, key := range []string{"email", "name", "plan_type", "chatgpt_account_id", "organization_id", "account_uuid", "org_uuid", "project_id", "oauth_type", "expires_at"} {
		if value, ok := jsonValue[key]; ok && value != "" {
			meta[key] = value
		}
	}
	return meta
}

func enrichOpenAIClaims(token map[string]any) {
	idToken := stringFromAny(token["id_token"])
	claims := parseJWTClaims(idToken)
	if len(claims) == 0 {
		return
	}
	if email := stringFromAny(claims["email"]); email != "" {
		token["email"] = email
	}
	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	if auth == nil {
		return
	}
	for source, target := range map[string]string{
		"chatgpt_account_id": "chatgpt_account_id",
		"chatgpt_user_id":    "chatgpt_user_id",
		"chatgpt_plan_type":  "plan_type",
		"organization_id":    "organization_id",
	} {
		if value, ok := auth[source]; ok {
			token[target] = value
		}
	}
}

func parseJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	claims := map[string]any{}
	_ = json.Unmarshal(payload, &claims)
	return claims
}

func googleUserEmail(ctx context.Context, client *http.Client, accessToken string) string {
	if accessToken == "" {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	var out map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return ""
	}
	return stringFromAny(out["email"])
}

func geminiScopes(oauthType string) string {
	if oauthType == "ai_studio" {
		return "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/generative-language.retriever"
	}
	return "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile"
}

func randomCodeVerifier(provider string) (string, error) {
	if provider == "openai" {
		bytes, err := randomBytes(64)
		if err != nil {
			return "", err
		}
		return hex.EncodeToString(bytes), nil
	}
	bytes, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(bytes), "="), nil
}

func randomState() (string, error) {
	bytes, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(bytes), "="), nil
}

func randomHex(n int) (string, error) {
	bytes, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func randomBytes(n int) ([]byte, error) {
	bytes := make([]byte, n)
	_, err := rand.Read(bytes)
	return bytes, err
}

func codeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return strings.TrimRight(base64.URLEncoding.EncodeToString(hash[:]), "=")
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
