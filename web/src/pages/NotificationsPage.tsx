import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Copy, Info, Lock, Plus, RotateCcw, Save, Send } from "lucide-react";
import { channelsApi } from "@/api/services";
import { AppShell } from "@/components/layout/AppShell";
import { Button } from "@/components/ui/Button";
import { ErrorState, LoadingSkeleton } from "@/components/ui/State";
import type { NotificationChannel } from "@/lib/types";

const CHANNEL_TYPES = [
  "dingtalk",
  "feishu",
  "wecom",
  "webhook",
  "phone",
  "email_smtp",
  "sendgrid_email",
  "twilio_sms",
  "aliyun_sms",
  "tencent_sms",
] as const;

type ChannelType = (typeof CHANNEL_TYPES)[number];
type FieldKind =
  | "text"
  | "password"
  | "textarea"
  | "number"
  | "checkbox"
  | "list";
type TemplateKey =
  | "card"
  | "markdown"
  | "text"
  | "html"
  | "sms"
  | "voice"
  | "json";
type PreviewTab = "message" | "vars";

type FieldSpec = {
  key: string;
  label: string;
  kind?: FieldKind;
  required?: boolean;
  placeholder?: string;
  defaultValue?: string | number | boolean;
  help: string;
};

type ChannelSpec = {
  type: ChannelType;
  group: "IM" | "HTTP" | "Email" | "SMS" | "Voice";
  emoji: string;
  accent: string;
  secretLabel: string;
  secretRequired?: boolean;
  help: string;
  sendPath: string;
  fields: FieldSpec[];
  templates: TemplateKey[];
};

type ChannelForm = {
  id?: string;
  name: string;
  type: ChannelType;
  enabled: boolean;
  secretValue: string;
  settings: Record<string, string | number | boolean>;
};

const templateVars = [
  "title",
  "message",
  "severity",
  "status",
  "openedAt",
  "targetName",
  "provider",
  "group",
  "balance",
  "quota",
  "health",
];

const variableDescriptions = [
  ["title", "告警标题", "[CRITICAL] openai-relay-prod 余额严重不足"],
  ["message", "告警正文", "余额 $8.42 低于阈值 $10.00，连续 2 次命中"],
  ["severity", "严重级别", "critical / warning / info"],
  ["status", "告警状态", "open / resolved / silenced"],
  ["openedAt", "触发时间", "2026-06-24T14:40:01+08:00"],
  ["targetName", "资产名称", "openai-relay-prod"],
  ["provider", "上游类型", "openai-official"],
  ["group", "所属分组", "production"],
  ["balance", "当前余额", "$8.42"],
  ["quota", "剩余额度", "2.1%"],
  ["health", "健康状态", "critical / degraded / healthy"],
];

const defaultTemplates: Record<TemplateKey, string> = {
  card: `{
  "msg_type": "interactive",
  "card": {
    "header": {
      "title": { "content": "{{title}}", "tag": "plain_text" },
      "template": "red"
    },
    "elements": [
      { "tag": "div", "text": { "content": "{{message}}", "tag": "lark_md" } }
    ]
  }
}`,
  markdown: `## 🚨 {{title}}

**级别**: {{severity}} | **状态**: {{status}}

{{message}}

| 字段 | 值 |
|------|----|
| 资产 | {{targetName}} |
| 上游 | {{provider}} |
| 分组 | {{group}} |
| 余额 | {{balance}} |
| 额度 | {{quota}} |
| 时间 | {{openedAt}} |

请登录 API Monitor 控制台处理。`,
  text: `[API Monitor] {{severity}}: {{title}}
{{message}}
资产: {{targetName}} | 上游: {{provider}} | 余额: {{balance}} | 额度: {{quota}}
时间: {{openedAt}}`,
  html: `<h2>{{title}}</h2>
<p>{{message}}</p>
<table>
  <tr><td>资产</td><td>{{targetName}}</td></tr>
  <tr><td>余额</td><td>{{balance}}</td></tr>
  <tr><td>额度</td><td>{{quota}}</td></tr>
  <tr><td>时间</td><td>{{openedAt}}</td></tr>
</table>`,
  sms: `【API Monitor】{{severity}}: {{targetName}} 余额{{balance}}，剩余额度{{quota}}，请立即登录控制台处理。`,
  voice: `API Monitor {{severity}}告警。资产 {{targetName}} 当前余额 {{balance}}，剩余额度 {{quota}}。请立即登录控制台处理。`,
  json: `{
  "event": "alert.triggered",
  "title": "{{title}}",
  "message": "{{message}}",
  "severity": "{{severity}}",
  "status": "{{status}}",
  "openedAt": "{{openedAt}}",
  "target": {
    "name": "{{targetName}}",
    "provider": "{{provider}}",
    "group": "{{group}}",
    "balance": "{{balance}}",
    "quota": "{{quota}}",
    "health": "{{health}}"
  }
}`,
};

