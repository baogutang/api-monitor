import type {
  Capability,
  HealthStatus,
  Money,
  ProviderKind,
  Quota,
} from "./types";

export function formatMoney(m?: Money): string {
  if (!m) return "—";
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency: m.currency || "USD",
    maximumFractionDigits: 2,
  }).format(m.amount);
}

export function formatQuota(q?: Quota, locale?: string): string {
  if (!q) return "—";
  const unit = quotaUnitLabel(q.unit, locale);
  if (q.remaining != null && q.total != null) {
    return `${formatNumber(q.remaining)} / ${formatNumber(q.total)} ${unit}`;
  }
  if (q.remaining != null) return `${formatNumber(q.remaining)} ${unit}`;
  return unit || "—";
}

export function formatNumber(n: number): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(
    n,
  );
}

export function quotaPercent(q?: Quota): number | null {
  if (!q?.remaining || !q?.total || q.total <= 0) return null;
  return Math.round((q.remaining / q.total) * 100);
}

export function formatDate(iso?: string, locale?: string): string {
  if (!iso) return "—";
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(iso));
}

export function formatRelative(iso?: string, locale?: string): string {
  if (!iso) return "—";
  const diff = Date.now() - new Date(iso).getTime();
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  const minutes = Math.round(diff / 60000);
  if (Math.abs(minutes) < 60) return rtf.format(-minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 48) return rtf.format(-hours, "hour");
  return formatDate(iso, locale);
}

export function statusTone(s: HealthStatus): "ok" | "warn" | "crit" | "muted" {
  if (s === "healthy") return "ok";
  if (s === "warning") return "warn";
  if (s === "critical") return "crit";
  return "muted";
}

export function providerLabel(
  kind: ProviderKind,
  t: (k: string) => string,
): string {
  return t(`provider.${kind}`);
}

export function capabilityLabel(
  cap: Capability,
  t: (k: string) => string,
): string {
  return t(`capability.${cap}`);
}

function quotaUnitLabel(unit?: string, locale?: string) {
  if (!unit) return "";
  const lang =
    locale ??
    (typeof document !== "undefined"
      ? document.documentElement.lang || navigator.language
      : "");
  if (unit === "quota" && !String(lang).toLowerCase().startsWith("en")) {
    return "额度";
  }
  return unit;
}
