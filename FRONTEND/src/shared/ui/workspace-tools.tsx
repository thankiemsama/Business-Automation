"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { useServices } from "../providers/services-provider";
import { useQuery } from "../hooks/use-query";
import { errorMessage } from "../api/http-client";
import { Modal } from "./primitives";
import { Icon } from "./icon";

type Theme = "system" | "light" | "dark";
function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
}

function ThemeControl() {
  const [theme, setTheme] = useState<Theme>("system");
  const restore = useCallback((node: HTMLSelectElement | null) => {
    if (!node) return;
    const saved = localStorage.getItem("nexa-theme");
    if (saved === "light" || saved === "dark") {
      applyTheme(saved);
      setTheme(saved);
    }
  }, []);
  return (
    <select
      ref={restore}
      aria-label="Giao diện sáng tối"
      value={theme}
      onChange={(event) => {
        const choice = event.target.value as Theme;
        setTheme(choice);
        applyTheme(choice);
        localStorage.setItem("nexa-theme", choice);
      }}
    >
      <option value="system">Theo hệ thống</option>
      <option value="light">Sáng</option>
      <option value="dark">Tối</option>
    </select>
  );
}

export function WorkspaceTools({
  navigation,
}: {
  navigation: { title: string; href: string }[];
}) {
  const [palette, setPalette] = useState(false);
  useEffect(() => {
    const source = new EventSource("/api/platform/events");
    const refresh = () => window.dispatchEvent(new Event("workspace-refresh"));
    source.addEventListener("refresh", refresh);
    const shortcut = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPalette((open) => !open);
      }
    };
    window.addEventListener("keydown", shortcut);
    return () => {
      source.close();
      window.removeEventListener("keydown", shortcut);
    };
  }, []);
  return (
    <>
      <button
        className="button secondary compact"
        onClick={() => setPalette(true)}
        aria-label="Tìm nhanh, Ctrl hoặc Cmd K"
      >
        <Icon name="search" size={16} />{" "}
        <span className="palette-shortcut">Tìm nhanh</span>
      </button>
      <ThemeControl />
      <Notifications />
      {palette && (
        <CommandPalette
          navigation={navigation}
          close={() => setPalette(false)}
        />
      )}
    </>
  );
}

function CommandPalette({
  navigation,
  close,
}: {
  navigation: { title: string; href: string }[];
  close: () => void;
}) {
  const { workspace } = useServices();
  const router = useRouter();
  const [q, setQ] = useState("");
  const load = useCallback(
    async (signal: AbortSignal) => {
      if (!q.trim()) return [];
      await new Promise((resolve) => setTimeout(resolve, 150));
      if (signal.aborted) return [];
      return workspace.search(q, signal);
    },
    [q, workspace],
  );
  const matches = useQuery(load);
  const pages = navigation.filter((item) =>
    (item.title + " " + item.href).toLowerCase().includes(q.toLowerCase()),
  );
  return (
    <Modal title="Tìm nhanh" onClose={close}>
      <div className="workspace-popover-content">
        <input
          autoFocus
          aria-label="Tìm theo tên, mã hoặc đường dẫn"
          value={q}
          maxLength={50}
          onChange={(event) => setQ(event.target.value)}
          onKeyDown={(event) => {
            if (
              event.key !== "Enter" ||
              event.nativeEvent.isComposing ||
              matches.loading
            )
              return;
            const first = matches.data?.[0];
            const href = first
              ? `/${first.kind}/${encodeURIComponent(first.id)}`
              : pages[0]?.href;
            if (href) {
              event.preventDefault();
              close();
              router.push(href);
            }
          }}
          placeholder="Tìm phiếu, khách hàng, hội thoại hoặc trang…"
        />
        <nav className="workspace-results" aria-label="Kết quả tìm nhanh">
          {matches.data?.map((item) => (
            <Link
              key={item.id}
              href={`/${item.kind}/${encodeURIComponent(item.id)}`}
              onClick={close}
            >
              {item.title} <small>#{item.id.slice(0, 8)}</small>
            </Link>
          ))}
          {pages.map((item) => (
            <Link key={item.href} href={item.href} onClick={close}>
              {item.title}
            </Link>
          ))}
          {matches.error && <p role="alert">{matches.error}</p>}
        </nav>
      </div>
    </Modal>
  );
}

