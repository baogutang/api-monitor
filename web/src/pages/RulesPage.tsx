import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { instancesApi, rulesApi, targetsApi } from "@/api/services";
import { AppShell } from "@/components/layout/AppShell";
import { Button } from "@/components/ui/Button";
import { Card, CardBody } from "@/components/ui/Card";
import { ErrorState, LoadingSkeleton } from "@/components/ui/State";
import type {
  AlertRule,
  Instance,
  MonitorTarget,
  ProviderKind,
} from "@/lib/types";
import { usePreferences } from "@/contexts/PreferencesContext";
import { providerLabel } from "@/lib/format";

const CONDITIONS = [
  "balance_below",
  "remaining_quota_below",
  "remaining_percent_below",
  "days_until_expiry_below",
  "scan_failures_gte",
  "health_not_healthy",
  "monthly_cost_above",
  "cost_spike_percent_above",
] as const;

const SCOPES = [
  {
    value: "global",
    zh: "全局",
    en: "Global",
    helpZh: "所有上游和资产都会套用这条规则。",
    helpEn: "Apply this rule to every upstream and asset.",
  },
  {
    value: "provider",
    zh: "按上游类型",
    en: "By provider",
    helpZh: "只匹配某一类上游，例如 openai_account 或 sub2api_user。",
    helpEn: "Match one provider kind, such as openai_account or sub2api_user.",
  },
  {
    value: "group",
    zh: "按分组",
    en: "By group",
    helpZh: "只匹配账号配置里填写的分组名称。",
    helpEn: "Match the group name configured on instances.",
  },
  {
    value: "instance",
    zh: "按上游实例",
    en: "By instance",
    helpZh: "只匹配某个上游实例 ID。",
    helpEn: "Match one upstream instance ID.",
  },
  {
    value: "asset",
    zh: "按监控资产",
    en: "By asset",
    helpZh: "只匹配某个监控资产 ID，比如单个 API Key 或订阅。",
    helpEn:
      "Match one monitored asset ID, such as a single API key or subscription.",
  },
];

const PROVIDER_KINDS: ProviderKind[] = [
  "openai_account",
  "gemini_account",
  "anthropic_account",
  "newapi_user",
  "sub2api_user",
  "newapi_token",
  "sub2api_token",
  "openai_key",
  "anthropic_key",
  "openai_admin",
  "manual_subscription",
  "generic_http",
];

