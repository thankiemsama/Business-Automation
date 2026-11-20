"use client";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { PlatformTools, TenantBrand } from "./platform-tools";
import { usePathname } from "next/navigation";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import type { User } from "../domain/models";
import { errorMessage } from "../api/http-client";
import { WorkspaceTools } from "./workspace-tools";
import { Avatar, Brand } from "./primitives";
import { Icon } from "./icon";
import { navigation } from "./workspace-navigation";
import styles from "./app-sidebar.module.css";

const sidebarPreference = "nexa-sidebar-collapsed";

export function AppShell({
  user,
  onLogout,
  children,
}: {
  user: User;
  onLogout: () => Promise<void>;
  children: ReactNode;
}) {
  const path = usePathname();
  const translate = useTranslations("Shell");
  const navTitle = (href: string, fallback: string) =>
    translate.has(href.slice(1) || "home")
      ? translate(href.slice(1) || "home")
      : fallback;
  const [menu, setMenu] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const mobileToggle = useRef<HTMLButtonElement>(null);
  const mobileClose = useRef<HTMLButtonElement>(null);
  const restoreSidebar = useCallback((node: HTMLElement | null) => {
    if (!node) return;
    try {
      setCollapsed(localStorage.getItem(sidebarPreference) === "true");
    } catch {
      // The current layout still works when browser storage is unavailable.
    }
  }, []);
  const closeMenu = useCallback(() => {
    setMenu(false);
    mobileToggle.current?.focus();
  }, []);
  useEffect(() => {
    if (!menu) return;
    mobileClose.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeMenu();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, [closeMenu, menu]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const visibleNavigation = navigation.filter(
    (item) =>
      user.role === "owner" ||
      !["/workflows", "/admin", "/integrations"].includes(item.href),
  );
  const active = visibleNavigation.find(
    (item) =>
      item.href === path ||
      (item.href !== "/" && path.startsWith(item.href + "/")),
  );
  return (
    <div
      className={`workspace ${styles.workspace}`}
      data-sidebar-collapsed={collapsed}
    >
      <a className="skip-link" href="#workspace-content">
        {translate("skip")}
      </a>
      {menu && (
        <button
          className="sidebar-backdrop"
          onClick={closeMenu}
          aria-label="Đóng menu"
        />
      )}
      <aside
        ref={restoreSidebar}
        id="workspace-navigation"
        className={`sidebar ${menu ? "is-open" : ""}`}
      >
        <div className={styles.header}>
          <Link
            href="/"
            className="brand-link"
            aria-label="Nexa — Tổng quan"
            onClick={() => setMenu(false)}
          >
            <TenantBrand fallback={<Brand />} />
          </Link>
          <button
            className={`icon-button ${styles.desktopToggle}`}
            type="button"
            aria-label={collapsed ? "Mở rộng sidebar" : "Thu gọn sidebar"}
            title={collapsed ? "Mở rộng sidebar" : "Thu gọn sidebar"}
            aria-expanded={!collapsed}
            aria-controls="workspace-navigation"
            onClick={() => {
              const next = !collapsed;
              setCollapsed(next);
              try {
                localStorage.setItem(sidebarPreference, String(next));
              } catch {
                // The current layout can still be changed.
              }
            }}
          >
            <Icon
              name={collapsed ? "sidebarExpand" : "sidebarCollapse"}
              size={19}
            />
          </button>
          <button
            ref={mobileClose}
            className={`icon-button ${styles.mobileClose}`}
            type="button"
            aria-label="Đóng menu"
            onClick={closeMenu}
          >
            <Icon name="close" size={19} />
          </button>
        </div>
        <nav aria-label="Điều hướng không gian làm việc">
          {visibleNavigation.map((item, index, items) => {
            const title =
              item.href === "/team" && user.role === "agent"
                ? "Tài khoản"
                : navTitle(item.href, item.title);
            return (
              <div key={item.href} className={styles.entry}>
                {(index === 0 || items[index - 1].group !== item.group) && (
                  <p className="nav-group">{item.group}</p>
                )}
                <Link
                  href={item.href}
                  aria-label={title}
                  title={collapsed ? title : undefined}
                  aria-current={active?.href === item.href ? "page" : undefined}
                  onClick={() => setMenu(false)}
                  className={`nav-link ${active?.href === item.href ? "active" : ""}`}
                >
                  <Icon name={item.icon} size={19} />
                  <span className={styles.label}>{title}</span>
                </Link>
              </div>
            );
          })}
        </nav>
        <div className="sidebar-bottom">
          <div
            className="sidebar-user"
            title={collapsed ? user.name : undefined}
          >
            <Avatar name={user.name} small />
            <div className={styles.userDetails}>
              <strong>{user.name}</strong>
              <span>
                {user.role === "owner" ? "Quản trị viên" : "Nhân viên"}
              </span>
            </div>
            <button
              disabled={busy}
              title="Đăng xuất"
              aria-label="Đăng xuất"
              className="icon-button"
              onClick={async () => {
                setBusy(true);
                try {
                  await onLogout();
                } catch (error) {
                  setError(errorMessage(error));
                } finally {
                  setBusy(false);
                }
              }}
            >
              <Icon name="logout" size={18} />
            </button>
          </div>
          {error && (
            <p className="inline-error" role="alert">
              {error}
            </p>
          )}
        </div>
      </aside>
      <div className="workspace-main">
        <header className="topbar">
          <div className="breadcrumb">
            <button
              ref={mobileToggle}
              className="icon-button mobile-menu"
              onClick={() => (menu ? closeMenu() : setMenu(true))}
              aria-label={menu ? "Đóng menu" : "Mở menu"}
              aria-expanded={menu}
              aria-controls="workspace-navigation"
            >
              <Icon name="menu" />
            </button>
            <span>Không gian làm việc</span>
            <Icon name="chevron" size={13} />
            <strong>
              {active ? navTitle(active.href, active.title) : "Nexa"}
            </strong>
          </div>
          <div className="topbar-right">
            <PlatformTools user={user.id} />
            <WorkspaceTools navigation={visibleNavigation} />
          </div>
        </header>
        <main id="workspace-content" tabIndex={-1} className="page-content">
          {children}
        </main>
      </div>
    </div>
  );
}