const specs: Record<ChannelType, ChannelSpec> = {
  dingtalk: {
    type: "dingtalk",
    group: "IM",
    emoji: "📫",
    accent: "rgba(59,130,246,.14)",
    secretLabel: "Secret 签名密钥",
    help: "钉钉群机器人 Markdown，支持官方加签。Webhook 从群机器人设置中复制，Secret 填机器人安全设置里的加签密钥。",
    sendPath: "POST markdown JSON + HMAC-SHA256 sign",
    fields: [
      {
        key: "webhookUrl",
        label: "Webhook URL",
        required: true,
        placeholder: "https://oapi.dingtalk.com/robot/send?access_token=...",
        help: "钉钉机器人完整 Webhook 地址。",
      },
      {
        key: "atMobiles",
        label: "@手机号",
        kind: "list",
        placeholder: "13800000000, 13900000000",
        help: "可选。多个手机号用英文逗号分隔。",
      },
      {
        key: "isAtAll",
        label: "@所有人",
        kind: "checkbox",
        help: "启用后 payload 会带 isAtAll=true。",
      },
    ],
    templates: ["markdown", "text"],
  },
  feishu: {
    type: "feishu",
    group: "IM",
    emoji: "🪁",
    accent: "rgba(99,102,241,.14)",
    secretLabel: "签名校验 Secret",
    help: "飞书 / Lark 自定义机器人，默认发送 interactive card，并支持官方签名校验。",
    sendPath: "POST interactive card JSON + timestamp/sign",
    fields: [
      {
        key: "webhookUrl",
        label: "Webhook URL",
        required: true,
        placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/...",
        help: "飞书群机器人完整 Webhook 地址。",
      },
      {
        key: "severity",
        label: "卡片颜色级别",
        placeholder: "warning / critical / phone",
        help: "critical/phone 使用红色，warning 使用橙色，其他为蓝色。",
      },
    ],
    templates: ["card", "markdown", "text"],
  },
  wecom: {
    type: "wecom",
    group: "IM",
    emoji: "💬",
    accent: "rgba(34,197,94,.14)",
    secretLabel: "保留字段",
    help: "企业微信群机器人 Markdown。Webhook URL 自带 key，通常不需要额外密钥。",
    sendPath: "POST markdown JSON",
    fields: [
      {
        key: "webhookUrl",
        label: "Webhook URL",
        required: true,
        placeholder: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...",
        help: "企业微信群机器人完整 Webhook 地址。",
      },
    ],
    templates: ["markdown", "text"],
  },
  webhook: {
    type: "webhook",
    group: "HTTP",
    emoji: "🔗",
    accent: "rgba(113,113,122,.18)",
    secretLabel: "Header Token",
    help: "通用 JSON Webhook，适合自建网关、Serverless、自动化平台。",
    sendPath: "POST application/json",
    fields: [
      {
        key: "webhookUrl",
        label: "Webhook URL",
        required: true,
        placeholder: "https://example.com/api-monitor-alert",
        help: "接收 API Monitor 告警 payload 的 HTTP 地址。",
      },
      {
        key: "authHeader",
        label: "认证 Header",
        placeholder: "Authorization",
        help: "可选。后端会把密钥写入这个 Header。",
      },
    ],
    templates: ["json", "markdown", "text", "html"],
  },
  phone: {
    type: "phone",
    group: "Voice",
    emoji: "📞",
    accent: "rgba(236,72,153,.14)",
    secretLabel: "电话服务 Token",
    help: "电话升级 Webhook。后端把号码、模板、重试策略传给你的语音外呼服务。",
    sendPath: "POST phone escalation JSON",
    fields: [
      {
        key: "webhookUrl",
        label: "电话服务 Webhook",
        required: true,
        placeholder: "https://voice.example.com/call",
        help: "你的外呼网关入口。",
      },
      {
        key: "phoneProvider",
        label: "外呼服务商",
        placeholder: "阿里云语音 / 腾讯云语音 / 自建服务",
        help: "传给电话服务用于路由。",
      },
      {
        key: "phoneNumbers",
        label: "接收号码池",
        kind: "list",
        required: true,
        placeholder: "+8613800000000, +8613900000000",
        help: "多个号码用英文逗号分隔。",
      },
      {
        key: "retryCount",
        label: "重试次数",
        kind: "number",
        defaultValue: 1,
        help: "传给外呼服务的建议重试次数。",
      },
      {
        key: "escalateAfterMinutes",
        label: "升级等待（分钟）",
        kind: "number",
        defaultValue: 5,
        help: "传给外呼服务的建议升级等待。",
      },
    ],
    templates: ["voice", "text"],
  },
  email_smtp: {
    type: "email_smtp",
    group: "Email",
    emoji: "✉️",
    accent: "rgba(59,130,246,.14)",
    secretLabel: "SMTP 密码 / 授权码",
    secretRequired: true,
    help: "标准 SMTP 邮件，支持 STARTTLS 或隐式 TLS。",
    sendPath: "SMTP AUTH + HTML email",
    fields: [
      {
        key: "smtpHost",
        label: "SMTP 主机",
        required: true,
        placeholder: "smtp.example.com",
        help: "SMTP 服务器域名。",
      },
      {
        key: "smtpPort",
        label: "端口",
        kind: "number",
        required: true,
        defaultValue: 587,
        help: "587 STARTTLS，465 隐式 TLS。",
      },
      {
        key: "smtpUsername",
        label: "用户名",
        placeholder: "alerts@example.com",
        help: "SMTP AUTH 用户名。",
      },
      {
        key: "fromEmail",
        label: "发件人地址",
        required: true,
        placeholder: "alerts@example.com",
        help: "邮件 From 地址。",
      },
      {
        key: "fromName",
        label: "发件人名称",
        placeholder: "API Monitor",
        help: "显示在收件箱里的发件人名称。",
      },
      {
        key: "toEmails",
        label: "收件人地址",
        kind: "list",
        required: true,
        placeholder: "ops@example.com, finance@example.com",
        help: "多个地址用英文逗号分隔。",
      },
      {
        key: "smtpStartTLS",
        label: "使用 STARTTLS",
        kind: "checkbox",
        defaultValue: true,
        help: "587 端口通常开启。",
      },
      {
        key: "smtpUseTLS",
        label: "隐式 TLS",
        kind: "checkbox",
        help: "465 端口通常开启。",
      },
    ],
    templates: ["html", "text"],
  },
  sendgrid_email: {
    type: "sendgrid_email",
    group: "Email",
    emoji: "📧",
    accent: "rgba(99,102,241,.14)",
    secretLabel: "SendGrid API Key",
    secretRequired: true,
    help: "SendGrid Mail Send API，使用 Bearer API Key 调用 /v3/mail/send。",
    sendPath: "POST /v3/mail/send JSON",
    fields: [
      {
        key: "fromEmail",
        label: "发件人地址",
        required: true,
        placeholder: "alerts@example.com",
        help: "SendGrid 已验证发件人。",
      },
      {
        key: "fromName",
        label: "发件人名称",
        placeholder: "API Monitor",
        help: "显示在收件箱里的名称。",
      },
      {
        key: "toEmails",
        label: "收件人地址",
        kind: "list",
        required: true,
        placeholder: "ops@example.com, finance@example.com",
        help: "多个地址用英文逗号分隔。",
      },
      {
        key: "endpoint",
        label: "API Endpoint",
        placeholder: "https://api.sendgrid.com/v3/mail/send",
        help: "默认官方端点。代理环境可覆盖。",
      },
    ],
    templates: ["html", "text"],
  },
  twilio_sms: {
    type: "twilio_sms",
    group: "SMS",
    emoji: "💬",
    accent: "rgba(34,197,94,.14)",
    secretLabel: "Twilio Auth Token",
    secretRequired: true,
    help: "Twilio Messages API，Account SID + Auth Token 做 Basic Auth。",
    sendPath: "POST form Messages API",
    fields: [
      {
        key: "accountSid",
        label: "Account SID",
        required: true,
        placeholder: "ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
        help: "Twilio 账号 SID。",
      },
      {
        key: "fromNumber",
        label: "From 号码",
        placeholder: "+14155550100",
        help: "Twilio 发送号码。与 Messaging Service 二选一。",
      },
      {
        key: "messagingServiceSid",
        label: "Messaging Service SID",
        placeholder: "MGxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
        help: "可选。",
      },
      {
        key: "toNumbers",
        label: "To 号码",
        kind: "list",
        required: true,
        placeholder: "+8613800000000, +16505550123",
        help: "E.164 手机号，多个用英文逗号分隔。",
      },
    ],
    templates: ["sms", "text"],
  },
  aliyun_sms: {
    type: "aliyun_sms",
    group: "SMS",
    emoji: "📱",
    accent: "rgba(245,158,11,.14)",
    secretLabel: "AccessKey Secret",
    secretRequired: true,
    help: "阿里云短信签名请求，使用 AccessKeyId/Secret 和短信模板。",
    sendPath: "GET signed query HMAC-SHA1",
    fields: [
      {
        key: "accessKeyId",
        label: "AccessKey ID",
        required: true,
        placeholder: "LTAI...",
        help: "阿里云 AccessKey ID。",
      },
      {
        key: "signName",
        label: "短信签名",
        required: true,
        placeholder: "API Monitor",
        help: "已审核签名。",
      },
      {
        key: "templateCode",
        label: "模板 Code",
        required: true,
        placeholder: "SMS_123456789",
        help: "已审核模板 Code。",
      },
      {
        key: "templateParam",
        label: "模板参数 JSON",
        kind: "textarea",
        placeholder: '{"title":"{{title}}"}',
        help: "可选。留空时后端传 title/message。",
      },
      {
        key: "toNumbers",
        label: "To 号码",
        kind: "list",
        required: true,
        placeholder: "8613800000000, 6512345678",
        help: "多个号码用英文逗号分隔。",
      },
    ],
    templates: ["sms", "text"],
  },
  tencent_sms: {
    type: "tencent_sms",
    group: "SMS",
    emoji: "📱",
    accent: "rgba(245,158,11,.14)",
    secretLabel: "SecretKey",
    secretRequired: true,
    help: "腾讯云 SendSms，使用 SecretId/SecretKey 计算 TC3-HMAC-SHA256 签名。",
    sendPath: "POST SendSms JSON + TC3 signature",
    fields: [
      {
        key: "secretId",
        label: "SecretId",
        required: true,
        placeholder: "AKID...",
        help: "腾讯云 SecretId。",
      },
      {
        key: "smsSdkAppId",
        label: "SmsSdkAppId",
        required: true,
        placeholder: "1400000000",
        help: "短信应用 ID。",
      },
      {
        key: "signName",
        label: "短信签名",
        required: true,
        placeholder: "API Monitor",
        help: "已审核签名。",
      },
      {
        key: "templateId",
        label: "模板 ID",
        required: true,
        placeholder: "1234567",
        help: "已审核短信模板 ID。",
      },
      {
        key: "templateParamSet",
        label: "模板参数",
        kind: "list",
        placeholder: "{{title}}, {{message}}",
        help: "按模板变量顺序填写。",
      },
      {
        key: "toNumbers",
        label: "To 号码",
        kind: "list",
        required: true,
        placeholder: "+8613800000000, +6512345678",
        help: "E.164 手机号。",
      },
    ],
    templates: ["sms", "text"],
  },
};

