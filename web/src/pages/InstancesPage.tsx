import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  CheckCircle2,
  Copy,
  ExternalLink,
  Info,
  KeyRound,
  Pencil,
  Plus,
  TestTube,
  Trash2,
  Wand2,
  X,
} from "lucide-react";
import {
  accountOAuthApi,
  instancesApi,
  type AccountOAuthAuthorizeResponse,
} from "@/api/services";
import { AppShell } from "@/components/layout/AppShell";
import { Button } from "@/components/ui/Button";
import { ErrorState, LoadingSkeleton } from "@/components/ui/State";
import { providerLabel } from "@/lib/format";
import type {
  Instance,
  ProviderKind,
  UpsertInstanceRequest,
} from "@/lib/types";
import { Card, CardBody } from "@/components/ui/Card";
import { BrandIcon } from "@/components/BrandIcon";
import { usePreferences } from "@/contexts/PreferencesContext";

type ProviderCategory = "official" | "relay" | "api_key" | "manual";
type ProviderDef = {
  kind: ProviderKind;
  category: ProviderCategory;
  brand: Parameters<typeof BrandIcon>[0]["kind"];
  zh: string;
  en: string;
  descZh: string;
  descEn: string;
  baseUrl?: string;
  credentialType: NonNullable<UpsertInstanceRequest["credential"]>["type"];
};

type InstanceForm = UpsertInstanceRequest & {
  settings: Record<string, unknown>;
  credential: NonNullable<UpsertInstanceRequest["credential"]>;
};

type TestFeedback = {
  type: "ok" | "err" | "info";
  text: string;
};

const PROVIDERS: ProviderDef[] = [
  {
    kind: "openai_account",
    category: "official",
    brand: "openai",
    zh: "OpenAI 官方账号",
    en: "OpenAI account",
    descZh: "通过官方授权链接接入，扫描后自动读取套餐档位与 5H、7D 可用窗口。",
    descEn:
      "Connect through the official authorization link; scans read plan tiers and 5H/7D windows automatically.",
    baseUrl: "https://api.openai.com",
    credentialType: "json",
  },
  {
    kind: "gemini_account",
    category: "official",
    brand: "gemini",
    zh: "Gemini 官方账号",
    en: "Gemini account",
    descZh: "通过 Google 授权接入，扫描后自动读取账号健康与可用窗口。",
    descEn:
      "Connect through Google authorization; scans read account health and available windows.",
    baseUrl: "https://generativelanguage.googleapis.com",
    credentialType: "json",
  },
  {
    kind: "anthropic_account",
    category: "official",
    brand: "anthropic",
    zh: "Claude 官方账号",
    en: "Claude account",
    descZh: "通过 Claude 授权接入，扫描后自动读取 5H、7D 发送窗口。",
    descEn:
      "Connect through Claude authorization; scans read 5H/7D message windows automatically.",
    baseUrl: "https://api.anthropic.com",
    credentialType: "json",
  },
  {
    kind: "newapi_user",
    category: "relay",
    brand: "newapi",
    zh: "New API 用户登录",
    en: "New API user login",
    descZh: "普通用户账号密码登录，调用 /api/user/login 与 /api/user/self。",
    descEn:
      "Login as a normal user through /api/user/login and /api/user/self.",
    credentialType: "basic",
  },
  {
    kind: "sub2api_user",
    category: "relay",
    brand: "sub2api",
    zh: "Sub2Api 用户登录",
    en: "Sub2Api user login",
    descZh: "普通用户邮箱密码登录，读取 API Keys、订阅和 platform quotas。",
    descEn:
      "Login as a normal user and read API keys, subscriptions, and platform quotas.",
    credentialType: "basic",
  },
  {
    kind: "newapi_token",
    category: "api_key",
    brand: "newapi",
    zh: "New API Token",
    en: "New API token",
    descZh: "只监控单个 New API token 的用量和可用性。",
    descEn: "Monitor one New API token usage and health.",
    credentialType: "bearer",
  },
  {
    kind: "sub2api_token",
    category: "api_key",
    brand: "sub2api",
    zh: "Sub2Api Token",
    en: "Sub2Api token",
    descZh: "通过 OpenAI 兼容 /v1/models 探测 token 可用性。",
    descEn:
      "Probe token health through the OpenAI-compatible /v1/models endpoint.",
    credentialType: "bearer",
  },
  {
    kind: "openai_admin",
    category: "api_key",
    brand: "openai",
    zh: "OpenAI Admin Key",
    en: "OpenAI admin key",
    descZh: "使用 OpenAI Admin costs API 读取组织成本，需要官方管理权限。",
    descEn:
      "Uses the OpenAI Admin costs API; requires official organization permissions.",
    baseUrl: "https://api.openai.com",
    credentialType: "api_key",
  },
  {
    kind: "openai_key",
    category: "api_key",
    brand: "openai",
    zh: "OpenAI API Key",
    en: "OpenAI API key",
    descZh: "通过 /v1/models 检查官方 API Key 健康状态。",
    descEn: "Checks official API key health through /v1/models.",
    baseUrl: "https://api.openai.com",
    credentialType: "api_key",
  },
  {
    kind: "anthropic_key",
    category: "api_key",
    brand: "anthropic",
    zh: "Anthropic API Key",
    en: "Anthropic API key",
    descZh: "通过 Anthropic /v1/models 检查 API Key 健康状态。",
    descEn: "Checks API key health through Anthropic /v1/models.",
    baseUrl: "https://api.anthropic.com",
    credentialType: "api_key",
  },
  {
    kind: "manual_subscription",
    category: "manual",
    brand: "manual_subscription",
    zh: "手动订阅",
    en: "Manual subscription",
    descZh: "用于官方暂不开放接口的套餐余额和有效期录入。",
    descEn: "Manual plan balance and expiry for providers without public APIs.",
    credentialType: "none",
  },
  {
    kind: "generic_http",
    category: "manual",
    brand: "generic_http",
    zh: "通用 HTTP",
    en: "Generic HTTP",
    descZh: "接入自定义 JSON 余额接口。",
    descEn: "Connect a custom JSON balance endpoint.",
    credentialType: "json",
  },
];

