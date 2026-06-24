package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"api-monitor/internal/domain"
)

type Service struct {
	client *http.Client
}

type renderedAlert struct {
	Title    string
	Text     string
	Markdown string
	HTML     string
	Values   map[string]string
}

func New(client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Service{client: client}
}

func (s *Service) Send(ctx context.Context, channel domain.NotificationChannel, alert domain.AlertEvent, target *domain.MonitorTarget) (string, error) {
	if !channel.Enabled {
		return "channel disabled", nil
	}
	settings := map[string]any{}
	_ = json.Unmarshal(channel.Settings, &settings)
	rendered := renderAlert(settings, alert, target)

	switch normalizeType(channel.Type) {
	case "dingtalk":
		return s.sendDingTalk(ctx, settings, channel.SecretValue, rendered)
	case "feishu":
		return s.sendFeishu(ctx, settings, channel.SecretValue, rendered)
	case "wecom":
		return s.sendWeCom(ctx, settings, rendered)
	case "webhook":
		return s.sendGenericWebhook(ctx, channel, settings, rendered, alert, target)
	case "phone":
		return s.sendPhoneWebhook(ctx, channel, settings, rendered, alert, target)
	case "email_smtp":
		return s.sendSMTP(ctx, settings, channel.SecretValue, rendered)
	case "sendgrid_email":
		return s.sendSendGrid(ctx, settings, channel.SecretValue, rendered)
	case "twilio_sms":
		return s.sendTwilioSMS(ctx, settings, channel.SecretValue, rendered)
	case "aliyun_sms":
		return s.sendAliyunSMS(ctx, settings, channel.SecretValue, rendered)
	case "tencent_sms":
		return s.sendTencentSMS(ctx, settings, channel.SecretValue, rendered)
	default:
		return "", fmt.Errorf("unsupported notification channel type %q", channel.Type)
	}
}

func normalizeType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "dingding":
		return "dingtalk"
	case "lark":
		return "feishu"
	case "wechat_work", "enterprise_wechat":
		return "wecom"
	case "smtp", "email":
		return "email_smtp"
	case "sendgrid":
		return "sendgrid_email"
	case "sms_twilio":
		return "twilio_sms"
	case "sms_aliyun":
		return "aliyun_sms"
	case "sms_tencent":
		return "tencent_sms"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func (s *Service) sendDingTalk(ctx context.Context, settings map[string]any, secret string, rendered renderedAlert) (string, error) {
	webhook := stringSetting(settings, "webhookUrl", "webhook", "url")
	if webhook == "" {
		return "", errors.New("DingTalk webhookUrl is required")
	}
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]any{
			"title": rendered.Title,
			"text":  rendered.Markdown,
		},
	}
	atMobiles := stringSliceSetting(settings, "atMobiles")
	if len(atMobiles) > 0 || boolSetting(settings, "isAtAll") {
		payload["at"] = map[string]any{
			"atMobiles": atMobiles,
			"isAtAll":   boolSetting(settings, "isAtAll"),
		}
	}
	return s.postJSON(ctx, signedDingTalkURL(webhook, secret), nil, payload)
}

func (s *Service) sendFeishu(ctx context.Context, settings map[string]any, secret string, rendered renderedAlert) (string, error) {
	webhook := stringSetting(settings, "webhookUrl", "webhook", "url")
	if webhook == "" {
		return "", errors.New("Feishu webhookUrl is required")
	}
	if template := stringSetting(settings, "cardTemplate"); template != "" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(applyTemplate(template, rendered.Values)), &payload); err != nil {
			return "", fmt.Errorf("invalid Feishu cardTemplate JSON: %w", err)
		}
		addFeishuSignature(payload, secret)
		return s.postJSON(ctx, webhook, nil, payload)
	}
	payload := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"config": map[string]any{"wide_screen_mode": true},
			"header": map[string]any{
				"title":    map[string]any{"tag": "plain_text", "content": rendered.Title},
				"template": feishuTemplate(stringSetting(settings, "severity")),
			},
			"elements": []map[string]any{
				{"tag": "markdown", "content": rendered.Markdown},
			},
		},
	}
	addFeishuSignature(payload, secret)
	return s.postJSON(ctx, webhook, nil, payload)
}

func (s *Service) sendWeCom(ctx context.Context, settings map[string]any, rendered renderedAlert) (string, error) {
	webhook := stringSetting(settings, "webhookUrl", "webhook", "url")
	if webhook == "" {
		return "", errors.New("WeCom webhookUrl is required")
	}
	payload := map[string]any{
		"msgtype":  "markdown",
		"markdown": map[string]any{"content": rendered.Markdown},
	}
	return s.postJSON(ctx, webhook, nil, payload)
}