const sampleValues: Record<string, string> = {
  title: "[CRITICAL] openai-relay-prod 余额严重不足",
  message:
    "openai-relay-prod 余额严重不足，当前余额 $8.42，剩余额度 2.1%，已低于阈值 $10.00，需立即处理。",
  severity: "critical",
  status: "open",
  openedAt: "2026-06-24 14:40:01",
  targetName: "openai-relay-prod",
  provider: "OpenAI Official",
  group: "production",
  balance: "$8.42",
  quota: "2.1%",
  health: "critical",
};

function normalizeType(value: string): ChannelType {
  return CHANNEL_TYPES.includes(value as ChannelType)
    ? (value as ChannelType)
    : "webhook";
}

function defaultName(type: ChannelType, t: (key: string) => string) {
  return t(`channelType.${type}`);
}

function formForType(
  type: ChannelType,
  t: (key: string) => string,
): ChannelForm {
  const settings: ChannelForm["settings"] = {
    titleTemplate: defaultTemplates.text.split("\n")[0],
    cardTemplate: defaultTemplates.card,
    jsonTemplate: defaultTemplates.json,
    markdownTemplate: defaultTemplates.markdown,
    textTemplate: defaultTemplates.text,
    htmlTemplate: defaultTemplates.html,
  };
  specs[type].fields.forEach((field) => {
    if (field.defaultValue != null) settings[field.key] = field.defaultValue;
    if (field.kind === "checkbox" && settings[field.key] == null)
      settings[field.key] = false;
    if (field.kind === "number" && settings[field.key] == null)
      settings[field.key] = 0;
  });
  return {
    name: defaultName(type, t),
    type,
    enabled: true,
    secretValue: "",
    settings,
  };
}