function Notifications() {
  const { workspace } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => workspace.notifications(signal),
    [workspace],
  );
  const notices = useQuery(load, "workspace-refresh");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [pushEnabled, setPushEnabled] = useState<boolean | null>(null);
  const [details, setDetails] = useState<HTMLDetailsElement | null>(null);
  useEffect(() => {
    if (!details) return;
    const outside = (event: PointerEvent) => {
      if (event.target instanceof Node && !details.contains(event.target))
        details.open = false;
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") details.open = false;
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("keydown", escape);
    };
  }, [details]);
  async function changePush() {
    setBusy(true);
    setError("");
    try {
      if (
        !("serviceWorker" in navigator) ||
        !("PushManager" in window) ||
        !("Notification" in window)
      )
        throw new Error("Trình duyệt này chưa hỗ trợ Web Push.");
      const registration =
        await navigator.serviceWorker.register("/nexa-push.js");
      await navigator.serviceWorker.ready;
      const current = await registration.pushManager.getSubscription();
      if (current && pushEnabled !== false) {
        await workspace.push(current.toJSON(), true);
        await current.unsubscribe();
        setPushEnabled(false);
        return;
      }
      const { key } = await workspace.pushKey();
      if (!key) throw new Error("Web Push chưa được cấu hình.");
      if ((await Notification.requestPermission()) !== "granted")
        throw new Error("Cần cho phép thông báo trong trình duyệt.");
      const bytes = Uint8Array.from(
        atob(key.replace(/-/g, "+").replace(/_/g, "/")),
        (character) => character.charCodeAt(0),
      );
      const subscription =
        current ??
        (await registration.pushManager.subscribe({
          userVisibleOnly: true,
          applicationServerKey: bytes,
        }));
      try {
        await workspace.push(subscription.toJSON(), false);
      } catch (error) {
        if (!current) await subscription.unsubscribe();
        throw error;
      }
      setPushEnabled(true);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  async function read(id = "") {
    try {
      await workspace.read(id);
      notices.reload();
    } catch (error) {
      setError(errorMessage(error));
    }
  }
  return (
    <details
      ref={setDetails}
      className="notification-menu"
      onToggle={(event) => {
        if (event.currentTarget.open && "serviceWorker" in navigator)
          void navigator.serviceWorker
            .getRegistration()
            .then((registration) =>
              registration?.pushManager?.getSubscription(),
            )
            .then((sub) => setPushEnabled(Boolean(sub)))
            .catch(() => {});
      }}
    >
      <summary
        className="button secondary compact"
        aria-label={`Thông báo, ${notices.data?.unread ?? 0} chưa đọc`}
      >
        Thông báo
        {Boolean(notices.data?.unread) && (
          <span className="count-pill">{notices.data?.unread}</span>
        )}
      </summary>
      <div className="panel notification-dropdown workspace-popover-content">
        <div className="notification-actions">
          <button
            className="text-link"
            disabled={!notices.data?.unread}
            onClick={() => void read()}
          >
            Đọc tất cả
          </button>
          <button
            className="text-link"
            disabled={busy}
            onClick={() => void changePush()}
          >
            {pushEnabled ? "Tắt Web Push" : "Bật Web Push"}
          </button>
        </div>
        {(error || notices.error) && (
          <p role="alert">{error || notices.error}</p>
        )}
        {!notices.loading && !notices.data?.items.length && !notices.error && (
          <p>Chưa có thông báo.</p>
        )}
        <div className="workspace-results">
          {notices.data?.items.map((notice) => (
            <Link
              key={notice.id}
              className={notice.read_at ? "" : "notification-unread"}
              href={`/${notice.name}/${encodeURIComponent(notice.conversation_id)}`}
              onClick={() => {
                if (details) details.open = false;
                if (!notice.read_at) void read(notice.id);
              }}
            >
              <strong>{notice.subject}</strong>
              <br />
              {notice.content}
              <br />
              <small>
                {new Date(notice.created_at).toLocaleString("vi-VN")}
              </small>
            </Link>
          ))}
        </div>
      </div>
    </details>
  );
}