func (s *Service) sendGenericWebhook(ctx context.Context, channel domain.NotificationChannel, settings map[string]any, rendered renderedAlert, alert domain.AlertEvent, target *domain.MonitorTarget) (string, error) {
	webhook := stringSetting(settings, "webhookUrl", "webhook", "url")
	if webhook == "" {
		webhook = channel.SecretValue
	}
	if webhook == "" {
		return "", errors.New("webhookUrl is required")
	}
	headers := map[string]string{}
	authHeader := stringSetting(settings, "authHeader")
	if authHeader != "" && channel.SecretValue != "" {
		headers[authHeader] = channel.SecretValue
	}
	if template := stringSetting(settings, "jsonTemplate"); template != "" {
		var payload any
		if err := json.Unmarshal([]byte(applyTemplate(template, rendered.Values)), &payload); err != nil {
			return "", fmt.Errorf("invalid jsonTemplate JSON: %w", err)
		}
		return s.postJSON(ctx, webhook, headers, payload)
	}
	payload := map[string]any{
		"event":    "api_monitor_alert",
		"title":    rendered.Title,
		"text":     rendered.Text,
		"markdown": rendered.Markdown,
		"html":     rendered.HTML,
		"alert":    alert,
		"target":   target,
	}
	return s.postJSON(ctx, webhook, headers, payload)
}

func (s *Service) sendPhoneWebhook(ctx context.Context, channel domain.NotificationChannel, settings map[string]any, rendered renderedAlert, alert domain.AlertEvent, target *domain.MonitorTarget) (string, error) {
	webhook := stringSetting(settings, "webhookUrl", "webhook", "url")
	if webhook == "" {
		return "", errors.New("phone service webhookUrl is required")
	}
	headers := map[string]string{}
	if channel.SecretValue != "" {
		headers["Authorization"] = "Bearer " + channel.SecretValue
	}
	payload := map[string]any{
		"event":                   "api_monitor_phone_alert",
		"provider":                stringSetting(settings, "phoneProvider", "provider"),
		"phoneNumbers":            stringSliceSetting(settings, "phoneNumbers"),
		"callTemplate":            stringSetting(settings, "callTemplate", "template"),
		"region":                  stringSetting(settings, "region"),
		"retryCount":              intSetting(settings, "retryCount"),
		"escalateAfterMinutes":    intSetting(settings, "escalateAfterMinutes"),
		"title":                   rendered.Title,
		"message":                 rendered.Text,
		"recommendedTTS":          rendered.Text,
		"recommendedMarkdownText": rendered.Markdown,
		"alert":                   alert,
		"target":                  target,
	}
	return s.postJSON(ctx, webhook, headers, payload)
}

func (s *Service) sendSMTP(ctx context.Context, settings map[string]any, password string, rendered renderedAlert) (string, error) {
	host := stringSetting(settings, "smtpHost")
	if host == "" {
		return "", errors.New("smtpHost is required")
	}
	port := intSettingDefault(settings, "smtpPort", 587)
	from := stringSetting(settings, "fromEmail", "smtpFrom")
	if from == "" {
		return "", errors.New("fromEmail is required")
	}
	to := stringSliceSetting(settings, "toEmails", "smtpTo")
	if len(to) == 0 {
		return "", errors.New("toEmails is required")
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if boolSetting(settings, "smtpUseTLS") {
		conn, err = tls.DialWithDialer(&dialer, "tcp", addr, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: boolSetting(settings, "smtpSkipVerify"),
		})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return "", err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return "", err
	}
	defer client.Quit()

	if !boolSetting(settings, "smtpUseTLS") && boolSettingDefault(settings, "smtpStartTLS", true) {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{
				ServerName:         host,
				InsecureSkipVerify: boolSetting(settings, "smtpSkipVerify"),
			}); err != nil {
				return "", err
			}
		}
	}

	username := stringSetting(settings, "smtpUsername")
	if username != "" || password != "" {
		if err := client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
			return "", err
		}
	}
	if err := client.Mail(from); err != nil {
		return "", err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return "", err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return "", err
	}
	if _, err := writer.Write(emailMessage(settings, from, to, rendered)); err != nil {
		_ = writer.Close()
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return fmt.Sprintf("sent email to %d recipient(s)", len(to)), nil
}