function listSetting(channel: NotificationChannel, key: string) {
  const value = channel.settings?.[key];
  if (Array.isArray(value))
    return value
      .filter((item): item is string => typeof item === "string")
      .join(", ");
  return typeof value === "string" ? value : "";
}

function draftFromChannel(
  channel: NotificationChannel,
  t: (key: string) => string,
): ChannelForm {
  const type = normalizeType(channel.type);
  const draft = formForType(type, t);
  draft.id = channel.id;
  draft.name = channel.name;
  draft.enabled = channel.enabled;
  specs[type].fields.forEach((field) => {
    const value = channel.settings?.[field.key];
    if (field.kind === "list")
      draft.settings[field.key] = listSetting(channel, field.key);
    else if (
      typeof value === "string" ||
      typeof value === "number" ||
      typeof value === "boolean"
    ) {
      draft.settings[field.key] = value;
    }
  });
  (
    [
      "titleTemplate",
      "cardTemplate",
      "jsonTemplate",
      "markdownTemplate",
      "textTemplate",
      "htmlTemplate",
    ] as const
  ).forEach((key) => {
    const value = channel.settings?.[key];
    if (typeof value === "string") draft.settings[key] = value;
  });
  return draft;
}

function settingText(settings: ChannelForm["settings"], key: string) {
  const value = settings[key];
  return typeof value === "string" || typeof value === "number"
    ? String(value)
    : "";
}

