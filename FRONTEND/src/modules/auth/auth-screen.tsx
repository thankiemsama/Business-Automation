"use client";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { LanguageSelect, TenantBrand } from "@/shared/ui/platform-tools";
import { useEffect, useState, type FormEvent } from "react";
import { useServices } from "@/shared/providers/services-provider";
import {
  ApiError,
  errorMessage,
  FetchHttpClient,
} from "@/shared/api/http-client";
import type { User } from "@/shared/domain/models";
import { Brand, ErrorState } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
export function AuthScreen({ onSuccess }: { onSuccess: (user: User) => void }) {
  const t = useTranslations("Auth");
  const { auth } = useServices();
  const [providers, setProviders] = useState<string[]>([]);
  const [needCode, setNeedCode] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    new FetchHttpClient()
      .request<string[]>("oidc/providers", { signal: controller.signal })
      .then(setProviders)
      .catch(() => {});
    return () => controller.abort();
  }, []);
  const [mode, setMode] = useState<"login" | "register">("login");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const fields = new FormData(event.currentTarget);
    const credentials = {
      email: String(fields.get("email")),
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
    };
    try {
      onSuccess(await auth[mode](credentials));
    } catch (error) {
      if (mode === "login" && error instanceof ApiError && error.status === 409)
        setNeedCode(true);
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
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
            Quên mật khẩu?
          </Link>
        )}
        {mode === "login" && providers.length > 0 && (
          <div className="form-stack">
            {providers.map((provider) => (
              <a
                key={provider}
                className="button secondary"
                href={`/api/oidc/${provider}/start`}
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
            }}
          >
            {mode === "login" ? "Tạo tài khoản" : "Đăng nhập"}
          </button>
        </div>
      </section>
    </main>
  );
}
