import { NavLink } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  Bell,
  Cloud,
  LayoutDashboard,
  LogOut,
  PanelLeftClose,
  PanelLeftOpen,
  RefreshCw,
  Search,
  Send,
  Server,
  Settings,
  SlidersHorizontal,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { alertsApi, dashboardApi } from "@/api/services";
import { useAuth } from "@/contexts/AuthContext";
import { LanguageSwitch } from "@/components/LanguageSwitch";
import { BrandLogo } from "@/components/BrandLogo";
import { useEffect, useState, type ReactNode } from "react";

const navItems: Array<{
  to: string;
  icon: typeof LayoutDashboard;
  key: string;
  badge?: boolean;
}> = [
  { to: "/", icon: LayoutDashboard, key: "dashboard" },
  { to: "/instances", icon: Cloud, key: "instances" },
  { to: "/assets", icon: Server, key: "assets" },
  { to: "/alerts", icon: Bell, key: "alerts", badge: true },
  { to: "/rules", icon: SlidersHorizontal, key: "rules" },
  { to: "/notifications", icon: Send, key: "notifications" },
  { to: "/scans", icon: Search, key: "scans" },
  { to: "/settings", icon: Settings, key: "settings" },
];

export function AppShell({
  title,
  description,
  children,
  actions,
  contentClassName,
  onRefresh,
  refreshing,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  actions?: ReactNode;
  contentClassName?: string;
  onRefresh?: () => void | Promise<unknown>;
  refreshing?: boolean;
}) {
  const { t } = useTranslation();
  const { logout } = useAuth();
  const [clock, setClock] = useState(() => formatClock());
  const [refreshSpin, setRefreshSpin] = useState(false);
  const [navExpanded, setNavExpanded] = useState(
    () => localStorage.getItem("api_monitor_sidebar_expanded") === "1",
  );

  const openAlerts = useQuery({
    queryKey: ["alerts-open-count"],
    queryFn: async () => {
      const res = await alertsApi.list({ status: "open", limit: 1 });
      return res.total;
    },
    refetchInterval: 15000,
    retry: false,
  });
  const summary = useQuery({
    queryKey: ["dashboard-summary-shell"],
    queryFn: dashboardApi.summary,
    refetchInterval: 15000,
    retry: false,
  });

  useEffect(() => {
    const timer = window.setInterval(() => setClock(formatClock()), 1000);
    return () => window.clearInterval(timer);
  }, []);

  const handleRefresh = () => {
    setRefreshSpin(true);
    window.setTimeout(() => setRefreshSpin(false), 460);
    if (onRefresh) {
      void onRefresh();
      return;
    }
    window.location.reload();
  };

  const toggleNav = () => {
    setNavExpanded((current) => {
      const next = !current;
      localStorage.setItem("api_monitor_sidebar_expanded", next ? "1" : "0");
      return next;
    });
  };

  const healthPill = shellHealth(summary.data, t);

  return (
    <div className={`app${navExpanded ? " nav-expanded" : ""}`}>
      <nav className="sidebar" aria-label={t("nav.workspace")}>
        <div className="sidebar-head">
          <NavLink
            to="/"
            className="logo-mark"
            title={t("app.name")}
            aria-label={t("app.name")}
          >
            <BrandLogo loadCustom />
          </NavLink>
          <div className="sidebar-brand-text">
            <div>{t("app.name")}</div>
            <span>mission control</span>
          </div>
        </div>

        <button
          className="nav-item nav-toggle"
          type="button"
          title={navExpanded ? "收起菜单" : "展开菜单"}
          aria-label={navExpanded ? "收起菜单" : "展开菜单"}
          onClick={toggleNav}
        >
          {navExpanded ? (
            <PanelLeftClose size={18} />
          ) : (
            <PanelLeftOpen size={18} />
          )}
          <span className="nav-label">
            {navExpanded ? "收起菜单" : "展开菜单"}
          </span>
        </button>

        <div className="nav-items">
          {navItems.map(({ to, icon: Icon, key, badge }, index) => (
            <div key={to}>
              {(index === 3 || index === 6) && <div className="sidebar-sep" />}
              <NavLink
                to={to}
                end={to === "/"}
                className={({ isActive }) =>
                  `nav-item${isActive ? " active" : ""}`
                }
                title={t(`nav.${key}`)}
                aria-label={t(`nav.${key}`)}
              >
                <Icon size={15} />
                <span className="nav-label">{t(`nav.${key}`)}</span>
                {badge && (openAlerts.data ?? 0) > 0 && (
                  <span className="nav-badge">{openAlerts.data}</span>
                )}
              </NavLink>
            </div>
          ))}
        </div>

        <button
          className="nav-item nav-item-button"
          type="button"
          title={t("auth.logout")}
          aria-label={t("auth.logout")}
          onClick={logout}
        >
          <LogOut size={18} />
          <span className="nav-label">{t("auth.logout")}</span>
        </button>
      </nav>

      <div className="main">
        <header className="topbar">
          <div className="topbar-title">
            <h1>{title}</h1>
            <p className="mission-clock">
              {description ? `${description} · ` : ""}
              上次更新 {clock}
            </p>
          </div>
          <div className="topbar-actions">
            {actions}
            <LanguageSwitch />
            <div className={healthPill.className}>
              <span className="dot-live" />
              {healthPill.text}
            </div>
            <button
              className={`btn-refresh${refreshSpin || refreshing ? " is-spinning" : ""}`}
              type="button"
              onClick={handleRefresh}
            >
              <RefreshCw size={13} />
              {t("common.refresh")}
            </button>
          </div>
        </header>
        <main
          className={`content page-enter${contentClassName ? ` ${contentClassName}` : ""}`}
        >
          {children}
        </main>
      </div>
    </div>
  );
}

function formatClock() {
  return new Date().toTimeString().slice(0, 8);
}

function shellHealth(
  summary: Awaited<ReturnType<typeof dashboardApi.summary>> | undefined,
  t: (key: string) => string,
) {
  if (!summary) return { text: t("common.loading"), className: "pill-muted" };
  if (summary.totalTargets === 0)
    return { text: t("common.noAssets"), className: "pill-muted" };
  if (summary.criticalTargets > 0)
    return {
      text: `${summary.criticalTargets} ${t("status.critical")}`,
      className: "pill-crit",
    };
  if (summary.warningTargets > 0)
    return {
      text: `${summary.warningTargets} ${t("status.warning")}`,
      className: "pill-warn",
    };
  if (summary.unknownTargets > 0)
    return {
      text: `${summary.unknownTargets} ${t("status.unknown")}`,
      className: "pill-muted",
    };
  return { text: t("common.allOnline"), className: "pill-ok" };
}
