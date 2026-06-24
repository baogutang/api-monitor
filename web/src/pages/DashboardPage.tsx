import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import {
  CategoryScale,
  Chart as ChartJS,
  Filler,
  LinearScale,
  LineController,
  LineElement,
  PointElement,
  Tooltip,
  type ChartConfiguration,
} from "chart.js";
import { alertsApi, dashboardApi, instancesApi, targetsApi } from "@/api/services";
import { AppShell } from "@/components/layout/AppShell";
import { ErrorState, LoadingSkeleton } from "@/components/ui/State";
import { formatMoney, providerLabel } from "@/lib/format";
import type {
  MonitorTarget,
  Money,
  ProviderKind,
  TrendPoint,
  Instance,
} from "@/lib/types";
import { usePreferences } from "@/contexts/PreferencesContext";

ChartJS.register(
  LineController,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Filler,
  Tooltip,
);

type RangeId = "1h" | "24h" | "7d" | "30d";

const ranges: RangeId[] = ["1h", "24h", "7d", "30d"];

export function DashboardPage() {
  const { t } = useTranslation();
  const { resolvedLocale, resolvedTheme } = usePreferences();
  const [showBanner, setShowBanner] = useState(true);
  const [range, setRange] = useState<RangeId>("24h");

  const summary = useQuery({
    queryKey: ["dashboard-summary"],
    queryFn: dashboardApi.summary,
    refetchInterval: 15000,
  });
  const trend = useQuery({
    queryKey: ["dashboard-trends", range],
    queryFn: () => dashboardApi.trends(range),
    refetchInterval: 15000,
    retry: false,
  });
  const targets = useQuery({
    queryKey: ["dashboard-targets"],
    queryFn: () => targetsApi.list({ limit: 200 }),
    refetchInterval: 15000,
    retry: false,
  });
  const instances = useQuery({
    queryKey: ["dashboard-instances"],
    queryFn: instancesApi.list,
    refetchInterval: 30000,
    retry: false,
  });
  const recentAlerts = useQuery({
    queryKey: ["dashboard-alerts"],
    queryFn: () => alertsApi.list({ limit: 6 }),
    refetchInterval: 15000,
  });

  const s = summary.data;
  const targetItems = targets.data?.items ?? [];
  const instanceMap = useMemo(
    () =>
      new Map(
        (instances.data ?? []).map((instance) => [instance.id, instance]),
      ),
    [instances.data],
  );
  const balanceTargets = useMemo(
    () => targetItems.filter((target) => isTopLevelBalanceTarget(target)),
    [targetItems],
  );
  const openAlertCount = s?.openAlerts ?? 0;
  const criticalAlertCount = s?.criticalAlerts ?? 0;
  const totalBalance = useMemo(
    () => s?.totalBalance ?? sumBalances(balanceTargets),
    [s?.totalBalance, balanceTargets],
  );
  const trendSeries = useMemo(
    () => buildTrendSeries(trend.data ?? [], range, resolvedLocale),
    [trend.data, range, resolvedLocale],
  );
  const todaySpend = s?.todayCost;

  const refetchAll = () => {
    void summary.refetch();
    void trend.refetch();
    void targets.refetch();
    void instances.refetch();
    void recentAlerts.refetch();
  };

  return (
    <AppShell
      title={t("dashboard.title")}
      description={t("dashboard.desc")}
      onRefresh={refetchAll}
      refreshing={summary.isFetching || trend.isFetching || targets.isFetching}
      actions={
        criticalAlertCount > 0 ? (
          <span
            className="status-pill"
            style={{
              color: "var(--err)",
              borderColor: "color-mix(in srgb, var(--err) 28%, transparent)",
              background: "color-mix(in srgb, var(--err) 9%, transparent)",
            }}
          >
            <span className="dot dot-crit" />
            {criticalAlertCount} 严重告警
          </span>
        ) : null
      }
    >
      {summary.isLoading ? (
        <LoadingSkeleton rows={8} />
      ) : summary.isError ? (
        <ErrorState
          message={t("errors.network")}
          onRetry={() => void summary.refetch()}
        />
      ) : (
        <>
          {showBanner && criticalAlertCount > 0 && (
            <div className="incident-banner">
              <span className="banner-dot" />
              <span className="incident-banner-msg">
                <strong>{criticalAlertCount} 条严重告警仍未关闭</strong>
                ，请到告警中心确认低余额、低额度、扫描失败或变更监控事件。
              </span>
              <button
                className="incident-close"
                type="button"
                aria-label="关闭"
                onClick={() => setShowBanner(false)}
              >
                <X size={14} />
              </button>
            </div>
          )}

          <section className="mc-stat-grid">
            <StatCard
              primary
              label="上游总余额"
              value={formatMoneyOrZero(totalBalance)}
              sub={`← ${balanceTargets.length || s?.activeChannels || 0} 个渠道合计`}
            />
            <StatCard
              label="今日消耗"
              value={formatMoney(todaySpend)}
              sub={todaySpend ? "来自今日快照差值" : "暂无今日消耗差值"}
            />
            <StatCard
              label="本月累计消耗"
              value={formatMoney(s?.monthlyCost)}
              sub={s?.monthlyCost ? "来自后端汇总" : "暂无月成本数据"}
            />
            <StatCard
              label="活跃渠道"
              value={`${s?.activeChannels ?? balanceTargets.length} / ${balanceTargets.length || s?.activeChannels || 0}`}
              sub={`${openAlertCount} 条未关闭告警 · ${s?.riskTargets ?? 0} 个风险资产`}
            />
          </section>

          <section className="mc-mid">
            <div className="card">
              <div className="card-hdr">
                <div className="card-ttl">
                  <span className="d" />
                  {trendSeries?.title ?? "余额趋势"}
                </div>
                <div
                  className="time-tabs"
                  role="tablist"
                  aria-label="趋势时间范围"
                >
                  {ranges.map((item) => (
                    <button
                      key={item}
                      type="button"
                      className={`tt${range === item ? " active" : ""}`}
                      onClick={() => setRange(item)}
                    >
                      {item.toUpperCase()}
                    </button>
                  ))}
                </div>
              </div>
              <div className="chart-box">
                {trendSeries ? (
                  <MissionAreaChart
                    labels={trendSeries.labels}
                    values={trendSeries.values}
                    currency={trendSeries.currency}
                    label={trendSeries.title}
                    theme={resolvedTheme}
                  />
                ) : (
                  <DashboardEmpty
                    title="暂无消耗趋势"
                    description="后端还没有返回趋势点。等扫描和用量采集产生数据后，这里会展示真实曲线。"
                  />
                )}
              </div>
            </div>

            <div className="card">
              <div className="card-hdr">
                <div className="card-ttl">
                  <span className="d" />
                  上游渠道余额
                </div>
              </div>
              <div className="prov-list">
                {balanceTargets.length === 0 ? (
                  <DashboardEmpty
                    title="暂无上游渠道"
                    description="添加实例并同步监控资产后，这里会显示真实余额、额度和健康状态。"
                    compact
                  />
                ) : (
                  balanceTargets
                    .slice(0, 7)
                    .map((target) => (
                      <ProviderCard
                        key={target.id}
                        target={target}
                        instance={instanceMap.get(target.instanceId)}
                        t={t}
                      />
                    ))
                )}
              </div>
            </div>
          </section>

          <section className="mc-bot">
            <div className="card">
              <div className="card-hdr">
                <div className="card-ttl">
                  <span className="d" />
                  模型调用分布（今日）
                </div>
              </div>
              <DashboardEmpty
                title="暂无模型调用统计"
                description="当前后端还没有模型级 token 统计接口，因此这里不展示模拟数据。"
              />
            </div>

            <div className="card">
              <div className="card-hdr">
                <div className="card-ttl">
                  <span className="d" />
                  最近调用记录
                </div>
              </div>
              <DashboardEmpty
                title="暂无调用记录"
                description="当前后端还没有调用交易流水接口，因此这里不展示模拟记录。"
              />
            </div>
          </section>

          {(recentAlerts.data?.items ?? []).length > 0 && (
            <section className="card mt-4">
              <div className="card-hdr">
                <div className="card-ttl">
                  <span className="d" />
                  最近告警事件
                </div>
              </div>
              {(recentAlerts.data?.items ?? []).slice(0, 4).map((a) => (
                <div key={a.id} className="alert-row">
                  <span
                    className={`dot dot-${a.severity === "critical" || a.severity === "phone" ? "crit" : a.severity === "warning" ? "warn" : "muted"}`}
                  />
                  <div className="alert-row-body">
                    <div className="alert-row-title">{a.title}</div>
                    <div className="alert-row-meta">{a.message}</div>
                  </div>
                  <div className="alert-row-time">
                    {new Intl.DateTimeFormat(resolvedLocale, {
                      hour: "2-digit",
                      minute: "2-digit",
                    }).format(new Date(a.openedAt))}
                  </div>
                </div>
              ))}
            </section>
          )}
        </>
      )}
    </AppShell>
  );
}

