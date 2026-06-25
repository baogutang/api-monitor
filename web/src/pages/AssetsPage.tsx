import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronRight, Scan } from "lucide-react";
import { instancesApi, targetsApi } from "@/api/services";
import { AppShell } from "@/components/layout/AppShell";
import { Button } from "@/components/ui/Button";
import { StatusDot } from "@/components/ui/Badge";
import { ErrorState, LoadingSkeleton } from "@/components/ui/State";
import {
  formatDate,
  formatMoney,
  formatNumber,
  providerLabel,
  statusTone,
} from "@/lib/format";
import type {
  HealthStatus,
  Instance,
  InstanceUsageRange,
  InstanceUsageSummary,
  MonitorTarget,
  ProviderKind,
} from "@/lib/types";
import { AssetDetailDrawer } from "@/components/assets/AssetDetailDrawer";
import { usePreferences } from "@/contexts/PreferencesContext";
import { Fragment, useMemo, useState } from "react";
import { BrandIcon } from "@/components/BrandIcon";
import type { UsageWindow } from "@/lib/types";

const assetUsageRanges: InstanceUsageRange[] = ["today", "24h", "7d", "30d"];

export function AssetsPage() {
  const { t } = useTranslation();
  const { resolvedLocale } = usePreferences();
  const qc = useQueryClient();
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [provider, setProvider] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [usageRange, setUsageRange] =
    useState<InstanceUsageRange>("today");
  const [collapsedGroups, setCollapsedGroups] = useState<
    Record<string, boolean>
  >({});

  const query = useQuery({
    queryKey: ["targets", q, status, provider],
    queryFn: () =>
      targetsApi.list({
        q: q || undefined,
        status: status || undefined,
        providerKind: provider || undefined,
        limit: 100,
      }),
  });
  const instancesQuery = useQuery({
    queryKey: ["instances"],
    queryFn: instancesApi.list,
  });
  const usageQuery = useQuery({
    queryKey: ["instances", "usage", usageRange],
    queryFn: () => instancesApi.usage(usageRange),
  });

  const scanMut = useMutation({
    mutationFn: (id: string) => targetsApi.scan(id),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["targets"] }),
  });

  const items = query.data?.items ?? [];
  const instanceMap = useMemo(
    () =>
      new Map(
        (instancesQuery.data ?? []).map((instance) => [instance.id, instance]),
      ),
    [instancesQuery.data],
  );
  const groups = useMemo(
    () => groupTargets(items, instanceMap),
    [items, instanceMap],
  );
  const usageMap = useMemo(
    () =>
      new Map(
        (usageQuery.data ?? []).map((item) => [item.instanceId, item]),
      ),
    [usageQuery.data],
  );
  const isEN = resolvedLocale === "en";
  const toggleGroup = (id: string) => {
    setCollapsedGroups((current) => ({ ...current, [id]: !current[id] }));
  };
  const statusOptions: HealthStatus[] = [
    "healthy",
    "warning",
    "critical",
    "unknown",
  ];

  return (
    <AppShell
      title={t("assets.title")}
      description={t("assets.desc")}
      onRefresh={() => void query.refetch()}
      refreshing={query.isFetching}
    >
      <div className="filter-row">
        <input
          className="input"
          placeholder={t("common.search")}
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <select
          className="select"
          value={status}
          onChange={(e) => setStatus(e.target.value)}
        >
          <option value="">
            {t("common.status")}: {t("common.all")}
          </option>
          {statusOptions.map((s) => (
            <option key={s} value={s}>
              {t(`status.${s}`)}
            </option>
          ))}
        </select>
        <select
          className="select"
          value={provider}
          onChange={(e) => setProvider(e.target.value)}
        >
          <option value="">
            {t("assets.provider")}: {t("common.all")}
          </option>
          <option value="newapi_user">{t("provider.newapi_user")}</option>
          <option value="sub2api_user">{t("provider.sub2api_user")}</option>
          <option value="openai_account">{t("provider.openai_account")}</option>
          <option value="gemini_account">{t("provider.gemini_account")}</option>
          <option value="anthropic_account">
            {t("provider.anthropic_account")}
          </option>
          <option value="openai_key">{t("provider.openai_key")}</option>
          <option value="anthropic_key">{t("provider.anthropic_key")}</option>
        </select>
        <div className="asset-range-tabs" role="tablist" aria-label="实例用量范围">
          {assetUsageRanges.map((range) => (
            <button
              key={range}
              type="button"
              className={usageRange === range ? "active" : ""}
              onClick={() => setUsageRange(range)}
            >
              {usageRangeLabel(range, resolvedLocale)}
            </button>
          ))}
        </div>
      </div>

      {query.isLoading ? (
        <LoadingSkeleton />
      ) : query.isError ? (
        <ErrorState
          message={t("errors.network")}
          onRetry={() => void query.refetch()}
        />
      ) : items.length === 0 ? (
        <div className="empty-state">
          <h3>暂无监控资产</h3>
          <p>
            先在“上游实例”里保存官方账号、中转站用户或 API
            Token，然后同步监控资产。
          </p>
          <Link to="/instances">
            <Button variant="primary">
              <Scan size={14} />
              新建上游实例
            </Button>
          </Link>
        </div>
      ) : (
        <div className="data-table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th />
                <th>{t("assets.name")}</th>
                <th>{isEN ? "Upstream / Kind" : "上游 / 类型"}</th>
                <th>{isEN ? "Group / Rate" : "分组 / 倍率"}</th>
                <th>{isEN ? "Usage today" : "今日用量"}</th>
                <th>{t("assets.quota")}</th>
                <th>{t("assets.balance")}</th>
                <th>{t("assets.monthlyCost")}</th>
                <th>{t("assets.lastScan")}</th>
                <th>{t("assets.risk")}</th>
                <th>{t("common.actions")}</th>
              </tr>
            </thead>
            <tbody>
              {groups.map((group) => (
                <Fragment key={group.instanceId}>
                  <AssetGroupRow
                    group={group}
                    locale={resolvedLocale}
                    usageSummary={usageMap.get(group.instanceId)}
                    usageRange={usageRange}
                    collapsed={collapsedGroups[group.instanceId] ?? false}
                    onToggle={() => toggleGroup(group.instanceId)}
                    onViewUser={
                      group.userTarget
                        ? () => setSelectedId(group.userTarget!.id)
                        : undefined
                    }
                    t={t}
                  />
                  {!(collapsedGroups[group.instanceId] ?? false) &&
                    group.watchItems.length > 0 && (
                      <WatchPanelRow
                        items={group.watchItems}
                        locale={resolvedLocale}
                        onView={(id) => setSelectedId(id)}
                      />
                    )}
                  {!(collapsedGroups[group.instanceId] ?? false) &&
                    group.assetItems.map((row) => (
                      <AssetRow
                        key={row.id}
                        row={row}
                        instance={group.instance}
                        locale={resolvedLocale}
                        onView={() => setSelectedId(row.id)}
                        onScan={() => scanMut.mutate(row.id)}
                        scanning={
                          scanMut.isPending && scanMut.variables === row.id
                        }
                        t={t}
                      />
                    ))}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {selectedId && (
        <AssetDetailDrawer
          targetId={selectedId}
          onClose={() => setSelectedId(null)}
        />
      )}
    </AppShell>
  );
}

type AssetGroup = {
  instanceId: string;
  instance?: Instance;
  providerKind: ProviderKind;
  userTarget?: MonitorTarget;
  childItems: MonitorTarget[];
  assetItems: MonitorTarget[];
  watchItems: MonitorTarget[];
  items: MonitorTarget[];
};

function groupTargets(
  items: MonitorTarget[],
  instanceMap: Map<string, Instance>,
): AssetGroup[] {
  const order: string[] = [];
  const map = new Map<string, AssetGroup>();
  for (const item of items) {
    const key = item.instanceId || "unknown";
    if (!map.has(key)) {
      order.push(key);
      map.set(key, {
        instanceId: key,
        instance: instanceMap.get(key),
        providerKind:
          instanceMap.get(key)?.providerKind ?? item.providerKind ?? "generic_http",
        childItems: [],
        assetItems: [],
        watchItems: [],
        items: [],
      });
    }
    map.get(key)?.items.push(item);
  }
  return order
    .map((key) => {
      const group = map.get(key)!;
      const providerKind =
        group.instance?.providerKind ??
        group.items[0]?.providerKind ??
        "generic_http";
      const userTarget = isRelayProvider(providerKind)
        ? group.items.find((item) => item.kind === "user")
        : undefined;
      return {
        ...group,
        providerKind,
        userTarget,
        childItems: userTarget
          ? group.items.filter((item) => item.id !== userTarget.id)
          : group.items,
        watchItems: group.items.filter(
          (item) => item.id !== userTarget?.id && isWatchKind(item.kind),
        ),
        assetItems: group.items.filter(
          (item) => item.id !== userTarget?.id && !isWatchKind(item.kind),
        ),
      };
    })
    .filter(Boolean);
}

function AssetGroupRow({
  group,
  locale,
  usageSummary,
  usageRange,
  collapsed,
  onToggle,
  onViewUser,
  t,
}: {
  group: AssetGroup;
  locale: string;
  usageSummary?: InstanceUsageSummary;
  usageRange: InstanceUsageRange;
  collapsed: boolean;
  onToggle: () => void;
  onViewUser?: () => void;
  t: (k: string) => string;
}) {
  const instance = group.instance;
  const first = group.items[0];
  const title = instance?.name ?? group.items[0]?.groupName ?? group.instanceId;
  const provider = group.providerKind ?? first?.providerKind ?? "generic_http";
  const isEN = locale === "en";
  const summaryTarget = group.userTarget ?? first;
  const isRelay = isRelayProvider(provider);
  const apiKeys = group.assetItems.filter((item) => item.kind === "api_key").length;
  const watchAssets = group.watchItems.length;
  const childLabel = isRelay
    ? `${apiKeys} API Key${watchAssets ? ` · ${watchAssets} ${isEN ? "watchers" : "观察资产"}` : ""}`
    : `${group.childItems.length} ${isEN ? "assets" : "个资产"}`;
  const baseMetrics = group.userTarget
    ? relayUserMetrics(group.userTarget, locale)
    : compactMetrics(
        [
          {
            label: isEN ? "Assets" : "监控资产",
            value: String(group.items.length),
          },
          summaryTarget?.balance
            ? {
                label: isEN ? "Balance" : "余额",
                value: formatMoney(summaryTarget.balance),
              }
            : undefined,
          summaryTarget?.quota
            ? {
                label: isEN ? "Quota" : "额度",
                value: formatAssetQuota(summaryTarget, locale),
              }
            : undefined,
        ],
        4,
      );
  const metrics = compactMetrics(
    [
      ...baseMetrics,
      usageSummary?.cost
        ? {
            label:
              usageRangeLabel(usageRange, locale) +
              (isEN ? " usage" : "用量"),
            value: formatMoney(usageSummary.cost),
            tone: "warn" as const,
          }
        : undefined,
      usageSummary && usageSummary.requests > 0
        ? {
            label: isEN ? "Requests" : "请求数",
            value: formatNumber(usageSummary.requests),
          }
        : undefined,
    ],
    5,
  );
  return (
    <tr className="asset-group-row">
      <td colSpan={11}>
        <div className="asset-group-card">
          <div className="asset-group-main">
            <button
              type="button"
              className="asset-group-toggle"
              onClick={onToggle}
              aria-label={collapsed ? (isEN ? "Expand" : "展开") : isEN ? "Collapse" : "收起"}
            >
              {collapsed ? <ChevronRight size={15} /> : <ChevronDown size={15} />}
            </button>
            <BrandIcon kind={provider} />
            <div>
              <strong>{title}</strong>
              <span>
                {providerLabel(provider, t)}
                {instance?.groupName ? ` · ${instance.groupName}` : ""}
                {instance?.baseUrl ? ` · ${instance.baseUrl}` : ""}
              </span>
            </div>
          </div>
          <div className="asset-group-stats">
            {metrics.map((metric) => (
              <span
                key={`${metric.label}-${metric.value}`}
                className={`asset-group-stat ${metric.tone ?? ""}`}
              >
                <small>{metric.label}</small>
                <strong className="mono">{metric.value}</strong>
              </span>
            ))}
          </div>
          <div className="asset-group-meta">
            <span>{childLabel}</span>
            <span className="mono">{group.instanceId}</span>
            {onViewUser && (
              <button type="button" className="mini-link-button" onClick={onViewUser}>
                {isEN ? "User details" : "查看用户"}
              </button>
            )}
          </div>
        </div>
      </td>
    </tr>
  );
}

function AssetRow({
  row,
  instance,
  locale,
  onView,
  onScan,
  scanning,
  t,
}: {
  row: MonitorTarget;
  instance?: Instance;
  locale: string;
  onView: () => void;
  onScan: () => void;
  scanning: boolean;
  t: (k: string) => string;
}) {
  const raw = rawPayload(row);
  const groupRate = groupRateSummary(row, raw, locale);
  const todayUsage = todayUsageFact(row, raw, locale);
  return (
    <tr className="asset-child-row">
      <td>
        <span className="asset-relation-line" />
        <StatusDot tone={statusTone(row.status)} />
      </td>
      <td>
        <div className="asset-name-cell">
          <BrandIcon kind={row.providerKind} />
          <div>
            <button
              type="button"
              className="text-left text-brand-bright hover:underline"
              onClick={onView}
            >
              {row.name}
            </button>
            <WindowMini windows={usageWindows(row)} locale={locale} />
          </div>
        </div>
        <div className="asset-metadata-line">
          {assetKindHint(row, instance, locale)}
        </div>
        {row.keyFingerprint && (
          <div className="text-xs text-text-4 mono">{row.keyFingerprint}</div>
        )}
        <ApiKeyFacts row={row} locale={locale} />
      </td>
      <td>
        <div className="asset-value-cell">
          <span>{providerLabel(row.providerKind, t)}</span>
          <small>{t(`targetKind.${row.kind}`)}</small>
        </div>
      </td>
      <td>
        <div className="asset-value-cell">
          <span>{groupRate.primary}</span>
          <small>{groupRate.secondary}</small>
        </div>
      </td>
      <td>
        <div className="asset-value-cell">
          <span className="mono">{todayUsage.primary}</span>
          <small>{todayUsage.secondary}</small>
        </div>
      </td>
      <td>
        <div className="asset-value-cell">
          <span className="mono">{formatAssetQuota(row, locale)}</span>
          {planExpiryText(row, locale) && (
            <small>{planExpiryText(row, locale)}</small>
          )}
        </div>
      </td>
      <td>
        <div className="asset-value-cell">
          <span className="mono">{formatMoney(row.balance)}</span>
          {row.kind === "user" && (
            <small>
              {locale === "en" ? "Upstream user balance" : "上游用户余额"}
            </small>
          )}
        </div>
      </td>
      <td className="mono">{formatMoney(row.monthlyCost)}</td>
      <td className="text-xs">{formatDate(row.lastScanAt, locale)}</td>
      <td className="mono">{row.riskScore}</td>
      <td>
        <div className="flex gap-1">
          <Button size="sm" variant="ghost" onClick={onView}>
            {t("common.viewDetails")}
          </Button>
          <Button size="sm" loading={scanning} onClick={onScan}>
            <Scan size={12} />
          </Button>
        </div>
      </td>
    </tr>
  );
}

function WatchPanelRow({
  items,
  locale,
  onView,
}: {
  items: MonitorTarget[];
  locale: string;
  onView: (id: string) => void;
}) {
  const isEN = locale === "en";
  return (
    <tr className="asset-watch-row">
      <td />
      <td colSpan={10}>
        <div className="watch-source-grid">
          {sortWatchItems(items).map((row) => {
            const raw = row.raw ?? {};
            const watchItems = Array.isArray(raw.items) ? raw.items : [];
            const first = watchItems[0] as Record<string, unknown> | undefined;
            const label = watchLabel(row.kind, locale);
            const title =
              stringFromRaw(first ?? {}, ["title", "name"]) ??
              stringFromRaw(raw, ["summary"]) ??
              (isEN ? "Waiting for first scan" : "等待首次扫描");
            const source =
              stringFromRaw(raw, ["sourceUrl", "source"]) ??
              row.externalId ??
              "—";
            const fingerprint = stringFromRaw(raw, ["fingerprint"]);
            const checkedAt = stringFromRaw(raw, ["checkedAt"]);
            return (
              <article className="watch-source-card" key={row.id}>
                <div className="watch-source-top">
                  <div>
                    <span className="watch-source-kind">{label}</span>
                    <strong>{title}</strong>
                  </div>
                  <button
                    type="button"
                    className="mini-link-button"
                    onClick={() => onView(row.id)}
                  >
                    {isEN ? "Details" : "详情"}
                  </button>
                </div>
                {row.kind === "announcement_feed" && watchItems.length > 0 ? (
                  <div className="watch-history-list">
                    {watchItems.slice(0, 4).map((item, index) => {
                      const entry = item as Record<string, unknown>;
                      const entryTitle =
                        stringFromRaw(entry, ["title", "name"]) ??
                        (isEN ? "Untitled announcement" : "未命名公告");
                      const entrySummary = stringFromRaw(entry, [
                        "summary",
                        "content",
                        "description",
                        "text",
                      ]);
                      const entryTime = dateFromRaw(
                        entry,
                        ["date", "created_at", "createdAt", "updated_at", "updatedAt"],
                        locale,
                      );
                      return (
                        <div className="watch-history-item" key={`${entryTitle}-${index}`}>
                          <div>
                            <strong>{entryTitle}</strong>
                            {entrySummary && <p>{compactText(entrySummary, 92)}</p>}
                          </div>
                          {entryTime && <small>{entryTime}</small>}
                        </div>
                      );
                    })}
                  </div>
                ) : (
                  <p className="watch-source-summary">{compactText(String(title), 140)}</p>
                )}
                <div className="watch-source-meta">
                  <span>{isEN ? "Items" : "条目"} {watchItems.length || stringFromRaw(raw, ["count"]) || 0}</span>
                  {checkedAt && <span>{formatDate(checkedAt, locale)}</span>}
                  {fingerprint && <span className="mono">Hash {fingerprint.slice(0, 10)}</span>}
                </div>
                <div className="watch-source-url mono">{source}</div>
              </article>
            );
          })}
        </div>
      </td>
    </tr>
  );
}

function usageRangeLabel(range: InstanceUsageRange, locale: string) {
  const isEN = locale === "en";
  const labels: Record<InstanceUsageRange, string> = {
    today: isEN ? "Today" : "今日",
    "24h": isEN ? "24H" : "24小时",
    "7d": isEN ? "7D" : "7天",
    "30d": isEN ? "30D" : "30天",
  };
  return labels[range];
}

function sortWatchItems(items: MonitorTarget[]) {
  const order: Record<string, number> = {
    announcement_feed: 0,
    news_feed: 1,
    deprecation_feed: 2,
    group_catalog: 3,
    model_catalog: 4,
    pricing_catalog: 5,
  };
  return [...items].sort(
    (a, b) =>
      (order[a.kind] ?? 99) - (order[b.kind] ?? 99) ||
      a.name.localeCompare(b.name),
  );
}

function watchLabel(kind: string, locale: string) {
  const isEN = locale === "en";
  const labels: Record<string, [string, string]> = {
    announcement_feed: ["上游公告", "Announcements"],
    news_feed: ["官方新闻", "News"],
    deprecation_feed: ["模型下架", "Deprecations"],
    group_catalog: ["分组倍率", "Groups & rates"],
    model_catalog: ["模型目录", "Models"],
    pricing_catalog: ["价格目录", "Pricing"],
  };
  const label = labels[kind];
  return label ? (isEN ? label[1] : label[0]) : kind;
}

function compactText(value: string, max: number) {
  const text = value.replace(/\s+/g, " ").trim();
  if (text.length <= max) return text;
  return `${text.slice(0, Math.max(0, max - 1))}…`;
}

function assetKindHint(
  row: MonitorTarget,
  instance: Instance | undefined,
  locale: string,
) {
  const isEN = locale === "en";
  const source = instance?.name ?? row.groupName ?? row.instanceId;
  const kind =
    row.kind === "user"
      ? isEN
        ? "upstream user"
        : "上游用户"
      : row.kind === "api_key"
        ? isEN
          ? "API key"
          : "API Key 子资产"
        : isWatchKind(row.kind)
          ? isEN
            ? "watched source"
            : "观察源"
        : isEN
          ? row.kind
          : "订阅/套餐资产";
  return isEN ? `From ${source} · ${kind}` : `来自 ${source} · ${kind}`;
}

type AssetMetric = {
  label: string;
  value: string;
  tone?: "ok" | "warn" | "crit";
};

function isRelayProvider(provider: ProviderKind) {
  return provider === "newapi_user" || provider === "sub2api_user";
}

function isWatchKind(kind: string) {
  return [
    "announcement_feed",
    "news_feed",
    "deprecation_feed",
    "group_catalog",
    "model_catalog",
    "pricing_catalog",
  ].includes(kind);
}

function compactMetrics(
  metrics: Array<AssetMetric | undefined>,
  limit: number,
) {
  return metrics.filter((item): item is AssetMetric => Boolean(item)).slice(0, limit);
}

function relayUserMetrics(row: MonitorTarget, locale: string): AssetMetric[] {
  const raw = rawPayload(row);
  const isEN = locale === "en";
  const today = usageSummaryRange(raw, "today");
  const month = usageSummaryRange(raw, "30d");
  const balance =
    row.balance != null
      ? formatMoney(row.balance)
      : formatQuotaAsMoney(raw, [
          "balance",
          "available",
          "available_balance",
          "remaining_balance",
          "quota",
        ]);
  const spent =
    formatUsageRangeMoney(month, raw) ??
    (row.monthlyCost != null
      ? formatMoney(row.monthlyCost)
      : formatQuotaAsMoney(raw, [
          "used_quota",
          "usedQuota",
          "used",
          "used_amount",
          "usage_cost",
          "total_cost",
        ]));
  const requests = numberFromRaw(today, ["requests", "totalRequests"]) ?? numberFromRaw(month, ["requests", "totalRequests"]) ?? numberFromRaw(raw, [
    "request_count",
    "requestCount",
    "requests",
    "total_requests",
    "totalRequests",
    "api_requests",
  ]);
  const quota = formatAssetQuota(row, locale);
  return compactMetrics(
    [
      balance
        ? {
            label: isEN ? "Balance" : "当前余额",
            value: balance,
            tone: "ok",
          }
        : undefined,
      spent && spent !== "—"
        ? {
            label: isEN ? "Used" : "已消耗",
            value: spent,
            tone: "warn",
          }
        : undefined,
      quota !== "—"
        ? {
            label: isEN ? "Quota" : "额度",
            value: quota,
          }
        : undefined,
      typeof requests === "number"
        ? {
            label: isEN ? "Requests" : "请求数",
            value: formatNumber(requests),
          }
        : undefined,
    ],
    4,
  );
}

function ApiKeyFacts({
  row,
  locale,
}: {
  row: MonitorTarget;
  locale: string;
}) {
  if (row.kind !== "api_key") return null;
  const raw = rawPayload(row);
  const isEN = locale === "en";
  const facts: AssetMetric[] = [];
  const addFact = (label: string, value?: string) => {
    if (value && value !== "—") {
      facts.push({ label, value });
    }
  };

  addFact(isEN ? "Key" : "密钥", maskSecret(stringFromRaw(raw, ["key", "api_key", "apiKey", "token"])));
  addFact(isEN ? "Group" : "上游分组", groupNameFromRaw(row, raw));
  addFact(isEN ? "Rate" : "倍率", rateFact(raw, locale));
  addFact(isEN ? "Limit" : "额度限制", quotaFact(row, raw, locale));
  addFact(isEN ? "Used" : "已用", usageFact(row, raw, locale));
  addFact(isEN ? "Today" : "今日", dailyUsageFact(row, raw, locale));
  addFact(isEN ? "Requests" : "请求数", requestFact(raw, locale));
  addFact(isEN ? "Models" : "模型", listFromRaw(raw, ["models", "model", "model_limits", "modelLimits"]));
  addFact(isEN ? "IP" : "IP 限制", listFromRaw(raw, ["ip_whitelist", "ipWhitelist", "allow_ips", "allowIps", "ip_limit", "ipLimit"]));
  addFact(isEN ? "Expires" : "过期时间", expiryFact(row, raw, locale));
  addFact(isEN ? "Last used" : "最后使用", dateFromRaw(raw, ["accessed_time", "accessedTime", "last_used_at", "lastUsedAt"], locale));

  if (facts.length === 0) return null;
  return (
    <div className="asset-fact-strip">
      {facts.slice(0, 6).map((fact) => (
        <span key={`${fact.label}-${fact.value}`} className="asset-fact">
          <small>{fact.label}</small>
          <strong className="mono">{fact.value}</strong>
        </span>
      ))}
    </div>
  );
}

function quotaFact(row: MonitorTarget, raw: Record<string, unknown>, locale: string) {
  if (boolFromRaw(raw, ["unlimited_quota", "unlimitedQuota", "unlimited"])) {
    return locale === "en" ? "Unlimited" : "无限制";
  }
  const value = formatAssetQuota(row, locale);
  if (value !== "—") return value;
  const quota = numberFromRaw(raw, ["quota", "total_quota", "limit", "quota_limit"]);
  return typeof quota === "number"
    ? `${formatNumber(quota)} ${locale === "en" ? "quota" : "额度"}`
    : undefined;
}

function groupRateSummary(
  row: MonitorTarget,
  raw: Record<string, unknown>,
  locale: string,
) {
  const isEN = locale === "en";
  const group = groupNameFromRaw(row, raw);
  const rate = rateFact(raw, locale);
  return {
    primary: group ?? "—",
    secondary: rate
      ? `${isEN ? "Rate" : "倍率"} ${rate}`
      : isEN
        ? "No upstream rate"
        : "上游未返回倍率",
  };
}

function todayUsageFact(
  row: MonitorTarget,
  raw: Record<string, unknown>,
  locale: string,
) {
  const isEN = locale === "en";
  const daily = dailyUsageFact(row, raw, locale);
  const requests = requestFact(raw, locale, [
    "today_requests",
    "todayRequests",
    "daily_requests",
    "dailyRequests",
    "today_request_count",
    "todayRequestCount",
  ]);
  if (daily) {
    return {
      primary: daily,
      secondary: requests ?? (isEN ? "Today from upstream" : "上游今日返回"),
    };
  }
  if (row.monthlyCost) {
    return {
      primary: formatMoney(row.monthlyCost),
      secondary: requests ?? (isEN ? "Accumulated usage" : "累计用量"),
    };
  }
  if (requests) {
    return {
      primary: requests,
      secondary: isEN ? "Request count" : "请求数",
    };
  }
  return {
    primary: "—",
    secondary: isEN ? "No daily usage" : "上游未返回今日用量",
  };
}

function dailyUsageFact(
  row: MonitorTarget,
  raw: Record<string, unknown>,
  locale: string,
) {
  const summaryMoney = formatUsageRangeMoney(usageSummaryRange(raw, "today"), raw);
  if (summaryMoney) return summaryMoney;
  const money = formatRawMoney(raw, [
    "today_actual_cost",
    "todayActualCost",
    "today_cost",
    "todayCost",
    "daily_actual_cost",
    "dailyActualCost",
    "daily_cost",
    "dailyCost",
    "today_usage_usd",
    "todayUsageUsd",
    "daily_usage_usd",
    "dailyUsageUsd",
    "today_amount",
    "todayAmount",
  ]);
  if (money) return money;
  const quota = numberFromRaw(raw, [
    "today_used_quota",
    "todayUsedQuota",
    "today_usage",
    "todayUsage",
    "daily_usage",
    "dailyUsage",
    "daily_used",
    "dailyUsed",
  ]);
  if (typeof quota === "number") {
    return `${formatNumber(quota)} ${locale === "en" ? "quota" : "额度"}`;
  }
  if (
    row.kind === "api_key" &&
    row.monthlyCost &&
    !hasAnyRawNumber(raw, [
      "monthly_cost",
      "monthlyCost",
      "month_cost",
      "monthCost",
      "usage_cost",
      "usageCost",
      "total_cost",
      "totalCost",
    ])
  ) {
    return formatMoney(row.monthlyCost);
  }
  return undefined;
}

function rateFact(raw: Record<string, unknown>, locale: string) {
  const value = numberFromRaw(raw, [
    "ratio",
    "rate",
    "group_ratio",
    "groupRatio",
    "group_rate",
    "groupRate",
    "channel_rate",
    "channelRate",
    "multiplier",
    "model_ratio",
    "modelRatio",
    "quota_multiplier",
    "quotaMultiplier",
  ]);
  const nestedValue = numberFromRaw(objectFromRaw(raw, ["group"]), [
    "rate_multiplier",
    "rateMultiplier",
    "rate",
    "ratio",
    "multiplier",
  ]);
  const finalValue = typeof value === "number" ? value : nestedValue;
  if (typeof finalValue !== "number") {
    return stringFromRaw(raw, ["ratio", "rate", "multiplier"]);
  }
  const suffix = locale === "en" ? "x" : "倍";
  return `${formatNumber(finalValue)}${suffix}`;
}

function requestFact(
  raw: Record<string, unknown>,
  locale: string,
  extraKeys: string[] = [],
) {
  const summaryToday = usageSummaryRange(raw, "today");
  const value = numberFromRaw(summaryToday, ["requests", "totalRequests"]) ?? numberFromRaw(raw, [
    ...extraKeys,
    "request_count",
    "requestCount",
    "requests",
    "total_requests",
    "totalRequests",
    "api_requests",
    "apiRequests",
    "request_num",
    "requestNum",
  ]);
  if (typeof value !== "number") return undefined;
  return `${formatNumber(value)} ${locale === "en" ? "req" : "次"}`;
}

function usageFact(row: MonitorTarget, raw: Record<string, unknown>, locale: string) {
  const summaryMoney = formatUsageRangeMoney(usageSummaryRange(raw, "30d"), raw);
  if (summaryMoney) return summaryMoney;
  if (row.monthlyCost) return formatMoney(row.monthlyCost);
  if (row.providerKind === "newapi_user" || row.providerKind === "sub2api_user") {
    const relayMoney = formatRawMoney(raw, [
      "monthly_cost",
      "monthlyCost",
      "month_cost",
      "monthCost",
      "usage_cost",
      "usageCost",
      "total_cost",
      "totalCost",
      "used_amount",
      "usedAmount",
      "used_usd",
      "usedUSD",
    ]);
    if (relayMoney) return relayMoney;
    return formatQuotaAsMoney(raw, ["used_quota", "usedQuota", "used"]);
  }
  const used = numberFromRaw(raw, ["used_quota", "usedQuota", "used", "usage", "used_amount"]);
  if (typeof used !== "number") return undefined;
  return `${formatNumber(used)} ${locale === "en" ? "quota" : "额度"}`;
}

function expiryFact(
  row: MonitorTarget,
  raw: Record<string, unknown>,
  locale: string,
) {
  if (row.plan?.expireAt) return formatDate(row.plan.expireAt, locale);
  return dateFromRaw(
    raw,
    [
      "expired_time",
      "expiredTime",
      "expires_at",
      "expiresAt",
      "expire_at",
      "expireAt",
      "end_at",
      "endAt",
    ],
    locale,
  );
}

function formatAssetQuota(row: MonitorTarget, locale: string) {
  const raw = rawPayload(row);
  if (boolFromRaw(raw, ["unlimited_quota", "unlimitedQuota", "unlimited"])) {
    const used = numberFromRaw(raw, ["used_quota", "usedQuota", "used", "usage"]);
    const suffix = locale === "en" ? "unlimited" : "无限制";
    if (typeof used === "number") {
      return `${locale === "en" ? "used" : "已用"} ${formatNumber(used)} · ${suffix}`;
    }
    return suffix;
  }
  const quota = row.quota;
  if (!quota) return "—";
  const unit = quota.unit === "quota" && locale !== "en" ? "额度" : quota.unit;
  const values = [quota.remaining, quota.used, quota.total].filter(
    (value): value is number => typeof value === "number",
  );
  if (values.length === 0) return unit;
  if (typeof quota.used === "number" && typeof quota.total === "number") {
    return `${formatNumber(quota.used)} / ${formatNumber(quota.total)} ${unit}`;
  }
  if (typeof quota.remaining === "number" && typeof quota.total === "number") {
    return `${formatNumber(quota.remaining)} / ${formatNumber(quota.total)} ${unit}`;
  }
  return `${formatNumber(values[0])} ${unit}`;
}

function rawPayload(row?: MonitorTarget): Record<string, unknown> {
  const raw = row?.raw;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return {};
  const root = raw as Record<string, unknown>;
  const data = root.data;
  if (data && typeof data === "object" && !Array.isArray(data)) {
    return { ...(data as Record<string, unknown>), ...root };
  }
  return root;
}

function usageSummaryRange(
  raw: Record<string, unknown>,
  range: InstanceUsageRange | "total" | "5h",
): Record<string, unknown> {
  const summary = objectFromRaw(raw, ["usageSummary", "usage_summary"]);
  const ranges = objectFromRaw(summary, ["ranges"]);
  const value = ranges[range];
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  if (range === "24h") {
    const today = ranges.today;
    if (today && typeof today === "object" && !Array.isArray(today)) {
      return today as Record<string, unknown>;
    }
  }
  return {};
}

function stringFromRaw(
  raw: Record<string, unknown>,
  keys: string[],
): string | undefined {
  for (const key of keys) {
    const value = raw[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
    if (typeof value === "boolean") return value ? "true" : "false";
  }
  return undefined;
}

function numberFromRaw(
  raw: Record<string, unknown>,
  keys: string[],
): number | undefined {
  for (const key of keys) {
    const value = raw[key];
    if (typeof value === "number" && Number.isFinite(value)) return value;
    if (typeof value === "string") {
      const parsed = Number(value.trim());
      if (Number.isFinite(parsed)) return parsed;
    }
  }
  return undefined;
}

function boolFromRaw(raw: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = raw[key];
    if (typeof value === "boolean") return value;
    if (typeof value === "number") return value !== 0;
    if (typeof value === "string") {
      const text = value.trim().toLowerCase();
      if (["true", "1", "yes"].includes(text)) return true;
      if (["false", "0", "no"].includes(text)) return false;
    }
  }
  return false;
}

function objectFromRaw(
  raw: Record<string, unknown>,
  keys: string[],
): Record<string, unknown> {
  for (const key of keys) {
    const value = raw[key];
    if (value && typeof value === "object" && !Array.isArray(value)) {
      return value as Record<string, unknown>;
    }
  }
  return {};
}

function groupNameFromRaw(
  row: MonitorTarget,
  raw: Record<string, unknown>,
): string | undefined {
  const direct =
    row.groupName ??
    stringFromRaw(raw, [
      "group_name",
      "groupName",
      "channel_group",
      "channelGroup",
      "default_group",
      "defaultGroup",
    ]);
  if (direct) return direct;
  const groupObject = objectFromRaw(raw, ["group"]);
  const nested = stringFromRaw(groupObject, [
    "name",
    "display_name",
    "displayName",
    "title",
    "slug",
  ]);
  if (nested) return nested;
  const group = raw.group;
  if (typeof group === "string" && group.trim()) return group.trim();
  const groupId = stringFromRaw(raw, ["group_id", "groupId"]);
  return groupId ? `Group ${groupId}` : undefined;
}

function formatQuotaAsMoney(
  raw: Record<string, unknown>,
  keys: string[],
): string | undefined {
  const value = numberFromRaw(raw, keys);
  if (typeof value !== "number") return undefined;
  const scale = numberFromRaw(raw, [
    "quota_per_unit",
    "quotaPerUnit",
    "quota_unit_scale",
  ]);
  const currency = stringFromRaw(raw, ["currency", "balance_currency"]) ?? "USD";
  const amount = value / (scale && scale > 0 ? scale : 500000);
  return formatMoney({ amount, currency });
}

function formatRawMoney(
  raw: Record<string, unknown>,
  keys: string[],
): string | undefined {
  const value = numberFromRaw(raw, keys);
  if (typeof value !== "number") return undefined;
  const currency =
    stringFromRaw(raw, [
      "currency",
      "balance_currency",
      "balanceCurrency",
      "monthly_cost_currency",
      "monthlyCostCurrency",
      "cost_currency",
      "costCurrency",
    ]) ?? "USD";
  return formatMoney({ amount: value, currency });
}

function formatUsageRangeMoney(
  raw: Record<string, unknown>,
  fallbackRaw: Record<string, unknown>,
): string | undefined {
  const value = numberFromRaw(raw, [
    "actualCost",
    "actual_cost",
    "cost",
    "totalActualCost",
    "total_actual_cost",
    "totalCost",
    "total_cost",
  ]);
  if (typeof value !== "number") return undefined;
  const currency =
    stringFromRaw(raw, ["currency"]) ??
    stringFromRaw(objectFromRaw(fallbackRaw, ["usageSummary", "usage_summary"]), [
      "currency",
    ]) ??
    "USD";
  return formatMoney({ amount: value, currency });
}

function hasAnyRawNumber(raw: Record<string, unknown>, keys: string[]) {
  return typeof numberFromRaw(raw, keys) === "number";
}

function listFromRaw(
  raw: Record<string, unknown>,
  keys: string[],
): string | undefined {
  for (const key of keys) {
    const value = raw[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (Array.isArray(value)) {
      const items = value
        .map((item) => {
          if (typeof item === "string") return item;
          if (item && typeof item === "object") {
            return stringFromRaw(item as Record<string, unknown>, [
              "name",
              "model",
              "id",
              "value",
            ]);
          }
          return undefined;
        })
        .filter((item): item is string => Boolean(item));
      if (items.length > 0) return items.slice(0, 3).join(", ");
    }
    if (value && typeof value === "object") {
      const keys = Object.keys(value as Record<string, unknown>);
      if (keys.length > 0) return keys.slice(0, 3).join(", ");
    }
  }
  return undefined;
}

function dateFromRaw(
  raw: Record<string, unknown>,
  keys: string[],
  locale: string,
): string | undefined {
  const value = stringFromRaw(raw, keys);
  if (!value) return undefined;
  const numeric = Number(value);
  let date: Date;
  if (Number.isFinite(numeric)) {
    if (numeric <= 0) return undefined;
    date = new Date(numeric > 1_000_000_000_000 ? numeric : numeric * 1000);
  } else {
    date = new Date(value);
  }
  if (Number.isNaN(date.getTime())) return undefined;
  return formatDate(date.toISOString(), locale);
}

function maskSecret(value?: string) {
  if (!value) return undefined;
  if (value.includes("*") || value.length <= 12) return value;
  return `${value.slice(0, 6)}...${value.slice(-4)}`;
}

function planExpiryText(row: MonitorTarget, locale: string) {
  if (!row.plan?.expireAt) return "";
  const label = locale === "en" ? "Expires" : "到期";
  return `${label} ${formatDate(row.plan.expireAt, locale)}`;
}

function usageWindows(row: MonitorTarget): UsageWindow[] {
  const raw = row.raw;
  if (!raw) return [];
  if (Array.isArray(raw.usageWindows)) return raw.usageWindows;
  if (Array.isArray(raw.usage_windows)) return raw.usage_windows;
  return [];
}

function WindowMini({
  windows,
  locale,
}: {
  windows: UsageWindow[];
  locale: string;
}) {
  if (windows.length === 0) return null;
  return (
    <div className="window-strip window-strip-compact">
      {windows.slice(0, 2).map((window, index) => {
        const value = windowPercent(window);
        const label = shortWindowLabel(window);
        return (
          <div
            className="window-strip-item"
            key={window.key ?? `${window.label}-${index}`}
          >
            <span
              className={`window-chip ${value <= 20 ? "danger" : value <= 50 ? "warn" : ""}`}
            >
              {label}
            </span>
            <div className="window-mini-track" aria-hidden="true">
              <span style={{ width: `${value}%` }} />
            </div>
            <strong>{value}%</strong>
            <small>{resetText(window, locale)}</small>
          </div>
        );
      })}
    </div>
  );
}

function shortWindowLabel(window: UsageWindow) {
  const raw = (window.window || window.label || window.key || "window").trim();
  return raw
    .replace(/\s*window$/i, "")
    .replace(/\s+/g, " ")
    .replace(/^(\d+)\s*hours?$/i, "$1h")
    .replace(/^(\d+)\s*days?$/i, "$1d");
}

function windowPercent(window: UsageWindow) {
  if (
    typeof window.remaining === "number" &&
    typeof window.total === "number" &&
    window.total > 0
  ) {
    return clampPercent((window.remaining / window.total) * 100);
  }
  if (typeof window.utilization === "number") {
    const utilization =
      window.utilization > 1 ? window.utilization : window.utilization * 100;
    return clampPercent(100 - utilization);
  }
  if (
    typeof window.used === "number" &&
    typeof window.total === "number" &&
    window.total > 0
  ) {
    return clampPercent(100 - (window.used / window.total) * 100);
  }
  return 0;
}

function resetText(window: UsageWindow, locale: string) {
  if (!window.resetAt) return window.status ?? "";
  const timestamp = Date.parse(window.resetAt);
  if (Number.isNaN(timestamp)) return window.resetAt;
  const diffMs = timestamp - Date.now();
  const absMinutes = Math.max(0, Math.round(Math.abs(diffMs) / 60000));
  if (absMinutes < 60)
    return diffMs >= 0 ? `${absMinutes}m` : locale === "en" ? "now" : "现在";
  const hours = absMinutes / 60;
  if (hours < 48) return `${formatNumber(hours)}h`;
  return new Intl.DateTimeFormat(locale, {
    month: "numeric",
    day: "numeric",
  }).format(timestamp);
}

function clampPercent(value: number) {
  return Math.max(0, Math.min(100, Math.round(value)));
}

export function AssetDetailPage() {
  const { t } = useTranslation();
  const { id } = useParams<{ id: string }>();
  if (!id) return null;
  return (
    <AppShell title={t("assets.title")} description={t("common.viewDetails")}>
      <Link to="/assets" className="btn btn-ghost btn-sm mb-4 inline-flex">
        {t("common.back")}
      </Link>
      <AssetDetailDrawer targetId={id} onClose={() => {}} inline />
    </AppShell>
  );
}