export function RulesPage() {
  const { t } = useTranslation();
  const { resolvedLocale } = usePreferences();
  const isEN = resolvedLocale === "en";
  const qc = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState<Partial<AlertRule>>({
    name: "",
    scopeType: "global",
    severity: "warning",
    conditionType: "balance_below",
    thresholdValue: 10,
    sustainCount: 2,
    cooldownSeconds: 3600,
    notificationChannelIds: [],
    enabled: true,
  });

  const list = useQuery({ queryKey: ["rules"], queryFn: rulesApi.list });
  const instances = useQuery({
    queryKey: ["instances"],
    queryFn: instancesApi.list,
  });
  const targets = useQuery({
    queryKey: ["targets", "rule-options"],
    queryFn: () => targetsApi.list({ limit: 500 }),
  });
  const scopeOptions = useMemo(
    () =>
      buildScopeOptions(
        form.scopeType ?? "global",
        instances.data ?? [],
        targets.data?.items ?? [],
        t,
      ),
    [form.scopeType, instances.data, targets.data?.items, t],
  );

  const createMut = useMutation({
    mutationFn: () => rulesApi.create(form),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["rules"] });
      setShowForm(false);
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => rulesApi.delete(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["rules"] }),
  });

  const preview = buildPreview(form, t);
  const scope =
    SCOPES.find((s) => s.value === (form.scopeType ?? "global")) ?? SCOPES[0];

  return (
    <AppShell
      title={t("rules.title")}
      description={t("rules.desc")}
      onRefresh={() => void list.refetch()}
      refreshing={list.isFetching}
      actions={
        <Button size="sm" variant="primary" onClick={() => setShowForm(true)}>
          <Plus size={14} />
          {t("rules.create")}
        </Button>
      }
    >
      {showForm && (
        <Card className="mb-4">
          <CardBody>
            <div className="grid grid-cols-2 gap-3">
              <div className="field col-span-2">
                <label>{t("assets.name")}</label>
                <input
                  className="input"
                  value={form.name ?? ""}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                />
              </div>
              <div className="field">
                <label>{t("rules.scope")}</label>
                <select
                  className="select"
                  value={form.scopeType ?? "global"}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      scopeType: e.target.value,
                      scopeValue: "",
                    })
                  }
                >
                  {SCOPES.map((item) => (
                    <option key={item.value} value={item.value}>
                      {isEN ? item.en : item.zh}
                    </option>
                  ))}
                </select>
                <span className="field-help">
                  {isEN ? scope.helpEn : scope.helpZh}
                </span>
              </div>
              {(form.scopeType ?? "global") !== "global" && (
                <div className="field">
                  <label>{isEN ? "Scope value" : "作用域值"}</label>
                  {scopeOptions.length > 0 ? (
                    <select
                      className="select"
                      value={form.scopeValue ?? ""}
                      onChange={(e) =>
                        setForm({ ...form, scopeValue: e.target.value })
                      }
                    >
                      <option value="">
                        {isEN ? "Select a saved value" : "选择已保存的值"}
                      </option>
                      {scopeOptions.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                  ) : (
                    <input
                      className="input"
                      value={form.scopeValue ?? ""}
                      placeholder={
                        isEN
                          ? "No saved value yet; enter exact value"
                          : "暂无已保存的值，可手动填写精确值"
                      }
                      onChange={(e) =>
                        setForm({ ...form, scopeValue: e.target.value })
                      }
                    />
                  )}
                  <span className="field-help">
                    {scopeValueHelp(form.scopeType ?? "global", isEN)}
                  </span>
                </div>
              )}
              <div className="field">
                <label>{t("alerts.severity")}</label>
                <select
                  className="select"
                  value={form.severity ?? "warning"}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      severity: e.target.value as AlertRule["severity"],
                    })
                  }
                >
                  <option value="warning">{t("severity.warning")}</option>
                  <option value="critical">{t("severity.critical")}</option>
                  <option value="phone">{t("severity.phone")}</option>
                </select>
              </div>
              <div className="field">
                <label>{t("rules.condition")}</label>
                <select
                  className="select"
                  value={form.conditionType ?? "balance_below"}
                  onChange={(e) =>
                    setForm({ ...form, conditionType: e.target.value })
                  }
                >
                  {CONDITIONS.map((c) => (
                    <option key={c} value={c}>
                      {t(`condition.${c}`)}
                    </option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label>{t("rules.threshold")}</label>
                <input
                  className="input"
                  type="number"
                  value={form.thresholdValue ?? 0}
                  onChange={(e) =>
                    setForm({ ...form, thresholdValue: Number(e.target.value) })
                  }
                />
              </div>
              <div className="field">
                <label>{t("rules.sustain")}</label>
                <input
                  className="input"
                  type="number"
                  value={form.sustainCount ?? 1}
                  onChange={(e) =>
                    setForm({ ...form, sustainCount: Number(e.target.value) })
                  }
                />
              </div>
              <div className="field">
                <label>{t("rules.cooldown")}</label>
                <input
                  className="input"
                  type="number"
                  value={Math.round((form.cooldownSeconds ?? 0) / 60)}
                  onChange={(e) =>
                    setForm({
                      ...form,
                      cooldownSeconds: Number(e.target.value) * 60,
                    })
                  }
                />
              </div>
            </div>
            <p className="text-sm text-text-3 mt-3 p-3 bg-bg-elevated rounded-md">
              {t("rules.preview")}: {preview}
            </p>
            <div className="flex gap-2 mt-3">
              <Button
                variant="primary"
                loading={createMut.isPending}
                onClick={() => createMut.mutate()}
              >
                {t("common.save")}
              </Button>
              <Button variant="ghost" onClick={() => setShowForm(false)}>
                {t("common.cancel")}
              </Button>
            </div>
          </CardBody>
        </Card>
      )}

      {list.isLoading ? (
        <LoadingSkeleton />
      ) : list.isError ? (
        <ErrorState onRetry={() => void list.refetch()} />
      ) : (list.data ?? []).length === 0 ? (
        <div className="empty-state">
          <h3>{isEN ? "No alert rules yet" : "还没有告警规则"}</h3>
          <p>
            {isEN
              ? "Create rules for balance, quota windows, expiry, and scan health."
              : "创建余额、窗口额度、到期时间和扫描健康相关的告警策略。"}
          </p>
          <Button variant="primary" onClick={() => setShowForm(true)}>
            <Plus size={14} />
            {t("rules.create")}
          </Button>
        </div>
      ) : (
        <div className="data-table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>{t("assets.name")}</th>
                <th>{t("rules.condition")}</th>
                <th>{t("alerts.severity")}</th>
                <th>{t("common.status")}</th>
                <th>{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {(list.data ?? []).map((rule) => (
                <tr key={rule.id}>
                  <td className="font-medium">{rule.name}</td>
                  <td className="text-sm">
                    {t(`condition.${rule.conditionType}`)} {rule.thresholdValue}
                  </td>
                  <td>{t(`severity.${rule.severity}`)}</td>
                  <td>
                    {rule.enabled ? t("common.enabled") : t("common.disabled")}
                  </td>
                  <td>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => {
                        if (confirm(t("common.confirmDelete")))
                          deleteMut.mutate(rule.id);
                      }}
                    >
                      {t("common.delete")}
                    </Button>
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

function buildPreview(
  form: Partial<AlertRule>,
  t: (k: string) => string,
): string {
  const cond = t(`condition.${form.conditionType ?? "balance_below"}`);
  const sev = t(`severity.${form.severity ?? "warning"}`);
  return `${cond} ${form.thresholdValue ?? 0} × ${form.sustainCount ?? 1} → ${sev}`;
}

function buildScopeOptions(
  scopeType: string,
  instances: Instance[],
  targets: MonitorTarget[],
  t: (key: string) => string,
) {
  if (scopeType === "provider") {
    return PROVIDER_KINDS.map((kind) => ({
      value: kind,
      label: providerLabel(kind, t),
    }));
  }
  if (scopeType === "group") {
    const groups = new Set<string>();
    instances.forEach((item) => {
      if (item.groupName?.trim()) groups.add(item.groupName.trim());
    });
    targets.forEach((item) => {
      if (item.groupName?.trim()) groups.add(item.groupName.trim());
    });
    return [...groups].sort().map((group) => ({ value: group, label: group }));
  }
  if (scopeType === "instance") {
    return instances.map((item) => ({
      value: item.id,
      label: item.groupName ? `${item.name} · ${item.groupName}` : item.name,
    }));
  }
  if (scopeType === "asset") {
    return targets.map((item) => ({
      value: item.id,
      label: item.groupName ? `${item.name} · ${item.groupName}` : item.name,
    }));
  }
  return [];
}

function scopeValueHelp(scopeType: string, isEN: boolean) {
  if (scopeType === "group")
    return isEN
      ? "Options come from saved upstream instances and monitored assets."
      : "选项来自已保存的上游实例和监控资产分组。";
  if (scopeType === "provider")
    return isEN
      ? "Provider labels are shown here; the saved value remains the stable provider key."
      : "这里显示中文名称，保存时仍使用稳定的上游类型标识。";
  if (scopeType === "instance")
    return isEN
      ? "Match one saved upstream instance."
      : "匹配一个已保存的上游实例。";
  if (scopeType === "asset")
    return isEN ? "Match one monitored asset." : "匹配一个已同步的监控资产。";
  return "";
}
