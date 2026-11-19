"use client";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { PlatformTools, TenantBrand } from "./platform-tools";
import { usePathname } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import type { User } from "../domain/models";
import { errorMessage } from "../api/http-client";
import { WorkspaceTools } from "./workspace-tools";
import { Avatar, Brand } from "./primitives";
import { Icon } from "./icon";
import { navigation } from "./workspace-navigation";
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
  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenu(false);
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, []);
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
    <div className="workspace">
      <a className="skip-link" href="#workspace-content">
        {translate("skip")}
      </a>
      {menu && (
        <button
          className="sidebar-backdrop"
          onClick={() => setMenu(false)}
          aria-label="Đóng menu"
        />
      )}
      <aside
        id="workspace-navigation"
        className={`sidebar ${menu ? "is-open" : ""}`}
      >
        <Link href="/" className="brand-link" aria-label="Nexa — Tổng quan">
          <TenantBrand fallback={<Brand />} />
        </Link>
        <nav aria-label="Điều hướng không gian làm việc">
          {visibleNavigation.map((item, index, items) => (
            <div key={item.href}>
              {(index === 0 || items[index - 1].group !== item.group) && (
                <p className="nav-group">{item.group}</p>
              )}
              <Link
                href={item.href}
                aria-current={active?.href === item.href ? "page" : undefined}
                onClick={() => setMenu(false)}
                className={`nav-link ${active?.href === item.href ? "active" : ""}`}
              >
                <Icon name={item.icon} size={19} />
                <span>
                  {item.href === "/team" && user.role === "agent"
                    ? "Tài khoản"
                    : navTitle(item.href, item.title)}
                </span>
                {active?.href === item.href && (
                  <span className="nav-active-dot" />
                )}
              </Link>
            </div>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="sidebar-user">
            <Avatar name={user.name} small />
            <div>
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
              className="icon-button mobile-menu"
              onClick={() => setMenu(true)}
              aria-label="Mở menu"
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