func (s *Service) sendSendGrid(ctx context.Context, settings map[string]any, apiKey string, rendered renderedAlert) (string, error) {
	if apiKey == "" {
		return "", errors.New("SendGrid API key is required")
	}
	fromEmail := stringSetting(settings, "fromEmail")
	toEmails := stringSliceSetting(settings, "toEmails")
	if fromEmail == "" || len(toEmails) == 0 {
		return "", errors.New("fromEmail and toEmails are required")
	}
	to := make([]map[string]string, 0, len(toEmails))
	for _, email := range toEmails {
		to = append(to, map[string]string{"email": email})
	}
	payload := map[string]any{
		"personalizations": []map[string]any{{"to": to, "subject": rendered.Title}},
		"from": map[string]string{
			"email": fromEmail,
			"name":  stringSetting(settings, "fromName"),
		},
		"content": []map[string]string{
			{"type": "text/plain", "value": rendered.Text},
			{"type": "text/html", "value": rendered.HTML},
		},
	}
	return s.postJSON(ctx, stringSettingDefault(settings, "endpoint", "https://api.sendgrid.com/v3/mail/send"), map[string]string{
		"Authorization": "Bearer " + apiKey,
	}, payload)
}

func (s *Service) sendTwilioSMS(ctx context.Context, settings map[string]any, authToken string, rendered renderedAlert) (string, error) {
	accountSID := stringSetting(settings, "accountSid")
	if accountSID == "" || authToken == "" {
		return "", errors.New("Twilio accountSid and auth token are required")
	}
	toNumbers := stringSliceSetting(settings, "toNumbers", "phoneNumbers")
	if len(toNumbers) == 0 {
		return "", errors.New("toNumbers is required")
	}
	fromNumber := stringSetting(settings, "fromNumber")
	messagingServiceSID := stringSetting(settings, "messagingServiceSid")
	if fromNumber == "" && messagingServiceSID == "" {
		return "", errors.New("fromNumber or messagingServiceSid is required")
	}
	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", url.PathEscape(accountSID))
	responses := make([]string, 0, len(toNumbers))
	for _, to := range toNumbers {
		form := url.Values{}
		form.Set("To", to)
		form.Set("Body", rendered.Text)
		if messagingServiceSID != "" {
			form.Set("MessagingServiceSid", messagingServiceSID)
		} else {
			form.Set("From", fromNumber)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return "", err
		}
		req.SetBasicAuth(accountSID, authToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		body, err := s.do(req)
		if err != nil {
			return strings.Join(responses, "\n"), err
		}
		responses = append(responses, body)
	}
	return strings.Join(responses, "\n"), nil
}

func (s *Service) sendAliyunSMS(ctx context.Context, settings map[string]any, accessKeySecret string, rendered renderedAlert) (string, error) {
	accessKeyID := stringSetting(settings, "accessKeyId")
	if accessKeyID == "" || accessKeySecret == "" {
		return "", errors.New("Aliyun accessKeyId and accessKeySecret are required")
	}
	to := strings.Join(stringSliceSetting(settings, "toNumbers", "phoneNumbers"), ",")
	if to == "" {
		return "", errors.New("toNumbers is required")
	}
	params := map[string]string{
		"Format":           "JSON",
		"Version":          stringSettingDefault(settings, "version", "2018-05-01"),
		"AccessKeyId":      accessKeyID,
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"SignatureVersion": "1.0",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"Action":           stringSettingDefault(settings, "action", "SendMessageWithTemplate"),
		"To":               to,
		"From":             stringSetting(settings, "from", "senderId", "signName"),
		"TemplateCode":     stringSetting(settings, "templateCode"),
		"TemplateParam":    templateParam(settings, rendered),
	}
	if params["From"] == "" || params["TemplateCode"] == "" {
		return "", errors.New("Aliyun from/signName and templateCode are required")
	}
	endpoint := stringSettingDefault(settings, "endpoint", "https://dysmsapi.ap-southeast-1.aliyuncs.com/")
	signedURL := signAliyunURL(endpoint, params, accessKeySecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return "", err
	}
	return s.do(req)
}

func (s *Service) sendTencentSMS(ctx context.Context, settings map[string]any, secretKey string, rendered renderedAlert) (string, error) {
	secretID := stringSetting(settings, "secretId")
	if secretID == "" || secretKey == "" {
		return "", errors.New("Tencent secretId and secretKey are required")
	}
	body := map[string]any{
		"SmsSdkAppId":      stringSetting(settings, "smsSdkAppId"),
		"SignName":         stringSetting(settings, "signName"),
		"TemplateId":       stringSetting(settings, "templateId"),
		"TemplateParamSet": stringSliceSettingDefault(settings, "templateParamSet", []string{rendered.Text}),
		"PhoneNumberSet":   stringSliceSetting(settings, "toNumbers", "phoneNumbers"),
	}
	if body["SmsSdkAppId"] == "" || body["SignName"] == "" || body["TemplateId"] == "" || len(body["PhoneNumberSet"].([]string)) == 0 {
		return "", errors.New("Tencent smsSdkAppId, signName, templateId and toNumbers are required")
	}
	data, _ := json.Marshal(body)
	endpointHost := stringSettingDefault(settings, "endpointHost", "sms.intl.tencentcloudapi.com")
	endpoint := "https://" + endpointHost
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	signTencentRequest(req, endpointHost, "sms", "SendSms", stringSettingDefault(settings, "version", "2021-01-11"), stringSettingDefault(settings, "region", "ap-singapore"), secretID, secretKey, data)
	return s.do(req)
}

func (s *Service) postJSON(ctx context.Context, endpoint string, headers map[string]string, payload any) (string, error) {
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		if key != "" && value != "" {
			req.Header.Set(key, value)
		}
	}
	return s.do(req)
}