const CATEGORY_COPY = {
  official: ["官方账号", "Official accounts"],
  relay: ["中转站用户", "Relay users"],
  api_key: ["API Key / Token", "API keys / tokens"],
  manual: ["手动与通用", "Manual and generic"],
} satisfies Record<ProviderCategory, [string, string]>;

export function InstancesPage() {
  const { t } = useTranslation();
  const { resolvedLocale } = usePreferences();
  const qc = useQueryClient();
  const isEN = resolvedLocale === "en";
  const copy = useMemo(() => pageCopy(isEN), [isEN]);
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [credentialDirty, setCredentialDirty] = useState(false);
  const [form, setForm] = useState<InstanceForm>(() =>
    createDefaultForm("openai_account"),
  );
  const [draftTestFeedback, setDraftTestFeedback] =
    useState<TestFeedback | null>(null);
  const [instanceFeedback, setInstanceFeedback] = useState<
    Record<string, TestFeedback>
  >({});

  const list = useQuery({
    queryKey: ["instances"],
    queryFn: instancesApi.list,
  });

  const selectedProvider =
    PROVIDERS.find((p) => p.kind === form.providerKind) ?? PROVIDERS[0];

  const saveMut = useMutation({
    mutationFn: () => {
      const body = instancePayload(form, editingId !== null, credentialDirty);
      return editingId
        ? instancesApi.patch(editingId, body)
        : instancesApi.create(body);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["instances"] });
      resetForm();
    },
  });

  const draftTestMut = useMutation({
    mutationFn: () => {
      if (editingId && !credentialDirty) {
        return instancesApi.test(editingId);
      }
      return instancesApi.testDraft({
        ...instancePayload(form, editingId !== null, credentialDirty),
        name: form.name || (isEN ? selectedProvider.en : selectedProvider.zh),
      } as UpsertInstanceRequest);
    },
    onSuccess: (data) => {
      setDraftTestFeedback({
        type: data.ok ? "ok" : "err",
        text: data.ok
          ? `${copy.testOk}: ${data.message || copy.reachable}`
          : `${copy.testFailed}: ${data.message || copy.unreachable}`,
      });
    },
    onError: (err) =>
      setDraftTestFeedback({
        type: "err",
        text: `${copy.testFailed}: ${errorMessage(err)}`,
      }),
  });

  const testMut = useMutation({
    mutationFn: (id: string) => instancesApi.test(id),
    onSuccess: (data, id) =>
      setInstanceFeedback((current) => ({
        ...current,
        [id]: {
          type: data.ok ? "ok" : "err",
          text: data.ok
            ? `${copy.testOk}: ${data.message || copy.reachable}`
            : `${copy.testFailed}: ${data.message || copy.unreachable}`,
        },
      })),
    onError: (err, id) =>
      setInstanceFeedback((current) => ({
        ...current,
        [id]: { type: "err", text: `${copy.testFailed}: ${errorMessage(err)}` },
      })),
  });
  const discoverMut = useMutation({
    mutationFn: (id: string) => instancesApi.discover(id),
    onSuccess: (data, id) => {
      void qc.invalidateQueries({ queryKey: ["targets"] });
      setInstanceFeedback((current) => ({
        ...current,
        [id]: {
          type: "ok",
          text: isEN
            ? `Synced ${data.created} monitored assets.`
            : `已同步 ${data.created} 个监控资产。`,
        },
      }));
    },
    onError: (err, id) =>
      setInstanceFeedback((current) => ({
        ...current,
        [id]: {
          type: "err",
          text: isEN
            ? `Sync failed: ${errorMessage(err)}`
            : `同步失败：${errorMessage(err)}`,
        },
      })),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => instancesApi.delete(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["instances"] }),
  });

  const resetForm = () => {
    setShowForm(false);
    setEditingId(null);
    setCredentialDirty(false);
    setForm(createDefaultForm("openai_account"));
    setDraftTestFeedback(null);
  };

  const startCreate = () => {
    setEditingId(null);
    setCredentialDirty(false);
    setForm(createDefaultForm("openai_account"));
    setDraftTestFeedback(null);
    setShowForm(true);
  };

  const startEdit = (instance: Instance) => {
    setEditingId(instance.id);
    setCredentialDirty(false);
    setForm(formFromInstance(instance));
    setDraftTestFeedback(null);
    setShowForm(true);
  };

  return (
    <AppShell
      title={t("instances.title")}
      description={t("instances.desc")}
      onRefresh={() => void list.refetch()}
      refreshing={list.isFetching}
      actions={
        <Button size="sm" variant="primary" onClick={startCreate}>
          <Plus size={14} />
          {t("instances.create")}
        </Button>
      }
    >
      {showForm && (
        <section className="provider-workspace">
          <div className="provider-picker">
            {(Object.keys(CATEGORY_COPY) as ProviderCategory[]).map(
              (category) => (
                <div key={category} className="provider-category">
                  <div className="provider-category-title">
                    {CATEGORY_COPY[category][isEN ? 1 : 0]}
                  </div>
                  <div className="provider-card-grid">
                    {PROVIDERS.filter((p) => p.category === category).map(
                      (provider) => (
                        <button
                          type="button"
                          key={provider.kind}
                          className={`provider-choice${provider.kind === form.providerKind ? " active" : ""}`}
                          onClick={() => {
                            const next = createDefaultForm(provider.kind);
                            setForm({
                              ...next,
                              name: form.name,
                              groupName: form.groupName,
                              scanIntervalSeconds: form.scanIntervalSeconds,
                              baseUrl: next.baseUrl || form.baseUrl,
                            });
                            setCredentialDirty(editingId !== null);
                            setDraftTestFeedback(null);
                          }}
                        >
                          <BrandIcon kind={provider.brand} />
                          <span>
                            <strong>{isEN ? provider.en : provider.zh}</strong>
                            <small>
                              {isEN ? provider.descEn : provider.descZh}
                            </small>
                          </span>
                          {provider.kind === form.providerKind && (
                            <CheckCircle2 size={16} />
                          )}
                        </button>
                      ),
                    )}
                  </div>
                </div>
              ),
            )}
          </div>

          <Card className="provider-config">
            <CardBody>
              <div className="provider-config-head">
                <div>
                  <div className="provider-kicker">{copy.currentType}</div>
                  <h2>
                    <BrandIcon kind={selectedProvider.brand} />
                    {isEN ? selectedProvider.en : selectedProvider.zh}
                  </h2>
                  <p>
                    {isEN ? selectedProvider.descEn : selectedProvider.descZh}
                  </p>
                </div>
                <Button size="icon" variant="ghost" onClick={resetForm}>
                  <X size={16} />
                </Button>
              </div>

              <div className="form-grid">
                <Field label={copy.instanceName}>
                  <input
                    className="input"
                    value={form.name}
                    placeholder={
                      isEN ? selectedProvider.en : selectedProvider.zh
                    }
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                  />
                </Field>
                <Field label={copy.groupName}>
                  <input
                    className="input"
                    value={form.groupName ?? ""}
                    placeholder={copy.groupPlaceholder}
                    onChange={(e) =>
                      setForm({ ...form, groupName: e.target.value })
                    }
                  />
                </Field>
                <Field label={copy.scanInterval}>
                  <input
                    className="input"
                    type="number"
                    min={30}
                    value={form.scanIntervalSeconds}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        scanIntervalSeconds: Number(e.target.value),
                      })
                    }
                  />
                </Field>
                <Field label={copy.baseUrl}>
                  <input
                    className="input"
                    value={form.baseUrl ?? ""}
                    placeholder={
                      selectedProvider.baseUrl ?? copy.baseUrlPlaceholder
                    }
                    onChange={(e) =>
                      setForm({ ...form, baseUrl: e.target.value })
                    }
                  />
                </Field>
              </div>

              <ProviderCredentialFields
                key={selectedProvider.kind}
                provider={selectedProvider}
                form={form}
                setForm={setForm}
                onCredentialDirty={() => setCredentialDirty(true)}
                isEN={isEN}
              />

              <div className="provider-actions">
                <Button
                  size="sm"
                  loading={draftTestMut.isPending}
                  onClick={() => draftTestMut.mutate()}
                >
                  <TestTube size={13} />
                  {copy.testCurrentConfig}
                </Button>
                <Button
                  variant="primary"
                  loading={saveMut.isPending}
                  onClick={() => saveMut.mutate()}
                >
                  {t("common.save")}
                </Button>
                <Button variant="ghost" onClick={resetForm}>
                  {t("common.cancel")}
                </Button>
              </div>
              {draftTestFeedback && (
                <div
                  className={`test-result test-result-${draftTestFeedback.type}`}
                >
                  {draftTestFeedback.text}
                </div>
              )}
            </CardBody>
          </Card>
        </section>
      )}

      {list.isLoading ? (
        <LoadingSkeleton />
      ) : list.isError ? (
        <ErrorState onRetry={() => void list.refetch()} />
      ) : (list.data ?? []).length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon">
            <KeyRound size={18} />
          </div>
          <h3>{copy.emptyTitle}</h3>
          <p>{copy.emptyDesc}</p>
          <Button variant="primary" onClick={startCreate}>
            <Plus size={14} />
            {t("instances.create")}
          </Button>
        </div>
      ) : (
        <div className="data-table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>{t("assets.name")}</th>
                <th>{t("assets.provider")}</th>
                <th>{t("instances.baseUrl")}</th>
                <th>{t("common.fingerprint")}</th>
                <th>{t("instances.scanInterval")}</th>
                <th>{t("common.status")}</th>
                <th>{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {(list.data ?? []).map((inst) => (
                <tr key={inst.id}>
                  <td>
                    <div className="provider-table-name">
                      <BrandIcon kind={inst.providerKind} />
                      <div>
                        <strong>{inst.name}</strong>
                        {inst.groupName && <small>{inst.groupName}</small>}
                      </div>
                    </div>
                  </td>
                  <td>{providerLabel(inst.providerKind, t)}</td>
                  <td className="mono text-xs">{inst.baseUrl ?? "—"}</td>
                  <td className="mono text-xs">
                    {inst.credentialFingerprint ?? (
                      <span className="text-text-4">
                        {t("common.configured")}
                      </span>
                    )}
                  </td>
                  <td className="mono">{inst.scanIntervalSeconds}s</td>
                  <td>
                    {inst.enabled ? t("common.enabled") : t("common.disabled")}
                  </td>
                  <td>
                    <div className="table-action-row">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => startEdit(inst)}
                      >
                        <Pencil size={12} />
                        {copy.edit}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        loading={
                          testMut.isPending && testMut.variables === inst.id
                        }
                        onClick={() => testMut.mutate(inst.id)}
                      >
                        <TestTube size={12} />
                        {t("common.testConnection")}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        title={copy.syncAssetsHelp}
                        loading={
                          discoverMut.isPending &&
                          discoverMut.variables === inst.id
                        }
                        onClick={() => discoverMut.mutate(inst.id)}
                      >
                        <Wand2 size={12} />
                        {t("common.discover")}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          if (confirm(t("common.confirmDelete")))
                            deleteMut.mutate(inst.id);
                        }}
                      >
                        <Trash2 size={12} />
                      </Button>
                    </div>
                    {instanceFeedback[inst.id] && (
                      <div
                        className={`test-result test-result-${instanceFeedback[inst.id].type}`}
                      >
                        {instanceFeedback[inst.id].text}
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </AppShell>
  );
}

function ProviderCredentialFields({
  provider,
  form,
  setForm,
  onCredentialDirty,
  isEN,
}: {
  provider: ProviderDef;
  form: InstanceForm;
  setForm: (form: InstanceForm) => void;
  onCredentialDirty: () => void;
  isEN: boolean;
}) {
  const copy = pageCopy(isEN);
  if (provider.category === "official") {
    return (
      <OfficialAccountFields
        provider={provider}
        form={form}
        setForm={setForm}
        onCredentialDirty={onCredentialDirty}
        isEN={isEN}
      />
    );
  }
  if (provider.kind === "newapi_user" || provider.kind === "sub2api_user") {
    return (
      <section className="credential-panel">
        <div className="panel-title">
          <KeyRound size={15} />
          {copy.userPassword}
        </div>
        <div className="hint-row">
          <Info size={14} />
          {provider.kind === "newapi_user" ? copy.newapiHint : copy.sub2apiHint}
        </div>
        <div className="form-grid">
          <Field
            label={
              provider.kind === "sub2api_user" ? copy.email : copy.username
            }
          >
            <input
              className="input"
              autoComplete="username"
              value={form.credential.username ?? ""}
              onChange={(e) => {
                onCredentialDirty();
                setForm({
                  ...form,
                  credential: {
                    ...form.credential,
                    username: e.target.value,
                    type: "basic",
                  },
                });
              }}
            />
          </Field>
          <Field label={copy.password}>
            <input
              className="input"
              type="password"
              autoComplete="current-password"
              value={form.credential.password ?? ""}
              onChange={(e) => {
                onCredentialDirty();
                setForm({
                  ...form,
                  credential: {
                    ...form.credential,
                    password: e.target.value,
                    type: "basic",
                  },
                });
              }}
            />
          </Field>
          {provider.kind === "sub2api_user" && (
            <Field label="Turnstile token">
              <input
                className="input"
                value={String(form.credential.json?.turnstile_token ?? "")}
                placeholder={copy.optional}
                onChange={(e) => {
                  onCredentialDirty();
                  setCredentialJSON(form, setForm, {
                    turnstile_token: e.target.value,
                  });
                }}
              />
            </Field>
          )}
        </div>
      </section>
    );
  }
  if (provider.credentialType === "none") {
    return (
      <section className="credential-panel">
        <div className="panel-title">
          <Info size={15} />
          {copy.manualOnly}
        </div>
        <p className="panel-copy">{copy.manualHint}</p>
      </section>
    );
  }
  return (
    <section className="credential-panel">
      <div className="panel-title">
        <KeyRound size={15} />
        {copy.secret}
      </div>
      <Field
        label={
          provider.credentialType === "bearer"
            ? "Bearer Token"
            : "API Key / JSON"
        }
      >
        <input
          className="input"
          type="password"
          placeholder={copy.replaceSecret}
          value={form.credential.value ?? ""}
          onChange={(e) => {
            onCredentialDirty();
            setForm({
              ...form,
              credential: {
                ...form.credential,
                type: provider.credentialType,
                value: e.target.value,
              },
            });
          }}
        />
      </Field>
    </section>
  );
}

function OfficialAccountFields({
  provider,
  form,
  setForm,
  onCredentialDirty,
  isEN,
}: {
  provider: ProviderDef;
  form: InstanceForm;
  setForm: (form: InstanceForm) => void;
  onCredentialDirty: () => void;
  isEN: boolean;
}) {
  const copy = pageCopy(isEN);
  const [session, setSession] = useState<AccountOAuthAuthorizeResponse | null>(
    null,
  );
  const [callbackValue, setCallbackValue] = useState("");
  const [accountMeta, setAccountMeta] = useState<Record<string, unknown>>({});
  const [oauthType, setOAuthType] = useState(
    provider.kind === "gemini_account" ? "code_assist" : "default",
  );
  const isOfficialProvider =
    provider.kind === "openai_account" ||
    provider.kind === "gemini_account" ||
    provider.kind === "anthropic_account";

  const authorizeMut = useMutation({
    mutationFn: () =>
      accountOAuthApi.authorize(
        provider.kind as
          | "openai_account"
          | "gemini_account"
          | "anthropic_account",
        {
          oauthType: provider.kind === "gemini_account" ? oauthType : undefined,
        },
      ),
    onSuccess: (data) => {
      setSession(data);
      setCallbackValue("");
    },
  });

  const exchangeMut = useMutation({
    mutationFn: () => {
      if (!session) throw new Error(copy.generateFirst);
      return accountOAuthApi.exchange(
        provider.kind as
          | "openai_account"
          | "gemini_account"
          | "anthropic_account",
        {
          sessionId: session.sessionId,
          callbackUrl: callbackValue,
        },
      );
    },
    onSuccess: (data) => {
      const nextName =
        form.name || accountDisplayName(provider, data.account, isEN);
      setAccountMeta(data.account);
      onCredentialDirty();
      setForm({
        ...form,
        name: nextName,
        credential: data.credential,
        settings: {
          ...form.settings,
          account: data.account,
        },
      });
    },
  });

  if (!isOfficialProvider) return null;

  return (
    <section className="credential-panel official-panel">
      <div className="panel-title">
        <KeyRound size={15} />
        {copy.officialAuth}
      </div>
      <div className="hint-row">
        <Info size={14} />
        {officialHint(provider.kind, isEN)}
      </div>

      {provider.kind === "gemini_account" && (
        <div className="auth-method-grid">
          <button
            type="button"
            className={`auth-method${oauthType === "code_assist" ? " active" : ""}`}
            onClick={() => setOAuthType("code_assist")}
          >
            Gemini Code Assist
          </button>
          <button
            type="button"
            className={`auth-method${oauthType === "ai_studio" ? " active" : ""}`}
            onClick={() => setOAuthType("ai_studio")}
          >
            Google AI Studio
          </button>
        </div>
      )}

      <div className="official-oauth-steps">
        <div className="oauth-step">
          <span>1</span>
          <div>
            <strong>{copy.stepGenerate}</strong>
            <p>{copy.stepGenerateDesc}</p>
            <Button
              size="sm"
              variant="primary"
              loading={authorizeMut.isPending}
              onClick={() => authorizeMut.mutate()}
            >
              <ExternalLink size={13} />
              {copy.generateAuth}
            </Button>
            {authorizeMut.isError && (
              <div className="form-hint text-crit">
                {(authorizeMut.error as Error).message}
              </div>
            )}
          </div>
        </div>

        <div className="oauth-step">
          <span>2</span>
          <div>
            <strong>{copy.stepOpen}</strong>
            <p>{session ? copy.stepOpenDescReady : copy.stepOpenDesc}</p>
            {session && (
              <div className="oauth-link-box">
                <input
                  className="input mono"
                  value={session.authUrl}
                  readOnly
                />
                <Button
                  size="sm"
                  onClick={() =>
                    window.open(
                      session.authUrl,
                      "_blank",
                      "noopener,noreferrer",
                    )
                  }
                >
                  <ExternalLink size={13} />
                  {copy.openAuth}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() =>
                    navigator.clipboard?.writeText(session.authUrl)
                  }
                >
                  <Copy size={13} />
                  {copy.copy}
                </Button>
              </div>
            )}
          </div>
        </div>

        <div className="oauth-step">
          <span>3</span>
          <div>
            <strong>{copy.stepCallback}</strong>
            <p>{copy.stepCallbackDesc}</p>
            <textarea
              className="textarea mono"
              value={callbackValue}
              placeholder={copy.callbackPlaceholder}
              onChange={(event) => setCallbackValue(event.target.value)}
            />
            <Button
              size="sm"
              variant="primary"
              disabled={!session || !callbackValue.trim()}
              loading={exchangeMut.isPending}
              onClick={() => exchangeMut.mutate()}
            >
              <CheckCircle2 size={13} />
              {copy.finishAuth}
            </Button>
            {exchangeMut.isError && (
              <div className="form-hint text-crit">
                {(exchangeMut.error as Error).message}
              </div>
            )}
            {Object.keys(accountMeta).length > 0 && (
              <div className="oauth-account-summary">
                {Object.entries(accountMeta).map(([key, value]) => (
                  <div key={key}>
                    <span>{key}</span>
                    <strong>{String(value)}</strong>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="oauth-step muted">
          <span>4</span>
          <div>
            <strong>{copy.stepScan}</strong>
            <p>{copy.stepScanDesc}</p>
          </div>
        </div>
      </div>
    </section>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="field">
      <label>{label}</label>
      {children}
    </div>
  );
}

function createDefaultForm(kind: ProviderKind): InstanceForm {
  const provider = PROVIDERS.find((p) => p.kind === kind) ?? PROVIDERS[0];
  return {
    name: "",
    providerKind: kind,
    baseUrl: provider.baseUrl ?? "",
    groupName: "",
    enabled: true,
    scanIntervalSeconds: 300,
    settings: {},
    credential: {
      type: provider.credentialType,
      value: "",
      username: "",
      password: "",
      json: defaultCredentialJSON(kind),
    },
  };
}

function formFromInstance(instance: Instance): InstanceForm {
  const next = createDefaultForm(instance.providerKind);
  return {
    ...next,
    name: instance.name,
    baseUrl: instance.baseUrl ?? next.baseUrl,
    groupName: instance.groupName ?? "",
    enabled: instance.enabled,
    scanIntervalSeconds: instance.scanIntervalSeconds,
    settings: instance.settings ?? {},
  };
}

function instancePayload(
  form: InstanceForm,
  isEditing: boolean,
  credentialDirty: boolean,
): UpsertInstanceRequest {
  const body: UpsertInstanceRequest = {
    name: form.name,
    providerKind: form.providerKind,
    baseUrl: form.baseUrl,
    groupName: form.groupName,
    enabled: form.enabled,
    scanIntervalSeconds: form.scanIntervalSeconds,
    settings: form.settings,
  };
  if (!isEditing || credentialDirty) {
    body.credential = form.credential;
  }
  return body;
}

function defaultCredentialJSON(kind: ProviderKind) {
  if (kind === "openai_account") return { auth_method: "oauth" };
  if (kind === "gemini_account")
    return { auth_method: "oauth", oauth_type: "code_assist" };
  if (kind === "anthropic_account") return { auth_method: "oauth" };
  return {};
}

function setCredentialJSON(
  form: InstanceForm,
  setForm: (form: InstanceForm) => void,
  patch: Record<string, unknown>,
) {
  setForm({
    ...form,
    credential: {
      ...form.credential,
      type: form.credential.type,
      json: { ...(form.credential.json ?? {}), ...patch },
    },
  });
}

function pageCopy(isEN: boolean) {
  return isEN
    ? {
        currentType: "Selected provider",
        instanceName: "Instance name",
        groupName: "Group",
        groupPlaceholder: "Optional grouping name",
        scanInterval: "Scan interval (seconds)",
        baseUrl: "Base URL",
        baseUrlPlaceholder: "https://your-upstream.example.com",
        userPassword: "User login",
        username: "Username",
        email: "Email",
        password: "Password",
        optional: "Optional",
        newapiHint:
          "Uses normal user login. No upstream admin permission is required.",
        sub2apiHint:
          "Uses /api/v1/auth/login, then reads user profile, API keys, subscriptions, and platform quotas.",
        manualOnly: "Manual data source",
        manualHint:
          "Use this for packages that do not expose a public balance API.",
        secret: "Secret",
        replaceSecret:
          "Leave empty only when editing and keeping the existing secret",
        officialAuth: "Official account authorization",
        callbackPlaceholder:
          "Paste the full callback URL or only the code value",
        generateFirst: "Generate an authorization link first.",
        stepGenerate: "Generate the official authorization link",
        stepGenerateDesc:
          "API Monitor creates a short-lived authorization session. No token needs to be copied manually.",
        generateAuth: "Generate link",
        stepOpen: "Open the link and finish login",
        stepOpenDesc:
          "After the link is generated, open it in a new tab and sign in to the official account.",
        stepOpenDescReady:
          "Open the link in a new tab. After the official page redirects or shows a code, paste it below.",
        openAuth: "Open",
        copy: "Copy",
        stepCallback: "Paste callback URL or code",
        stepCallbackDesc:
          "Paste the full redirected callback URL when possible. A bare code also works.",
        finishAuth: "Complete authorization",
        stepScan: "Save and scan",
        stepScanDesc:
          "Plan tier and quota windows such as 5H/7D are read during scan. They are not manually configured here.",
        emptyTitle: "No upstream instances yet",
        emptyDesc:
          "Add official accounts, relay users, or API tokens, then sync monitored assets.",
        edit: "Edit",
        testCurrentConfig: "Test current config",
        testOk: "Connection test passed",
        testFailed: "Connection test failed",
        reachable: "Upstream is reachable",
        unreachable: "Upstream is not reachable",
        syncAssetsHelp:
          "Log in to the upstream and sync users, API keys, subscriptions, and quota windows that can be monitored.",
      }
    : {
        currentType: "当前类型",
        instanceName: "实例名称",
        groupName: "分组",
        groupPlaceholder: "可选，比如 官方账号 / 中转站 A",
        scanInterval: "扫描间隔（秒）",
        baseUrl: "Base URL",
        baseUrlPlaceholder: "https://你的上游域名",
        userPassword: "用户账号登录",
        username: "用户名",
        email: "邮箱",
        password: "密码",
        optional: "可选",
        newapiHint: "使用普通用户登录接口，不需要上游 admin 权限。",
        sub2apiHint:
          "使用 /api/v1/auth/login 登录，再读取用户资料、API Keys、订阅和平台窗口额度。",
        manualOnly: "手动数据源",
        manualHint:
          "用于官方暂不开放余额接口的套餐，可通过发现/扫描保存手动窗口数据。",
        secret: "密钥",
        replaceSecret: "编辑时留空可保留原密钥",
        officialAuth: "官方账号授权",
        callbackPlaceholder: "可粘贴完整 callback URL，也可以只粘贴 code 参数",
        generateFirst: "请先生成授权链接。",
        stepGenerate: "生成官方授权链接",
        stepGenerateDesc:
          "API Monitor 会创建一个短时授权会话，不需要手动复制 token。",
        generateAuth: "生成授权链接",
        stepOpen: "打开链接并完成登录授权",
        stepOpenDesc: "生成链接后，在新标签页打开并登录对应官方账号。",
        stepOpenDescReady:
          "在新标签页打开授权链接。官方页面跳转或展示 code 后，把完整回调链接或 code 粘贴到下方。",
        openAuth: "打开",
        copy: "复制",
        stepCallback: "粘贴回调链接或 code",
        stepCallbackDesc:
          "推荐粘贴完整回调 URL；如果页面只显示 code，也可以只粘贴 code。",
        finishAuth: "完成授权",
        stepScan: "保存并扫描",
        stepScanDesc:
          "套餐档位、5H/7D 等时间窗口由扫描自动读取，这里不手动配置。",
        emptyTitle: "还没有上游实例",
        emptyDesc: "先添加官方账号、中转站用户或 API Token，再同步监控资产。",
        edit: "编辑",
        testCurrentConfig: "测试当前配置",
        testOk: "连接测试正常",
        testFailed: "连接测试失败",
        reachable: "上游可访问",
        unreachable: "上游不可访问",
        syncAssetsHelp: "登录上游并同步可监控的用户、API Key、订阅和窗口额度。",
      };
}

function officialHint(kind: ProviderKind, isEN: boolean) {
  if (isEN) {
    if (kind === "openai_account")
      return "OpenAI official account access uses an authorization link. API Monitor reads plan and 5H/7D quota windows during scan.";
    if (kind === "gemini_account")
      return "Gemini official account access uses Google authorization. Account health and quota windows are queried after saving.";
    return "Claude official account access uses Claude authorization. 5H and 7D usage windows are queried after saving.";
  }
  if (kind === "openai_account")
    return "OpenAI 官方账号通过授权链接接入；套餐档位、5H/7D 窗口会在保存后的扫描中读取。";
  if (kind === "gemini_account")
    return "Gemini 官方账号通过 Google 授权接入；账号健康和可用窗口会在保存后的扫描中查询。";
  return "Claude 官方账号通过 Claude 授权接入；5H、7D 使用窗口会在保存后的扫描中查询。";
}

function accountDisplayName(
  provider: ProviderDef,
  account: Record<string, unknown>,
  isEN: boolean,
) {
  const email = typeof account.email === "string" ? account.email : "";
  if (email) return `${isEN ? provider.en : provider.zh} · ${email}`;
  return isEN ? provider.en : provider.zh;
}

function errorMessage(err: unknown) {
  return err instanceof Error ? err.message : String(err || "unknown error");
}
