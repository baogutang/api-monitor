import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import { targetsApi } from "@/api/services";
import { Button } from "@/components/ui/Button";
import { CapabilityBadge, StatusBadge } from "@/components/ui/Badge";
import { ErrorState, LoadingSkeleton } from "@/components/ui/State";
import {
  capabilityLabel,
  formatDate,
  formatMoney,
  formatNumber,
  formatQuota,
  providerLabel,
} from "@/lib/format";
import { usePreferences } from "@/contexts/PreferencesContext";
import type { UsageWindow, MonitorTarget } from "@/lib/types";

type Tab = "overview" | "snapshots" | "usage" | "alerts" | "raw";

export function AssetDetailDrawer({
  targetId,
  onClose,
  inline,
}: {
  targetId: string;
  onClose: () => void;
  inline?: boolean;
}) {
  const { t } = useTranslation();
  const { resolvedLocale } = usePreferences();
  const [tab, setTab] = useState<Tab>("overview");

  const target = useQuery({
    queryKey: ["target", targetId],
    queryFn: () => targetsApi.get(targetId),
  });

  const snapshots = useQuery({
    queryKey: ["target-snapshots", targetId],
    queryFn: () => targetsApi.snapshots(targetId, "7d"),
    enabled: tab === "snapshots" || tab === "usage",
  });

  const alerts = useQuery({
    queryKey: ["target-alerts", targetId],
    queryFn: () => targetsApi.alerts(targetId),
    enabled: tab === "alerts",
  });

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: t("assets.detailOverview") },
    { id: "snapshots", label: t("assets.detailSnapshots") },
    { id: "usage", label: t("assets.detailUsage") },
    { id: "alerts", label: t("assets.detailAlerts") },
    { id: "raw", label: t("assets.detailRaw") },
  ];

  const content = (
    <>
      <div className="drawer-header">
        <div>
          <h2 className="text-lg font-semibold">{target.data?.name ?? "…"}</h2>
          <p className="text-xs text-text-4 mt-1">
            {target.data ? providerLabel(target.data.providerKind, t) : ""}
          </p>
        </div>
        {!inline && (
          <Button size="icon" variant="ghost" onClick={onClose}>
            <X size={18} />
          </Button>
        )}
      </div>

      <div className="tabs">
        {tabs.map((tb) => (
          <button
            key={tb.id}
            className={`tab${tab === tb.id ? " active" : ""}`}
            onClick={() => setTab(tb.id)}
          >
            {tb.label}
          </button>
        ))}
      </div>

      <div className="drawer-body">
        {target.isLoading ? (
          <LoadingSkeleton rows={4} />
        ) : target.isError ? (
          <ErrorState onRetry={() => void target.refetch()} />
        ) : (
          <>
            {tab === "overview" && target.data && (
              <div className="flex flex-col gap-4">
                <StatusBadge
                  status={target.data.status}
                  label={t(`status.${target.data.status}`)}
                />
                <div className="grid grid-cols-2 gap-3 text-sm">
                  <div>
                    <div className="text-text-4 text-xs">
                      {t("assets.balance")}
                    </div>
                    <div className="mono font-medium">
                      {formatMoney(target.data.balance)}
                    </div>
                  </div>
                  <div>
                    <div className="text-text-4 text-xs">
                      {t("assets.quota")}
                    </div>
                    <div className="mono font-medium">
                      {formatQuota(target.data.quota, resolvedLocale)}
                    </div>
                  </div>
                  <div>
                    <div className="text-text-4 text-xs">
                      {t("assets.monthlyCost")}
                    </div>
                    <div className="mono font-medium">
                      {formatMoney(target.data.monthlyCost)}
                    </div>
                  </div>
                  <div>
                    <div className="text-text-4 text-xs">
                      {t("assets.lastScan")}
                    </div>
                    <div>
                      {formatDate(target.data.lastScanAt, resolvedLocale)}
                    </div>
                  </div>
                </div>
                <div className="flex flex-wrap gap-1">
                  {target.data.capabilities.map((c) => (
                    <CapabilityBadge key={c} label={capabilityLabel(c, t)} />
                  ))}
                </div>
                <UsageWindowPanel
                  windows={usageWindows(target.data)}
                  locale={resolvedLocale}
                  availableLabel={
                    resolvedLocale === "en" ? "available" : "可用"
                  }
                  resetLabel={resolvedLocale === "en" ? "reset" : "重置"}
                  nowLabel={resolvedLocale === "en" ? "now" : "现在"}
                />
                <WatchPreview target={target.data} locale={resolvedLocale} />
                {target.data.keyFingerprint && (
                  <div className="text-xs text-text-4">
                    {t("common.fingerprint")}:{" "}
                    <span className="mono">{target.data.keyFingerprint}</span>
                  </div>
                )}
              </div>
            )}

            {tab === "snapshots" && (
              <div className="data-table-wrap">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>{t("scans.startedAt")}</th>
                      <th>{t("assets.balance")}</th>
                      <th>{t("common.status")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(snapshots.data ?? []).map((s) => (
                      <tr key={s.id}>
                        <td>{formatDate(s.capturedAt, resolvedLocale)}</td>
                        <td className="mono">{formatMoney(s.balance)}</td>
                        <td>{t(`status.${s.status}`)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {tab === "usage" && (
              <div className="text-sm text-text-3 flex flex-col gap-4">
                {target.data && (
                  <UsageWindowPanel
                    windows={usageWindows(target.data)}
                    locale={resolvedLocale}
                    availableLabel={
                      resolvedLocale === "en" ? "available" : "可用"
                    }
                    resetLabel={resolvedLocale === "en" ? "reset" : "重置"}
                    nowLabel={resolvedLocale === "en" ? "now" : "现在"}
                  />
                )}
                {(snapshots.data ?? []).length === 0 ? (
                  t("common.noData")
                ) : (
                  <ul className="flex flex-col gap-2">
                    {snapshots.data?.map((s) => (
                      <li
                        key={s.id}
                        className="flex justify-between border-b border-border pb-2"
                      >
                        <span>{formatDate(s.capturedAt, resolvedLocale)}</span>
                        <span className="mono">
                          {formatQuota(s.quota, resolvedLocale)}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}

            {tab === "alerts" && (
              <div className="flex flex-col gap-3">
                {(alerts.data ?? []).length === 0 ? (
                  <p className="text-text-4">{t("common.noData")}</p>
                ) : (
                  alerts.data?.map((a) => (
                    <div key={a.id} className="card p-3">
                      <div className="font-medium text-sm">{a.title}</div>
                      <div className="text-xs text-text-4 mt-1">
                        {a.message}
                      </div>
                    </div>
                  ))
                )}
              </div>
            )}

            {tab === "raw" && (
              <pre className="text-xs mono bg-bg-elevated p-3 rounded-md overflow-auto max-h-96">
                {JSON.stringify(
                  snapshots.data?.[0]?.raw ?? target.data,
                  null,
                  2,
                )}
              </pre>
            )}
          </>
        )}
      </div>
    </>
  );

  if (inline) {
    return <div className="card">{content}</div>;
  }

  return (
    <>
      <div className="drawer-overlay" onClick={onClose} />
      <div className="drawer">{content}</div>
    </>
  );
}

function WatchPreview({
  target,
  locale,
}: {
  target: MonitorTarget;
  locale: string;
}) {
  if (!isWatchKind(target.kind)) return null;
  const raw = target.raw ?? {};
  const items = Array.isArray(raw.items)
    ? (raw.items as Array<Record<string, unknown>>)
    : [];
  const sourceUrl = stringFromRaw(raw, ["sourceUrl", "source"]);
  const fingerprint = stringFromRaw(raw, ["fingerprint"]);
  const summary = stringFromRaw(raw, ["summary"]);
  const isEN = locale === "en";
  return (
    <section className="watch-preview-panel">
      <div className="watch-preview-head">
        <div>
          <strong>{isEN ? "Watched source" : "观察源预览"}</strong>
          {summary && <span>{summary}</span>}
        </div>
        {fingerprint && <code>{fingerprint.slice(0, 12)}</code>}
      </div>
      {sourceUrl && (
        <a href={sourceUrl} target="_blank" rel="noreferrer" className="watch-source-link">
          {sourceUrl}
        </a>
      )}
      {items.length === 0 ? (
        <p className="text-xs text-text-4">
          {isEN ? "No items returned yet." : "还没有返回可预览条目。"}
        </p>
      ) : (
        <div className="watch-item-list">
          {items.slice(0, 6).map((item, index) => {
            const title = stringFromRaw(item, ["title", "name"]) ??
              (isEN ? "Untitled item" : "未命名条目");
            const itemSummary = stringFromRaw(item, ["summary", "description"]);
            const url = stringFromRaw(item, ["url", "link"]);
            return (
              <article key={`${title}-${index}`} className="watch-item">
                <div>
                  <strong>{title}</strong>
                  {itemSummary && <p>{itemSummary}</p>}
                </div>
                {url && (
                  <a href={url} target="_blank" rel="noreferrer">
                    {isEN ? "Open" : "打开"}
                  </a>
                )}
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
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

function stringFromRaw(
  raw: Record<string, unknown>,
  keys: string[],
): string | undefined {
  for (const key of keys) {
    const value = raw[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (typeof value === "number" && Number.isFinite(value)) return String(value);
  }
  return undefined;
}

function usageWindows(target?: MonitorTarget): UsageWindow[] {
  const raw = target?.raw;
  if (!raw) return [];
  if (Array.isArray(raw.usageWindows)) return raw.usageWindows;
  if (Array.isArray(raw.usage_windows)) return raw.usage_windows;
  return [];
}

function UsageWindowPanel({
  windows,
  locale,
  availableLabel,
  resetLabel,
  nowLabel,
}: {
  windows: UsageWindow[];
  locale: string;
  availableLabel: string;
  resetLabel: string;
  nowLabel: string;
}) {
  if (windows.length === 0) return null;
  return (
    <div className="usage-window-panel">
      {windows.map((window, index) => {
        const remainingPercent = windowPercent(window);
        const usedPercent = 100 - remainingPercent;
        return (
          <div
            className="usage-window-card"
            key={window.key ?? `${window.label}-${index}`}
          >
            <div className="usage-window-top">
              <div>
                <span
                  className={`window-chip ${remainingPercent <= 20 ? "danger" : remainingPercent <= 50 ? "warn" : ""}`}
                >
                  {shortWindowLabel(window)}
                </span>
                <strong>{window.label}</strong>
              </div>
              <span>
                {remainingPercent}% {availableLabel}
              </span>
            </div>
            <div className="usage-window-track">
              <span style={{ width: `${usedPercent}%` }} />
            </div>
            <div className="usage-window-meta">
              <span>
                {window.used != null ? formatNumber(window.used) : "—"} /{" "}
                {window.total != null ? formatNumber(window.total) : "—"}{" "}
                {window.unit ?? ""}
              </span>
              <span>
                {window.resetAt
                  ? `${resetLabel} ${formatReset(window.resetAt, locale, nowLabel)}`
                  : (window.status ?? "unknown")}
              </span>
            </div>
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

function formatReset(value: string, locale: string, nowLabel: string) {
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) return value;
  const minutes = Math.max(0, Math.round((timestamp - Date.now()) / 60000));
  if (minutes < 1) return nowLabel;
  if (minutes < 60) return `${minutes}m`;
  const hours = minutes / 60;
  if (hours < 48) return `${formatNumber(hours)}h`;
  return new Intl.DateTimeFormat(locale, {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(timestamp);
}

function clampPercent(value: number) {
  return Math.max(0, Math.min(100, Math.round(value)));
}
