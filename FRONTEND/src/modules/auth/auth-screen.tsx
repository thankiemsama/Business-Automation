"use client";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { LanguageSelect, TenantBrand } from "@/shared/ui/platform-tools";
import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useServices } from "@/shared/providers/services-provider";
import {
  ApiError,
  errorMessage,
  FetchHttpClient,
} from "@/shared/api/http-client";
import type { User } from "@/shared/domain/models";
import { Brand, ErrorState } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
import { Turnstile, type TurnstileHandle } from "@/shared/ui/turnstile";

// Cloudflare Turnstile proof-of-human. Enabled only when a site key is
// published at build time; the backend enforces it via NEXA_TURNSTILE_SECRET.
const TURNSTILE_SITE_KEY =
  process.env.NEXT_PUBLIC_NEXA_TURNSTILE_SITE_KEY ?? "";

export function AuthScreen({ onSuccess }: { onSuccess: (user: User) => void }) {
  const t = useTranslations("Auth");
  const { auth } = useServices();
  const [providers, setProviders] = useState<string[]>([]);
  const [needCode, setNeedCode] = useState(false);
  const [mode, setMode] = useState<"login" | "register">("login");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<{ email: string } | null>(null);
  const [resent, setResent] = useState(false);
  const turnstileRef = useRef<TurnstileHandle>(null);

  useEffect(() => {
    const controller = new AbortController();
    new FetchHttpClient()
      .request<string[]>("oidc/providers", { signal: controller.signal })
      .then(setProviders)
      .catch(() => {});
    return () => controller.abort();
  }, []);

  async function resend() {
    if (!pending) return;
    setBusy(true);
    setResent(false);
    setError("");
    try {
      // The endpoint answers success for any mailbox to stay enumeration-safe;
      // a missing mail provider (503) is the only distinct outcome. When
      // Turnstile is enabled the resend is gated like every guest endpoint.
      await auth.account("auth/verify", {
        email: pending.email,
        ...(TURNSTILE_SITE_KEY && turnstileRef.current
          ? { captcha: turnstileRef.current.getToken() }
          : {}),
      });
      setResent(true);
    } catch (err) {
      setError(
        err instanceof ApiError && err.status === 503
          ? t("recoveryUnavailable")
          : errorMessage(err),
      );
    } finally {
      setBusy(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const fields = new FormData(event.currentTarget);
    const email = String(fields.get("email"));
    const credentials = {
      email,
      password: String(fields.get("password")),
      ...(needCode && mode === "login"
        ? { code: String(fields.get("code")) }
        : {}),
      ...(mode === "register"
        ? {
            name: String(fields.get("name")),
            industry: String(fields.get("industry") ?? ""),
          }
        : {}),
      // Only attach a token while the widget has produced one; when the
      // backend runs without NEXA_TURNSTILE_SECRET this field is ignored.
      ...(TURNSTILE_SITE_KEY && turnstileRef.current
        ? { captcha: turnstileRef.current.getToken() }
        : {}),
    };
    try {
      const user = await auth[mode](credentials);
      if (user.email_verification_pending) {
        setPending({ email: user.email });
        setResent(true);
        return;
      }
      onSuccess(user);
    } catch (err) {
      if (mode === "login" && err instanceof ApiError && err.status === 409)
        setNeedCode(true);
      if (err instanceof ApiError && err.reason === "email_unverified") {
        setPending({ email });
        setResent(true);
        return;
      }
      setError(errorMessage(err));
    } finally {
      setBusy(false);
      turnstileRef.current?.reset();
    }
  }

  if (pending)
    return (
      <main className="auth-entry">
        <section className="auth-entry-card">
          <header className="auth-entry-header">
            <TenantBrand guest fallback={<Brand />} />
            <LanguageSelect />
          </header>
          <h1>{t("verifyTitle")}</h1>
          <p>{t("verifyDesc")}</p>
          <div className="form-stack">
            <p>
              {t("verifyMailbox")} <strong>{pending.email}</strong>
            </p>
            {TURNSTILE_SITE_KEY && (
              <div>
                <Turnstile ref={turnstileRef} siteKey={TURNSTILE_SITE_KEY} />
              </div>
            )}
            {resent && !busy && <p role="status">{t("verifySent")}</p>}
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => void resend()}
            >
              {busy ? t("verifyBusy") : t("verifyResend")}
              <Icon name="refresh" size={16} />
            </button>
          </div>
          {error && <ErrorState message={error} />}
          <Link className="text-link" href="/">
            {t("backToLogin")}
          </Link>
        </section>
      </main>
    );

  return (
    <main className="auth-entry">
      <section className="auth-entry-card">
        <header className="auth-entry-header">
          <TenantBrand guest fallback={<Brand />} />
          <LanguageSelect />
        </header>
        <h1>{t(mode === "login" ? "welcome" : "start")}</h1>
        {mode === "register" && (
          <p>Tạo tài khoản quản trị và không gian doanh nghiệp mới.</p>
        )}
        <form onSubmit={submit} className="form-stack">
          {mode === "register" && (
            <label>
              Họ và tên
              <input
                name="name"
                required
                autoComplete="name"
                placeholder="Nguyễn Minh Anh"
                maxLength={200}
              />
            </label>
          )}
          <label>
            {t("email")}
            <input
              type="email"
              name="email"
              required
              autoComplete="email"
              placeholder="ban@congty.vn"
              maxLength={254}
            />
          </label>
          {mode === "register" && (
            <label>
              {t("industry")}
              <select name="industry" defaultValue="">
                <option value="">{t("industryBlank")}</option>
                <option value="ecommerce">{t("industryEcommerce")}</option>
                <option value="food_beverage">{t("industryFood")}</option>
                <option value="saas">{t("industrySaas")}</option>
                <option value="education">{t("industryEducation")}</option>
                <option value="logistics">{t("industryLogistics")}</option>
              </select>
            </label>
          )}
          <label>
            {t("password")}
            <input
              type="password"
              name="password"
              required
              minLength={mode === "register" ? 12 : 1}
              maxLength={128}
              autoComplete={
                mode === "login" ? "current-password" : "new-password"
              }
              placeholder={
                mode === "register"
                  ? "Ít nhất 12 ký tự"
                  : "Nhập mật khẩu của bạn"
              }
            />
          </label>
          {mode === "login" && needCode && (
            <label>
              Mã xác thực hoặc mã khôi phục
              <input
                name="code"
                required
                autoComplete="one-time-code"
                maxLength={20}
              />
            </label>
          )}
          {TURNSTILE_SITE_KEY ? (
            <div>
              <Turnstile ref={turnstileRef} siteKey={TURNSTILE_SITE_KEY} />
            </div>
          ) : null}
          {error && <ErrorState message={error} />}
          <button className="button primary full" disabled={busy}>
            {busy
              ? "Đang xử lý…"
              : mode === "login"
                ? t("login")
                : "Tạo không gian làm việc"}
            <Icon name="arrow" size={18} />
          </button>
        </form>
        {mode === "login" && (
          <Link className="text-link" href="/account/forgot">
            {t("forgotLink")}
          </Link>
        )}
        {mode === "login" && providers.length > 0 && (
          <div className="form-stack">
            {providers.map((provider) => (
              <a
                key={provider}
                className="button secondary"
                href={"/api/oidc/" + provider + "/start"}
              >
                SSO · {provider}
              </a>
            ))}
          </div>
        )}
        <div className="auth-switch">
          {mode === "login"
            ? "Chưa có không gian làm việc?"
            : "Đã có tài khoản?"}{" "}
          <button
            disabled={busy}
            onClick={() => {
              setMode(mode === "login" ? "register" : "login");
              setError("");
              setResent(false);
            }}
          >
            {mode === "login" ? "Tạo tài khoản" : "Đăng nhập"}
          </button>
        </div>
      </section>
    </main>
  );
}