function DashboardEmpty({
  title,
  description,
  compact,
}: {
  title: string;
  description: string;
  compact?: boolean;
}) {
  return (
    <div
      className={`dashboard-empty${compact ? " dashboard-empty-compact" : ""}`}
    >
      <div>{title}</div>
      <p>{description}</p>
    </div>
  );
}

function StatCard({
  label,
  value,
  sub,
  tone,
  primary,
}: {
  label: string;
  value: string;
  sub?: string;
  tone?: "up" | "down";
  primary?: boolean;
}) {
  return (
    <div className={`s-card${primary ? " prim" : ""}`}>
      <div className="s-lbl">{label}</div>
      <div className={`s-val${primary ? " ac" : ""}`}>{value}</div>
      {sub && (
        <div
          className={`s-chg ${tone === "up" ? "up" : tone === "down" ? "dn" : "neu"}`}
        >
          {sub}
        </div>
      )}
      {primary && (
        <div className="rings" aria-hidden="true">
          <div className="ring" />
          <div className="ring" />
          <div className="ring" />
        </div>
      )}
    </div>
  );
}

function ProviderCard({
  target,
  instance,
  t,
}: {
  target: MonitorTarget;
  instance?: Instance;
  t: (key: string) => string;
}) {
  const displayName = instance?.name ?? target.name;
  const subName =
    instance?.name && instance.name !== target.name ? target.name : target.groupName;
  const visual = providerVisual(target.providerKind, displayName);
  const percent = balancePercent(target);
  const tone =
    target.status === "critical"
      ? "off"
      : target.status === "warning"
        ? "warn"
        : "on";
  const fill =
    target.status === "critical"
      ? "var(--err)"
      : target.status === "warning" || percent < 30
        ? "var(--warn)"
        : "var(--ok)";

  return (
    <div className="prov-item">
      <div
        className="prov-ico"
        style={{ background: visual.bg, color: visual.color }}
      >
        {visual.mark}
      </div>
      <div className="prov-info">
        <div className="prov-name">{displayName}</div>
        <div className="prov-model">
          {providerLabel(target.providerKind, t)} /{" "}
          {subName || target.kind}
        </div>
      </div>
      <div className="prov-bal">
        <div className="prov-amt">{formatMoney(target.balance)}</div>
        <div className="prov-bar">
          <div
            className="prov-bar-fill"
            style={{ width: `${percent}%`, background: fill }}
          />
        </div>
      </div>
      <div className={`prov-status ${tone}`} />
    </div>
  );
}