func (s *Service) do(req *http.Request) (string, error) {
	res, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return string(body), fmt.Errorf("status %d", res.StatusCode)
	}
	return string(body), nil
}

func renderAlert(settings map[string]any, alert domain.AlertEvent, target *domain.MonitorTarget) renderedAlert {
	values := templateValues(alert, target)
	title := applyTemplate(stringSettingDefault(settings, "titleTemplate", "{{title}}"), values)
	defaultMarkdown := defaultMarkdown(values)
	defaultText := defaultText(values)
	markdown := applyTemplate(stringSettingDefault(settings, "markdownTemplate", defaultMarkdown), values)
	text := applyTemplate(stringSettingDefault(settings, "textTemplate", defaultText), values)
	htmlTemplate := stringSetting(settings, "htmlTemplate")
	htmlText := strings.ReplaceAll(html.EscapeString(text), "\n", "<br>")
	if htmlTemplate != "" {
		htmlText = applyTemplate(htmlTemplate, values)
	}
	return renderedAlert{Title: title, Markdown: markdown, Text: text, HTML: htmlText, Values: values}
}

func templateValues(alert domain.AlertEvent, target *domain.MonitorTarget) map[string]string {
	values := map[string]string{
		"title":      alert.Title,
		"message":    alert.Message,
		"severity":   alert.Severity,
		"status":     alert.Status,
		"openedAt":   alert.OpenedAt.Format(time.RFC3339),
		"targetName": "-",
		"provider":   "-",
		"group":      "-",
		"balance":    "-",
		"quota":      "-",
		"health":     "-",
	}
	if target != nil {
		values["targetName"] = target.Name
		values["provider"] = string(target.ProviderKind)
		values["group"] = target.GroupName
		values["health"] = string(target.Status)
		if target.Balance != nil {
			values["balance"] = fmt.Sprintf("%.4f %s", target.Balance.Amount, target.Balance.Currency)
		}
		if target.Quota != nil && target.Quota.Remaining != nil {
			values["quota"] = fmt.Sprintf("%.4f %s", *target.Quota.Remaining, target.Quota.Unit)
		}
	}
	return values
}

func applyTemplate(template string, values map[string]string) string {
	out := template
	for key, value := range values {
		out = strings.ReplaceAll(out, "{{"+key+"}}", value)
	}
	return out
}

func defaultMarkdown(values map[string]string) string {
	return strings.Join([]string{
		"## {{title}}",
		"",
		"{{message}}",
		"",
		"- Severity: `{{severity}}`",
		"- Asset: `{{targetName}}`",
		"- Provider: `{{provider}}`",
		"- Group: `{{group}}`",
		"- Balance: `{{balance}}`",
		"- Remaining quota: `{{quota}}`",
		"- Opened: `{{openedAt}}`",
	}, "\n")
}

func defaultText(values map[string]string) string {
	return strings.Join([]string{
		"{{title}}",
		"{{message}}",
		"Severity: {{severity}}",
		"Asset: {{targetName}}",
		"Provider: {{provider}}",
		"Group: {{group}}",
		"Balance: {{balance}}",
		"Remaining quota: {{quota}}",
		"Opened: {{openedAt}}",
	}, "\n")
}

func signedDingTalkURL(webhook string, secret string) string {
	if secret == "" {
		return webhook
	}
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	message := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	sep := "?"
	if strings.Contains(webhook, "?") {
		sep = "&"
	}
	return webhook + sep + "timestamp=" + url.QueryEscape(timestamp) + "&sign=" + url.QueryEscape(sign)
}

