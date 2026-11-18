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
import { Icon, type IconName } from "./icon";
const navigation: {
  href: string;
  title: string;
  icon: IconName;
  group: string;
}[] = [
  { href: "/", title: "Tổng quan", icon: "grid", group: "KHÔNG GIAN LÀM VIỆC" },
  {
    href: "/conversations",
    title: "Hộp hội thoại",
    icon: "chat",
    group: "KHÔNG GIAN LÀM VIỆC",
  },
  {
    href: "/customers",
    title: "Khách hàng",
    icon: "users",
    group: "KHÔNG GIAN LÀM VIỆC",
  },
  {
    href: "/tickets",
    title: "Phiếu hỗ trợ",
    icon: "ticket",
    group: "KHÔNG GIAN LÀM VIỆC",
  },
  {
    href: "/knowledge",
    title: "Kho kiến thức",
    icon: "book",
    group: "VẬN HÀNH",
  },
  {
    href: "/automations",
    title: "Quy tắc từ khóa",
    icon: "bolt",
    group: "VẬN HÀNH",
  },
  {
    href: "/integrations",
    title: "Tích hợp",
    icon: "globe",
    group: "VẬN HÀNH",
  },
  {
    href: "/workflows",
    title: "Workflow và lịch gửi",
    icon: "bolt",
    group: "VẬN HÀNH",
  },
  { href: "/admin", title: "Quản trị", icon: "shield", group: "VẬN HÀNH" },
  { href: "/security", title: "Bảo mật", icon: "shield", group: "VẬN HÀNH" },
  { href: "/team", title: "Đội ngũ", icon: "shield", group: "VẬN HÀNH" },
];
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
  const active = navigation.find(
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
        <div className="workspace-switch">
          <span className="workspace-logo">N</span>
          <div>
            <strong>Không gian Nexa</strong>
            <span>
              {user.role === "owner"
                ? "Quản trị doanh nghiệp"
                : "Chuyên viên hỗ trợ"}
            </span>
          </div>
          <span className="workspace-online" />
        </div>
        <nav>
          {navigation
            .filter(
              (item) =>
                !["/workflows", "/admin"].includes(item.href) ||
                user.role === "owner",
            )
            .map((item, index, items) => (
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
          <div className="sidebar-tip">
            <span className="tip-icon">
              <Icon name="bolt" size={18} />
            </span>
            <strong>Để công việc tự vận hành</strong>
            <p>Từ khóa đúng. Hỗ trợ kịp thời.</p>
            <Link
              href={user.role === "owner" ? "/workflows" : "/automations"}
              onClick={() => setMenu(false)}
            >
              Khám phá tự động hóa <Icon name="arrow" size={15} />
            </Link>
          </div>
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
            <WorkspaceTools navigation={navigation} />
            <span className="private-label">
              <span className="status-dot" /> Không gian riêng tư
            </span>
            <Avatar name={user.name} small />
          </div>
        </header>
        <main id="workspace-content" tabIndex={-1} className="page-content">
          {children}
        </main>
        <footer className="workspace-footer">
          <span>
            NEXA <span>/</span> CUSTOMER EXPERIENCE
          </span>
          <span>Cùng bạn tạo nên trải nghiệm tốt hơn.</span>
        </footer>
      </div>
    </div>
  );
}