function MissionAreaChart({
  labels,
  values,
  currency,
  label,
  theme,
}: {
  labels: string[];
  values: number[];
  currency: string;
  label: string;
  theme: "light" | "dark";
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  useEffect(() => {
    if (!canvasRef.current) return undefined;
    const ctx = canvasRef.current.getContext("2d");
    if (!ctx) return undefined;

    const gradient = ctx.createLinearGradient(0, 0, 0, 220);
    gradient.addColorStop(
      0,
      theme === "dark" ? "rgba(56,189,248,.18)" : "rgba(2,132,199,.22)",
    );
    gradient.addColorStop(
      1,
      theme === "dark" ? "rgba(56,189,248,.01)" : "rgba(2,132,199,.02)",
    );

    const tickColor = theme === "dark" ? "#475569" : "#64748B";
    const tooltipBg =
      theme === "dark" ? "rgba(6,10,22,.96)" : "rgba(255,255,255,.96)";
    const tooltipText = theme === "dark" ? "#94A3B8" : "#334155";
    const config: ChartConfiguration<"line"> = {
      type: "line",
      data: {
        labels,
        datasets: [
          {
            label,
            data: values,
            borderColor: theme === "dark" ? "#38BDF8" : "#0284C7",
            borderWidth: 1.5,
            backgroundColor: gradient,
            fill: true,
            tension: 0.42,
            pointRadius: 0,
            pointHoverRadius: 4,
            pointHoverBackgroundColor: theme === "dark" ? "#38BDF8" : "#0284C7",
            pointHoverBorderColor:
              theme === "dark" ? "rgba(255,255,255,.8)" : "rgba(15,23,42,.5)",
            pointHoverBorderWidth: 1.5,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { intersect: false, mode: "index" },
        plugins: {
          legend: { display: false },
          tooltip: {
            backgroundColor: tooltipBg,
            borderColor:
              theme === "dark" ? "rgba(56,189,248,.28)" : "rgba(2,132,199,.24)",
            borderWidth: 1,
            titleColor: tooltipText,
            bodyColor: theme === "dark" ? "#38BDF8" : "#0284C7",
            bodyFont: { family: "'JetBrains Mono', monospace", size: 12 },
            titleFont: { family: "'Inter', sans-serif", size: 10 },
            padding: 10,
            cornerRadius: 8,
            callbacks: {
              label: (context) =>
                formatMoney({ amount: Number(context.parsed.y), currency }),
            },
          },
        },
        scales: {
          x: {
            ticks: {
              color: tickColor,
              font: { family: "'JetBrains Mono', monospace", size: 9.5 },
              maxTicksLimit: 8,
              maxRotation: 0,
            },
            grid: {
              color:
                theme === "dark"
                  ? "rgba(255,255,255,.03)"
                  : "rgba(15,23,42,.055)",
            },
            border: { display: false },
          },
          y: {
            ticks: {
              color: tickColor,
              font: { family: "'JetBrains Mono', monospace", size: 9.5 },
              callback: (value) =>
                formatMoney({
                  amount: Number(Number(value).toFixed(1)),
                  currency,
                }),
            },
            grid: {
              color:
                theme === "dark"
                  ? "rgba(255,255,255,.04)"
                  : "rgba(15,23,42,.06)",
            },
            border: { display: false },
          },
        },
      },
    };
    const chart = new ChartJS(ctx, config);
    return () => chart.destroy();
  }, [currency, label, labels, values, theme]);

  return (
    <canvas ref={canvasRef} role="img" aria-label={label} />
  );
}

function buildTrendSeries(
  points: TrendPoint[],
  range: RangeId,
  locale: string,
) {
  const hasCost = points.some((point) => point.cost?.amount != null);
  const usablePoints = points.filter((point) =>
    hasCost ? point.cost?.amount != null : point.balance?.amount != null,
  );
  if (usablePoints.length === 0) return null;
  const firstMoney = hasCost ? usablePoints[0].cost : usablePoints[0].balance;
  return {
    title: `${hasCost ? "消耗趋势" : "余额趋势"}（${firstMoney?.currency ?? "USD"}）`,
    currency: firstMoney?.currency ?? "USD",
    labels: usablePoints.map((point) =>
      formatTrendLabel(point.capturedAt, range, locale),
    ),
    values: usablePoints.map((point) =>
      Number(
        ((hasCost ? point.cost?.amount : point.balance?.amount) ?? 0).toFixed(2),
      ),
    ),
  };
}

function formatTrendLabel(iso: string, range: RangeId, locale: string) {
  const date = new Date(iso);
  if (range === "1h" || range === "24h") {
    return new Intl.DateTimeFormat(locale, {
      hour: "2-digit",
      minute: "2-digit",
    }).format(date);
  }
  return new Intl.DateTimeFormat(locale, {
    month: "2-digit",
    day: "2-digit",
  }).format(date);
}

function sumBalances(targets: MonitorTarget[]): Money {
  const firstCurrency =
    targets.find((item) => item.balance)?.balance?.currency || "USD";
  return {
    currency: firstCurrency,
    amount: targets.reduce(
      (sum, target) => sum + (target.balance?.amount ?? 0),
      0,
    ),
  };
}

function formatMoneyOrZero(money: Money) {
  if (!money.amount)
    return formatMoney({ amount: 0, currency: money.currency || "USD" });
  return formatMoney(money);
}

function isTopLevelBalanceTarget(target: MonitorTarget) {
  return target.kind === "user" || target.kind === "subscription";
}

function balancePercent(target: MonitorTarget) {
  if (
    target.quota?.remaining != null &&
    target.quota.total != null &&
    target.quota.total > 0
  ) {
    return clamp(
      Math.round((target.quota.remaining / target.quota.total) * 100),
      4,
      100,
    );
  }
  if (target.riskScore > 0) return clamp(100 - target.riskScore, 4, 100);
  return target.status === "critical"
    ? 12
    : target.status === "warning"
      ? 32
      : 74;
}

function providerVisual(kind: ProviderKind, name: string) {
  const map: Partial<
    Record<ProviderKind, { mark: string; color: string; bg: string }>
  > = {
    anthropic_key: { mark: "Cl", color: "#FF8C60", bg: "rgba(255,140,96,.1)" },
    anthropic_account: {
      mark: "Cl",
      color: "#FF8C60",
      bg: "rgba(255,140,96,.1)",
    },
    openai_key: { mark: "AI", color: "#10A37F", bg: "rgba(16,163,127,.1)" },
    openai_admin: { mark: "AI", color: "#10A37F", bg: "rgba(16,163,127,.1)" },
    openai_account: { mark: "AI", color: "#10A37F", bg: "rgba(16,163,127,.1)" },
    gemini_account: { mark: "G", color: "#4285F4", bg: "rgba(66,133,244,.1)" },
    sub2api_user: { mark: "S2", color: "#A78BFA", bg: "rgba(167,139,250,.12)" },
    sub2api_token: {
      mark: "S2",
      color: "#A78BFA",
      bg: "rgba(167,139,250,.12)",
    },
    newapi_user: { mark: "NA", color: "#38BDF8", bg: "rgba(56,189,248,.12)" },
    newapi_token: { mark: "NA", color: "#38BDF8", bg: "rgba(56,189,248,.12)" },
    manual_subscription: {
      mark: "Sub",
      color: "#FBBF24",
      bg: "rgba(251,191,36,.12)",
    },
    generic_http: { mark: "API", color: "#60A5FA", bg: "rgba(96,165,250,.12)" },
  };
  return (
    map[kind] ?? {
      mark: name.slice(0, 2).toUpperCase(),
      color: "#38BDF8",
      bg: "rgba(56,189,248,.12)",
    }
  );
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value));
}
