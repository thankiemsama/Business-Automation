"use client";
import Link from "next/link";
import { Dialog, DialogPanel, DialogTitle } from "@headlessui/react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "next/navigation";
import {
  useCallback,
  useEffect,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { FetchHttpClient } from "../api/http-client";
import { useQuery } from "../hooks/use-query";
import { Icon } from "./icon";

const api = new FetchHttpClient();
type InstallEvent = Event & {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: string }>;
};
let deferredInstall: InstallEvent | null = null;
const installListeners = new Set<() => void>();
function publishInstall(event: InstallEvent | null) {
  deferredInstall = event;
  installListeners.forEach((listener) => listener());
}
function subscribeInstall(listener: () => void) {
  installListeners.add(listener);
  return () => {
    installListeners.delete(listener);
  };
}
const readInstall = () => deferredInstall;
const serverInstall = () => null;

export function PWAProvider() {
  useEffect(() => {
    if ("serviceWorker" in navigator) {
      void navigator.serviceWorker.register("/nexa-push.js").catch(() => {});
    }
    const intercept = (event: Event) => {
      event.preventDefault();
      publishInstall(event as InstallEvent);
    };
    const done = () => publishInstall(null);
    window.addEventListener("beforeinstallprompt", intercept);
    window.addEventListener("appinstalled", done);
    return () => {
      window.removeEventListener("beforeinstallprompt", intercept);
      window.removeEventListener("appinstalled", done);
    };
  }, []);
  return null;
}
export function LanguageSelect() {
  const locale = useLocale(),
    t = useTranslations("Shell"),
    router = useRouter();
  return (
    <select
      aria-label={t("language")}
      value={locale}
      onChange={(event) => {
        document.cookie = `nexa_locale=${event.target.value === "en" ? "en" : "vi"}; Path=/; Max-Age=31536000; SameSite=Lax${location.protocol === "https:" ? "; Secure" : ""}`;
        router.refresh();
      }}
    >
      <option value="vi">Tiếng Việt</option>
      <option value="en">English</option>
    </select>
  );
}
export function PlatformTools({ user }: { user: string }) {
  const t = useTranslations("Shell"),
    locale = useLocale();
  const [dialog, setDialog] = useState<"shortcuts" | "tour" | null>(null);
  const [step, setStep] = useState(0);
  const install = useSyncExternalStore(
    subscribeInstall,
    readInstall,
    serverInstall,
  );
  const storage = `nexa-tour:${user}`;
  const tour =
    locale === "en"
      ? [
          "Open the inbox to view and reply to customer messages.",
          "Use tickets to assign ownership and track SLA deadlines.",
          "Open Security to enable two-factor authentication.",
        ]
      : [
          "Mở hộp hội thoại để xem và trả lời khách hàng.",
          "Dùng phiếu hỗ trợ để phân công và theo dõi hạn SLA.",
          "Mở Bảo mật để bật xác thực hai lớp.",
        ];
  useEffect(() => {
    const help = (event: KeyboardEvent) => {
      if (
        event.key !== "?" ||
        event.ctrlKey ||
        event.metaKey ||
        event.altKey ||
        event.isComposing
      )
        return;
      if (
        event.target instanceof HTMLElement &&
        event.target.closest("input,textarea,select,[contenteditable=true]")
      )
        return;
      event.preventDefault();
      setDialog("shortcuts");
    };
    window.addEventListener("keydown", help);
    return () => window.removeEventListener("keydown", help);
  }, []);
  const initial = useCallback(
    (node: HTMLButtonElement | null) => {
      if (node && !localStorage.getItem(storage)) {
        setStep(0);
        setDialog("tour");
      }
    },
    [storage],
  );
  const close = () => {
    if (dialog === "tour") localStorage.setItem(storage, "done");
    setDialog(null);
  };
  return (
    <>
      <div className="toolbar-select">
        <Icon name="globe" size={16} />
        <LanguageSelect />
      </div>
      <button
        ref={initial}
        type="button"
        className="button secondary toolbar-control toolbar-icon"
        aria-label={t("shortcuts")}
        title={t("shortcuts")}
        onClick={() => setDialog("shortcuts")}
      >
        <Icon name="help" size={17} />
      </button>
      {install && (
        <button
          className="button secondary toolbar-control"
          onClick={async () => {
            try {
              await install.prompt();
              await install.userChoice;
            } finally {
              publishInstall(null);
            }
          }}
        >
          <Icon name="download" size={16} />
          {t("install")}
        </button>
      )}
      <Dialog
        open={dialog !== null}
        onClose={close}
        className="platform-dialog"
      >
        <DialogPanel className="panel platform-dialog-panel">
          <header className="platform-dialog-title">
            <DialogTitle as="h2">
              {t(dialog === "tour" ? "tour" : "shortcuts")}
            </DialogTitle>
            <button
              type="button"
              className="icon-button"
              aria-label={t("close")}
              onClick={close}
            >
              <Icon name="close" size={18} />
            </button>
          </header>
          {dialog === "tour" ? (
            <>
              <p aria-live="polite">
                {step + 1}/{tour.length} · {tour[step]}
              </p>
              <button
                className="button primary"
                onClick={() =>
                  step + 1 === tour.length ? close() : setStep(step + 1)
                }
              >
                {t(step + 1 === tour.length ? "done" : "next")}
              </button>
            </>
          ) : (
            <>
              <dl className="platform-shortcuts">
                <div>
                  <dt>
                    <kbd>?</kbd>
                  </dt>
                  <dd>{t("shortcuts")}</dd>
                </div>
                <div>
                  <dt>
                    <kbd>Ctrl</kbd>/<kbd>Cmd</kbd> + <kbd>K</kbd>
                  </dt>
                  <dd>{locale === "en" ? "Quick search" : "Tìm nhanh"}</dd>
                </div>
                <div>
                  <dt>
                    <kbd>Esc</kbd>
                  </dt>
                  <dd>{t("close")}</dd>
                </div>
              </dl>
              <footer className="platform-dialog-actions">
                <button
                  className="button secondary"
                  onClick={() => {
                    setStep(0);
                    setDialog("tour");
                  }}
                >
                  <Icon name="info" size={16} /> {t("tour")}
                </button>
                <Link className="text-link" href="/help" onClick={close}>
                  {t("help")} · {t("release")}
                </Link>
              </footer>
            </>
          )}
        </DialogPanel>
      </Dialog>
    </>
  );
}
export function TenantBrand({
  fallback,
  guest = false,
}: {
  fallback: ReactNode;
  guest?: boolean;
}) {
  const load = useCallback(
    (signal: AbortSignal) =>
      api.request<{ name?: string; logo_url?: string; accent?: string }>(
        guest
          ? `branding?${new URLSearchParams({ host: location.hostname })}`
          : "admin/brand",
        { signal },
      ),
    [guest],
  );
  const brand = useQuery(load, "nexa-brand-refresh");
  const accent = brand.data?.accent;
  useEffect(() => {
    if (!accent || !/^#[a-f0-9]{6}$/i.test(accent)) return;
    document.documentElement.style.setProperty("--tenant-accent", accent);
    return () => {
      document.documentElement.style.removeProperty("--tenant-accent");
    };
  }, [accent]);
  if (!brand.data?.name && !brand.data?.logo_url) return fallback;
  return (
    <span className="toolbar-controls">
      {brand.data.logo_url && (
        <span
          className="tenant-logo"
          aria-hidden="true"
          style={{
            backgroundImage: `url(${JSON.stringify(brand.data.logo_url)})`,
          }}
        />
      )}
      <strong>{brand.data.name || "Nexa"}</strong>
    </span>
  );
}