function splitList(value: unknown) {
  if (typeof value !== "string") return [];
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function buildSettings(form: ChannelForm) {
  const settings: Record<string, unknown> = {};
  specs[form.type].fields.forEach((field) => {
    const value = form.settings[field.key];
    if (field.kind === "list") {
      const list = splitList(value);
      if (list.length || field.required) settings[field.key] = list;
    } else if (field.kind === "number") {
      settings[field.key] = Number(value || 0);
    } else if (field.kind === "checkbox") {
      settings[field.key] = value === true;
    } else if (typeof value === "string" && value.trim()) {
      settings[field.key] = value.trim();
    }
  });
  if (form.type === "aliyun_sms" && typeof settings.signName === "string")
    settings.from = settings.signName;
  settings.titleTemplate = settingText(form.settings, "titleTemplate");
  settings.cardTemplate = settingText(form.settings, "cardTemplate");
  settings.jsonTemplate = settingText(form.settings, "jsonTemplate");
  settings.markdownTemplate = settingText(form.settings, "markdownTemplate");
  settings.textTemplate = settingText(form.settings, "textTemplate");
  settings.htmlTemplate = settingText(form.settings, "htmlTemplate");
  return settings;
}

function renderTemplate(template: string) {
  return Object.entries(sampleValues).reduce(
    (text, [key, value]) => text.replaceAll(`{{${key}}}`, value),
    template,
  );
}

function templateSettingKey(template: TemplateKey) {
  if (template === "html") return "htmlTemplate";
  if (template === "markdown") return "markdownTemplate";
  if (template === "card") return "cardTemplate";
  if (template === "json") return "jsonTemplate";
  return "textTemplate";
}

function templateLabel(template: TemplateKey) {
  const labels: Record<TemplateKey, string> = {
    card: "卡片",
    markdown: "Markdown",
    text: "纯文本",
    html: "HTML",
    sms: "短信",
    voice: "语音",
    json: "JSON",
  };
  return labels[template];
}

export function NotificationsPage() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [activeType, setActiveType] = useState<ChannelType>("feishu");
  const [form, setForm] = useState<ChannelForm>(() => formForType("feishu", t));
  const [templateType, setTemplateType] = useState<TemplateKey>("card");
  const [previewTab, setPreviewTab] = useState<PreviewTab>("message");
  const [lastTest, setLastTest] = useState("");
  const [activeHelp, setActiveHelp] = useState(specs.feishu.help);

  const list = useQuery({ queryKey: ["channels"], queryFn: channelsApi.list });
  const channels = list.data ?? [];
  const spec = specs[activeType];
  const availableTemplates = spec.templates;
  const currentTemplate = currentTemplateValue(form, templateType);

  const saveMut = useMutation({
    mutationFn: () => {
      const body = {
        name: form.name.trim(),
        type: form.type,
        enabled: form.enabled,
        settings: buildSettings(form),
        secretValue: form.secretValue.trim() || undefined,
      };
      return form.id
        ? channelsApi.patch(form.id, body)
        : channelsApi.create(body);
    },
    onSuccess: (channel) => {
      void qc.invalidateQueries({ queryKey: ["channels"] });
      setForm(draftFromChannel(channel, t));
      setLastTest("");
    },
  });

  const testMut = useMutation({
    mutationFn: (id: string) => channelsApi.test(id),
    onSuccess: (data) =>
      setLastTest(
        `测试发送成功：${data.message || data.response || t("notifications.testOk")}`,
      ),
    onError: (err) =>
      setLastTest(
        `测试发送失败：${err instanceof Error ? err.message : t("errors.loadFailed")}`,
      ),
  });

  const draftTestMut = useMutation({
    mutationFn: () =>
      channelsApi.testDraft({
        name: form.name.trim() || t(`channelType.${form.type}`),
        type: form.type,
        enabled: form.enabled,
        settings: buildSettings(form),
        secretValue: form.secretValue.trim() || undefined,
      }),
    onSuccess: (data) =>
      setLastTest(
        `测试发送成功：${data.message || data.response || t("notifications.testOk")}`,
      ),
    onError: (err) =>
      setLastTest(
        `测试发送失败：${err instanceof Error ? err.message : t("errors.loadFailed")}`,
      ),
  });

  const grouped = useMemo(() => {
    const map: Record<ChannelSpec["group"], NotificationChannel[]> = {
      IM: [],
      HTTP: [],
      Email: [],
      SMS: [],
      Voice: [],
    };
    channels.forEach((channel) => {
      map[specs[normalizeType(channel.type)].group].push(channel);
    });
    return map;
  }, [channels]);

  function selectType(type: ChannelType) {
    setActiveType(type);
    setForm(formForType(type, t));
    setTemplateType(specs[type].templates[0]);
    setActiveHelp(specs[type].help);
    setLastTest("");
  }

  function selectChannel(channel: NotificationChannel) {
    const draft = draftFromChannel(channel, t);
    setActiveType(draft.type);
    setForm(draft);
    setTemplateType(specs[draft.type].templates[0]);
    setActiveHelp(specs[draft.type].help);
    setLastTest("");
  }

  function updateSetting(key: string, value: string | number | boolean) {
    setForm((cur) => ({ ...cur, settings: { ...cur.settings, [key]: value } }));
  }

  function updateTemplate(value: string) {
    const key = templateSettingKey(templateType);
    updateSetting(key, value);
  }

  function resetTemplate() {
    const key = templateSettingKey(templateType);
    updateSetting(key, defaultTemplates[templateType]);
  }

  return (
    <AppShell
      title={t("notifications.title")}
      description="配置告警消息路由规则与渠道模板"
      contentClassName="content-edge"
      onRefresh={() => void list.refetch()}
      refreshing={list.isFetching}
      actions={
        <Button
          size="sm"
          variant="primary"
          onClick={() => selectType("feishu")}
        >
          <Plus size={13} />
          添加渠道
        </Button>
      }
    >
      <div className="notif-layout">
        <aside className="notif-panel-l">
          {(["IM", "HTTP", "Email", "SMS", "Voice"] as const).map((group) => (
            <div key={group}>
              <div className="ch-group-label">{groupLabel(group)}</div>
              {grouped[group].map((channel) => {
                const type = normalizeType(channel.type);
                return (
                  <button
                    key={channel.id}
                    type="button"
                    className={`channel-item${form.id === channel.id ? " active" : ""}${!channel.enabled ? " ch-disabled" : ""}`}
                    onClick={() => selectChannel(channel)}
                  >
                    <div
                      className="channel-icon"
                      style={{ background: specs[type].accent }}
                    >
                      {specs[type].emoji}
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="channel-name truncate">
                        {channel.name}
                      </div>
                      <div className="channel-type-lbl">
                        {t(`channelType.${type}`)}
                      </div>
                    </div>
                    <span
                      className={`ch-dot ${channel.enabled ? "ch-dot-ok" : "ch-dot-warn"}`}
                    />
                  </button>
                );
              })}
              {CHANNEL_TYPES.filter((type) => specs[type].group === group).map(
                (type) => (
                  <button
                    key={type}
                    type="button"
                    className={`channel-item channel-blueprint${!form.id && activeType === type ? " active" : ""}`}
                    onClick={() => selectType(type)}
                  >
                    <div
                      className="channel-icon"
                      style={{ background: specs[type].accent }}
                    >
                      {specs[type].emoji}
                    </div>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="channel-name truncate">
                        {t(`channelType.${type}`)}
                      </div>
                      <div className="channel-type-lbl">新建配置</div>
                    </div>
                  </button>
                ),
              )}
            </div>
          ))}
        </aside>

        <section className="notif-panel-m">
          {list.isLoading ? (
            <LoadingSkeleton />
          ) : list.isError ? (
            <ErrorState onRetry={() => void list.refetch()} />
          ) : (
            <>
              <div className="panel-section">
                <div className="panel-section-head">
                  <div>
                    <div style={{ fontSize: 14, fontWeight: 600 }}>
                      {spec.emoji}{" "}
                      {form.id
                        ? form.name
                        : `新建 ${t(`channelType.${activeType}`)}`}
                    </div>
                    <div
                      style={{
                        fontSize: 11,
                        color: "var(--text-3)",
                        marginTop: 2,
                      }}
                    >
                      {t(`channelType.${activeType}`)} · {spec.sendPath}
                      <span
                        className={`badge ${form.enabled ? "badge-ok" : "badge-muted"}`}
                        style={{ marginLeft: 6 }}
                      >
                        {form.enabled
                          ? t("common.enabled")
                          : t("common.disabled")}
                      </span>
                    </div>
                  </div>
                  <label className="toggle" title="启用该渠道">
                    <input
                      type="checkbox"
                      checked={form.enabled}
                      onChange={(event) =>
                        setForm({ ...form, enabled: event.target.checked })
                      }
                    />
                    <span className="toggle-track" />
                  </label>
                </div>
                <div className="panel-section-body">
                  <div className="form-grid form-grid-2">
                    <div className="form-group">
                      <label className="form-label">
                        渠道名称
                        <InfoButton
                          text="在渠道列表、告警规则和通知日志里显示。"
                          onClick={setActiveHelp}
                        />
                      </label>
                      <input
                        className="input"
                        value={form.name}
                        onChange={(event) =>
                          setForm({ ...form, name: event.target.value })
                        }
                        placeholder="飞书-核心告警群"
                      />
                    </div>
                    <div className="form-group">
                      <label className="form-label">
                        渠道类型
                        <InfoButton text={spec.help} onClick={setActiveHelp} />
                      </label>
                      <select
                        className="select"
                        value={activeType}
                        onChange={(event) =>
                          selectType(event.target.value as ChannelType)
                        }
                      >
                        {CHANNEL_TYPES.map((type) => (
                          <option key={type} value={type}>
                            {t(`channelType.${type}`)}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>

                  <div className="form-group">
                    <label className="form-label">
                      {spec.secretLabel}
                      <InfoButton text={spec.help} onClick={setActiveHelp} />
                    </label>
                    <div className="field-row">
                      <div className="input-wrap" style={{ flex: 1 }}>
                        <Lock className="icon-l" size={14} />
                        <input
                          className="input input-mono"
                          type="password"
                          value={form.secretValue}
                          onChange={(event) =>
                            setForm({
                              ...form,
                              secretValue: event.target.value,
                            })
                          }
                          placeholder={
                            form.id
                              ? `${t("common.replaceSecret")} · ${form.id}`
                              : "保存后加密，不会回显明文"
                          }
                          required={!form.id && spec.secretRequired}
                        />
                      </div>
                      {form.id && <div className="cred-field">指纹已保存</div>}
                    </div>
                  </div>

                  <div className="divider" />

                  <div className="form-grid form-grid-2">
                    {spec.fields.map((field) => (
                      <FieldInput
                        key={field.key}
                        field={field}
                        value={form.settings[field.key]}
                        onChange={(value) => updateSetting(field.key, value)}
                        onHelp={setActiveHelp}
                      />
                    ))}
                  </div>
                </div>
              </div>

              <div className="panel-section">
                <div className="panel-section-head">
                  <div className="panel-section-title">消息模板</div>
                  <button
                    className="btn btn-ghost btn-xs"
                    type="button"
                    onClick={resetTemplate}
                  >
                    恢复默认
                  </button>
                </div>
                <div className="tpl-tabs">
                  {availableTemplates.map((tpl) => (
                    <button
                      key={tpl}
                      type="button"
                      className={`tpl-tab${templateType === tpl ? " active" : ""}`}
                      onClick={() => setTemplateType(tpl)}
                    >
                      {templateLabel(tpl)}
                    </button>
                  ))}
                </div>
                <div
                  style={{
                    padding: 14,
                    display: "flex",
                    flexDirection: "column",
                    gap: 12,
                  }}
                >
                  <textarea
                    className="input textarea"
                    style={{
                      fontFamily: "var(--mono)",
                      fontSize: 12,
                      height: 220,
                      resize: "vertical",
                    }}
                    value={currentTemplate}
                    onChange={(event) => updateTemplate(event.target.value)}
                    spellCheck={false}
                  />
                  <div>
                    <div
                      style={{
                        fontSize: 10,
                        fontWeight: 600,
                        textTransform: "uppercase",
                        letterSpacing: ".07em",
                        color: "var(--text-3)",
                        marginBottom: 6,
                      }}
                    >
                      点击复制变量
                    </div>
                    <div className="var-chips">
                      {templateVars.map((item) => (
                        <button
                          key={item}
                          type="button"
                          className="var-chip"
                          onClick={() =>
                            navigator.clipboard?.writeText(`{{${item}}}`)
                          }
                        >
                          {`{{${item}}}`}
                        </button>
                      ))}
                    </div>
                  </div>
                </div>
                <div className="action-bar">
                  <Button size="sm" variant="ghost" onClick={resetTemplate}>
                    <RotateCcw size={13} />
                    恢复默认
                  </Button>
                  <Button
                    size="sm"
                    loading={draftTestMut.isPending}
                    onClick={() => draftTestMut.mutate()}
                  >
                    <Send size={13} />
                    测试当前配置发送
                  </Button>
                  {form.id && (
                    <Button
                      size="sm"
                      variant="ghost"
                      loading={
                        testMut.isPending && testMut.variables === form.id
                      }
                      onClick={() => form.id && testMut.mutate(form.id)}
                    >
                      <Send size={13} />
                      测试已保存配置
                    </Button>
                  )}
                  <Button
                    size="sm"
                    variant="primary"
                    loading={saveMut.isPending}
                    onClick={() => saveMut.mutate()}
                  >
                    <Save size={13} />
                    保存配置
                  </Button>
                </div>
                {lastTest && (
                  <div className="card-foot text-xs text-muted">{lastTest}</div>
                )}
              </div>
            </>
          )}
        </section>

        <aside className="notif-panel-r">
          <div className="preview-header">
            <span className="preview-header-title">实时预览</span>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span className="preview-channel-badge">{form.name}</span>
              <button
                className="btn btn-ghost btn-sm btn-icon"
                type="button"
                onClick={() =>
                  navigator.clipboard?.writeText(
                    renderTemplate(currentTemplate),
                  )
                }
              >
                <Copy size={13} />
              </button>
            </div>
          </div>
          <div className="preview-tabs-row">
            {(["message", "vars"] as const).map((tab) => (
              <button
                key={tab}
                type="button"
                className={`ptab${previewTab === tab ? " active" : ""}`}
                onClick={() => setPreviewTab(tab)}
              >
                {tab === "message" ? "渠道消息" : "变量说明"}
              </button>
            ))}
          </div>
          {previewTab === "message" && (
            <div className="preview-body">
              <ChannelPreview type={activeType} template={currentTemplate} />
              <div
                style={{
                  fontSize: 10,
                  color: "var(--text-3)",
                  textAlign: "center",
                  marginTop: 8,
                }}
              >
                {spec.sendPath} · 预览使用示例资产数据
              </div>
            </div>
          )}
          {previewTab === "vars" && (
            <div className="preview-body" style={{ padding: 0 }}>
              <table className="var-tbl">
                <thead>
                  <tr>
                    <th>变量</th>
                    <th>说明 / 示例值</th>
                  </tr>
                </thead>
                <tbody>
                  {variableDescriptions.map(([key, label, example]) => (
                    <tr key={key}>
                      <td>{`{{${key}}}`}</td>
                      <td>
                        <div
                          style={{ fontWeight: 500, color: "var(--text-1)" }}
                        >
                          {label}
                        </div>
                        <div
                          style={{
                            fontSize: 10.5,
                            color: "var(--text-3)",
                            marginTop: 1,
                            fontFamily: "var(--mono)",
                          }}
                        >
                          示例: {example}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div className="card-foot">
            <div className="text-xs text-muted">{activeHelp}</div>
          </div>
        </aside>
      </div>
    </AppShell>
  );
}

function currentTemplateValue(form: ChannelForm, templateType: TemplateKey) {
  const key = templateSettingKey(templateType);
  const value = settingText(form.settings, key);
  return value || defaultTemplates[templateType];
}

function groupLabel(group: ChannelSpec["group"]) {
  if (group === "IM") return "即时通讯 (IM)";
  if (group === "HTTP") return "HTTP";
  if (group === "Email") return "邮件 (Email)";
  if (group === "SMS") return "短信 (SMS)";
  return "语音 (Voice)";
}

function InfoButton({
  text,
  onClick,
}: {
  text: string;
  onClick: (text: string) => void;
}) {
  return (
    <button
      className="info-btn"
      type="button"
      title={text}
      onClick={() => onClick(text)}
    >
      <Info size={10} />
    </button>
  );
}

function FieldInput({
  field,
  value,
  onChange,
  onHelp,
}: {
  field: FieldSpec;
  value: string | number | boolean | undefined;
  onChange: (value: string | number | boolean) => void;
  onHelp: (text: string) => void;
}) {
  const text =
    typeof value === "string" || typeof value === "number" ? String(value) : "";
  return (
    <div className="form-group">
      <label className="form-label">
        {field.label}
        {field.required && <span className="req">*</span>}
        <InfoButton text={field.help} onClick={onHelp} />
      </label>
      {field.kind === "textarea" ? (
        <textarea
          className="textarea input-mono"
          value={text}
          onChange={(event) => onChange(event.target.value)}
          placeholder={field.placeholder}
          required={field.required}
        />
      ) : field.kind === "checkbox" ? (
        <label className="toggle">
          <input
            type="checkbox"
            checked={value === true}
            onChange={(event) => onChange(event.target.checked)}
          />
          <span className="toggle-track" />
        </label>
      ) : (
        <input
          className={field.kind === "number" ? "input input-mono" : "input"}
          type={
            field.kind === "number"
              ? "number"
              : field.kind === "password"
                ? "password"
                : "text"
          }
          value={text}
          onChange={(event) =>
            onChange(
              field.kind === "number"
                ? Number(event.target.value)
                : event.target.value,
            )
          }
          placeholder={field.placeholder}
          required={field.required}
        />
      )}
      <div className="form-hint">{field.help}</div>
    </div>
  );
}

function ChannelPreview({
  type,
  template,
}: {
  type: ChannelType;
  template: string;
}) {
  const rendered = renderTemplate(template);
  if (type === "feishu") {
    return (
      <div className="fs-card">
        <div className="fs-hd" style={{ background: "#EE4040" }}>
          <div className="fs-hd-title">飞书卡片预览</div>
          <span className="fs-hd-badge">CRITICAL</span>
        </div>
        <div className="fs-body">
          <pre className="preview-rendered">{rendered}</pre>
        </div>
        <div className="fs-foot">
          <button className="fs-btn pri" type="button">
            查看详情
          </button>
          <button className="fs-btn" type="button">
            静默告警
          </button>
        </div>
      </div>
    );
  }
  if (type === "dingtalk" || type === "wecom") {
    return (
      <div
        className="dt-wrap"
        style={type === "wecom" ? { background: "#E8EEF3" } : undefined}
      >
        <div style={{ display: "flex", alignItems: "flex-start", gap: 8 }}>
          <div
            className="dt-avatar"
            style={type === "wecom" ? { background: "#07C160" } : undefined}
          >
            {type === "wecom" ? "W" : "Q"}
          </div>
          <div className="dt-bubble">
            <pre className="preview-rendered">{rendered}</pre>
          </div>
        </div>
      </div>
    );
  }
  if (type === "email_smtp" || type === "sendgrid_email") {
    return (
      <div className="email-sim">
        <div style={{ background: "#EF4444", height: 4 }} />
        <div className="email-sim-subject">{sampleValues.title}</div>
        <div className="email-sim-meta">
          发件人: API Monitor &lt;noreply@api-monitor.internal&gt; ·{" "}
          {sampleValues.openedAt}
        </div>
        <div
          className="email-sim-body"
          dangerouslySetInnerHTML={{ __html: rendered }}
        />
      </div>
    );
  }
  if (
    type === "twilio_sms" ||
    type === "aliyun_sms" ||
    type === "tencent_sms"
  ) {
    return (
      <div className="sms-wrap">
        <div className="sms-carrier">短信预览 · API Monitor Alerts</div>
        <div className="sms-bubble">{rendered}</div>
        <div className="sms-time">{sampleValues.openedAt}</div>
      </div>
    );
  }
  if (type === "phone") {
    return (
      <div
        style={{
          background: "var(--surface-2)",
          border: "1px solid var(--border)",
          borderRadius: "var(--r-lg)",
          padding: 16,
        }}
      >
        <div style={{ fontSize: 13, fontWeight: 500 }}>TTS 语音文案</div>
        <div style={{ fontSize: 11, color: "var(--text-3)", marginTop: 2 }}>
          预计播报时长 ~18 秒
        </div>
        <div className="voice-wave">
          {[10, 18, 12, 24, 16, 22, 14, 26, 18, 12, 20, 10, 24, 16].map(
            (h, i) => (
              <div
                key={i}
                className="vbar"
                style={{ width: 3, height: h, animationDelay: `${i * 0.04}s` }}
              />
            ),
          )}
        </div>
        <div
          style={{
            background: "var(--surface-3)",
            borderRadius: 6,
            padding: "12px 14px",
            fontSize: 12.5,
            lineHeight: 1.8,
          }}
        >
          {rendered}
        </div>
      </div>
    );
  }
  return <div className="wh-wrap">{rendered}</div>;
}