func addFeishuSignature(payload map[string]any, secret string) {
	if secret == "" {
		return
	}
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	payload["timestamp"] = timestamp
	payload["sign"] = sign
}

func signAliyunURL(endpoint string, params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		if params[key] == "" {
			continue
		}
		pairs = append(pairs, percentEncode(key)+"="+percentEncode(params[key]))
	}
	canonical := strings.Join(pairs, "&")
	stringToSign := "GET&%2F&" + percentEncode(canonical)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	return strings.TrimRight(endpoint, "?&") + sep + canonical + "&Signature=" + percentEncode(signature)
}

func percentEncode(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func signTencentRequest(req *http.Request, host, service, action, version, region, secretID, secretKey string, payload []byte) {
	now := time.Now().UTC()
	timestamp := now.Unix()
	date := now.Format("2006-01-02")
	algorithm := "TC3-HMAC-SHA256"
	contentType := "application/json; charset=utf-8"
	canonicalHeaders := "content-type:" + contentType + "\n" + "host:" + host + "\n"
	signedHeaders := "content-type;host"
	hash := sha256Hex(payload)
	canonicalRequest := strings.Join([]string{"POST", "/", "", canonicalHeaders, signedHeaders, hash}, "\n")
	credentialScope := date + "/" + service + "/tc3_request"
	stringToSign := strings.Join([]string{algorithm, strconv.FormatInt(timestamp, 10), credentialScope, sha256Hex([]byte(canonicalRequest))}, "\n")
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s", algorithm, secretID, credentialScope, signedHeaders, signature)

	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-TC-Region", region)
}

func emailMessage(settings map[string]any, from string, to []string, rendered renderedAlert) []byte {
	fromName := stringSetting(settings, "fromName", "smtpFromName")
	fromHeader := from
	if fromName != "" {
		fromHeader = mime.QEncoding.Encode("utf-8", fromName) + " <" + from + ">"
	}
	headers := map[string]string{
		"From":         fromHeader,
		"To":           strings.Join(to, ", "),
		"Subject":      mime.QEncoding.Encode("utf-8", rendered.Title),
		"MIME-Version": "1.0",
		"Content-Type": `text/html; charset="UTF-8"`,
	}
	var buf bytes.Buffer
	for key, value := range headers {
		buf.WriteString(key + ": " + value + "\r\n")
	}
	buf.WriteString("\r\n")
	buf.WriteString(rendered.HTML)
	return buf.Bytes()
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func templateParam(settings map[string]any, rendered renderedAlert) string {
	value := stringSetting(settings, "templateParam", "templateParamJson")
	if value != "" {
		return value
	}
	data, _ := json.Marshal(map[string]string{"title": rendered.Title, "message": rendered.Text})
	return string(data)
}

func feishuTemplate(severity string) string {
	switch severity {
	case "critical", "phone":
		return "red"
	case "warning":
		return "orange"
	default:
		return "blue"
	}
}

func stringSetting(settings map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := settings[key].(string); ok {
			return value
		}
	}
	return ""
}

func stringSettingDefault(settings map[string]any, key, fallback string) string {
	if value := stringSetting(settings, key); value != "" {
		return value
	}
	return fallback
}

func stringSliceSetting(settings map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := settings[key]
		if !ok {
			continue
		}
		if items, ok := value.([]any); ok {
			out := make([]string, 0, len(items))
			for _, item := range items {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					out = append(out, strings.TrimSpace(text))
				}
			}
			return out
		}
		if items, ok := value.([]string); ok {
			return items
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			parts := strings.Split(text, ",")
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					out = append(out, trimmed)
				}
			}
			return out
		}
	}
	return nil
}

func stringSliceSettingDefault(settings map[string]any, key string, fallback []string) []string {
	if values := stringSliceSetting(settings, key); len(values) > 0 {
		return values
	}
	return fallback
}

func intSetting(settings map[string]any, key string) int {
	return intSettingDefault(settings, key, 0)
}

func intSettingDefault(settings map[string]any, key string, fallback int) int {
	value, ok := settings[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		if parsed, err := strconv.Atoi(typed); err == nil {
			return parsed
		}
	}
	return fallback
}

func boolSetting(settings map[string]any, key string) bool {
	return boolSettingDefault(settings, key, false)
}

func boolSettingDefault(settings map[string]any, key string, fallback bool) bool {
	value, ok := settings[key]
	if !ok {
		return fallback
	}
	if typed, ok := value.(bool); ok {
		return typed
	}
	if typed, ok := value.(string); ok {
		parsed, err := strconv.ParseBool(typed)
		if err == nil {
			return parsed
		}
	}
	return fallback
}
