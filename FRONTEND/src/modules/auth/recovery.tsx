"use client";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { useRef, useState, type FormEvent, type ReactNode } from "react";
import { ApiError, errorMessage, FetchHttpClient } from "@/shared/api/http-client";
import { AuthRepository } from "@/shared/api/repositories";
import { Brand, ErrorState } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
import { Turnstile, type TurnstileHandle } from "@/shared/ui/turnstile";
import { LanguageSelect, TenantBrand } from "@/shared/ui/platform-tools";

// The guest auth endpoints are Turnstile-gated only when the deployment
// publishes a site key; the widget and the "captcha" field are skipped
// otherwise, so this file works with or without CAPTCHA.
const TURNSTILE_SITE_KEY =
  process.env.NEXT_PUBLIC_NEXA_TURNSTILE_SITE_KEY ?? "";
import styles from "./recovery.module.css";

const auth = new AuthRepository(new FetchHttpClient());

function RecoveryShell({ children }: { children: ReactNode }) {
  return (
    <main className="auth-entry">
      <section className="auth-entry-card">
        <header className="auth-entry-header">
          <TenantBrand guest fallback={<Brand />} />
          <LanguageSelect />
        </header>
        {children}
      </section>
    </main>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// /account/forgot — request a reset link by email
// ─────────────────────────────────────────────────────────────────────────────
export function ForgotPasswordView() {
  const t = useTranslations("Auth");
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [sent, setSent] = useState(false);
  const turnstileRef = useRef<TurnstileHandle>(null);

  async function send(target?: string) {
    const value = (target ?? email).trim();
    setBusy(true);
    setError("");
    try {
      await auth.account("auth/forgot", {
        email: value,
        ...(TURNSTILE_SITE_KEY && turnstileRef.current
          ? { captcha: turnstileRef.current.getToken() }
          : {}),
      });
      if (target) setEmail(target);
      setSent(true);
    } catch (error) {
      // 503 means the deployment has no email provider configured yet.
      setError(
        error instanceof ApiError && error.status === 503
          ? t("recoveryUnavailable")
          : errorMessage(error),
      );
    } finally {
      setBusy(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await send();
  }

  return (
    <RecoveryShell>
      <h1>{t("recoveryTitle")}</h1>
      <p className={styles.subtitle}>{t("recoveryDesc")}</p>
      {sent ? (
        <div className={styles.result}>
          <span className={styles["result-icon"]}>
            <Icon name="check" size={24} />
          </span>
          <div>
            <h2>{t("recoverySentTitle")}</h2>
            <p>
              {t("recoverySentDesc")} <strong>{email}</strong>
            </p>
          </div>
          <div className={styles["result-actions"]}>
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => void send()}
            >
              {busy ? t("recoveryBusy") : t("recoveryResend")}
              <Icon name="refresh" size={16} />
            </button>
            <Link className="text-link" href="/">
              {t("backToLogin")}
            </Link>
          </div>
        </div>
      ) : (
        <form className="form-stack" onSubmit={submit}>
          <label>
            {t("email")}
            <input
              type="email"
              name="email"
              required
              minLength={3}
              maxLength={254}
              autoComplete="email"
              placeholder="ban@congty.vn"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          </label>
          {error && <ErrorState message={error} />}
          {TURNSTILE_SITE_KEY && (
            <div>
              <Turnstile ref={turnstileRef} siteKey={TURNSTILE_SITE_KEY} />
            </div>
          )}
          <button className="button primary full" disabled={busy}>
            {busy ? t("recoveryBusy") : t("recoverySubmit")}
            <Icon name="arrow" size={18} />
          </button>
        </form>
      )}
      <Link className="text-link" href="/">
        {t("backToLogin")}
      </Link>
    </RecoveryShell>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// /account/verify — confirm the mailbox after registration (G1)
// ─────────────────────────────────────────────────────────────────────────────
type VerifyPhase = "idle" | "submitting" | "success" | "no-token" | "expired";

export function VerifyEmailView() {
  const t = useTranslations("Auth");
  const [phase, setPhase] = useState<VerifyPhase>("idle");
  const [error, setError] = useState("");

  async function submit() {
    const token = decodeURIComponent(window.location.hash.slice(1));
    // The verification token arrives in the URL fragment. Without it the
    // confirmation cannot succeed — point back to the sign-in page.
    if (token.length < 16) {
      setPhase("no-token");
      return;
    }
    setPhase("submitting");
    setError("");
    try {
      await auth.account("auth/confirm", { token });
      // The link is single-use: strip it so a refresh cannot replay it.
      history.replaceState(null, "", location.pathname);
      setPhase("success");
    } catch (err) {
      setError(errorMessage(err));
      setPhase("expired");
    }
  }

  if (phase === "no-token")
    return (
      <RecoveryShell>
        <div className={styles.result}>
          <span className={styles["result-icon"]}>
            <Icon name="info" size={24} />
          </span>
          <div>
            <h1>{t("verifyNoTokenTitle")}</h1>
            <p className={styles.subtitle}>{t("verifyNoTokenDesc")}</p>
          </div>
          <div className={styles["result-actions"]}>
            <Link className="button primary" href="/">
              {t("signInNow")}
              <Icon name="arrow" size={16} />
            </Link>
          </div>
        </div>
      </RecoveryShell>
    );

  if (phase === "success")
    return (
      <RecoveryShell>
        <div className={styles.result}>
          <span className={styles["result-icon"]}>
            <Icon name="check" size={24} />
          </span>
          <div>
            <h1>{t("verifySuccessTitle")}</h1>
            <p className={styles.subtitle}>{t("verifySuccessDesc")}</p>
          </div>
          <div className={styles["result-actions"]}>
            <Link className="button primary" href="/">
              {t("signInNow")}
              <Icon name="arrow" size={16} />
            </Link>
          </div>
        </div>
      </RecoveryShell>
    );

  return (
    <RecoveryShell>
      <h1>{t("verifyConfirmTitle")}</h1>
      <p className={styles.subtitle}>{t("verifyConfirmDesc")}</p>
      <form
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        {error && <ErrorState message={error} />}
        <button
          className="button primary full"
          disabled={phase === "submitting"}
        >
          {phase === "submitting"
            ? t("verifyBusy")
            : error
              ? t("verifyResendLink")
              : t("verifySubmit")}
          <Icon name="arrow" size={18} />
        </button>
      </form>
      <Link className="text-link" href="/">
        {t("backToLogin")}
      </Link>
    </RecoveryShell>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// /account/setup — set a new password from a reset link
// ─────────────────────────────────────────────────────────────────────────────
type SetupPhase = "idle" | "submitting" | "success" | "no-token";

function strengthLevel(length: number, password: string): 0 | 1 | 2 | 3 | 4 {
  if (length === 0) return 0;
  if (length < 12) return 1;
  if (length < 16) return 2;
  const rich =
    /[A-Z]/.test(password) && /[0-9]/.test(password) && /[^A-Za-z0-9]/.test(password);
  return rich ? 4 : 3;
}

export function SetupPasswordView() {
  const t = useTranslations("Auth");
  const [phase, setPhase] = useState<SetupPhase>("idle");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [show, setShow] = useState(false);

  const level = strengthLevel(password.length, password);
  const mismatch =
    password.length >= 12 && confirm.length > 0 && password !== confirm;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (password !== confirm) return;
    // The reset token arrives in the URL fragment (#tenant.token). Without it
    // the API call cannot succeed — send the user back to the request page.
    const token = decodeURIComponent(window.location.hash.slice(1));
    if (token.length < 16) {
      setPhase("no-token");
      return;
    }
    setPhase("submitting");
    setError("");
    try {
      await auth.account("auth/redeem", { token, password });
      // The link is single-use: strip it so a refresh can never replay it.
      history.replaceState(null, "", location.pathname);
      setPhase("success");
    } catch (error) {
      setError(errorMessage(error));
      setPhase("idle");
    }
  }

  if (phase === "no-token")
    return (
      <RecoveryShell>
        <div className={styles.result}>
          <span className={styles["result-icon"]}>
            <Icon name="info" size={24} />
          </span>
          <div>
            <h1>{t("setupNoTokenTitle")}</h1>
            <p className={styles.subtitle}>{t("setupNoTokenDesc")}</p>
          </div>
          <div className={styles["result-actions"]}>
            <Link className="button primary" href="/account/forgot">
              {t("requestReset")}
              <Icon name="arrow" size={16} />
            </Link>
            <Link className="text-link" href="/">
              {t("backToLogin")}
            </Link>
          </div>
        </div>
      </RecoveryShell>
    );

  if (phase === "success")
    return (
      <RecoveryShell>
        <div className={styles.result}>
          <span className={styles["result-icon"]}>
            <Icon name="check" size={24} />
          </span>
          <div>
            <h1>{t("setupSuccessTitle")}</h1>
            <p className={styles.subtitle}>{t("setupSuccessDesc")}</p>
          </div>
          <div className={styles["result-actions"]}>
            <Link className="button primary" href="/">
              {t("signInNow")}
              <Icon name="arrow" size={16} />
            </Link>
          </div>
        </div>
      </RecoveryShell>
    );

  return (
    <RecoveryShell>
      <h1>{t("setupTitle")}</h1>
      <p className={styles.subtitle}>{t("setupDesc")}</p>
      <form className="form-stack" onSubmit={submit}>
        <label>
          {t("newPassword")}
          <span className={styles["password-field"]}>
            <input
              type={show ? "text" : "password"}
              name="password"
              required
              minLength={12}
              maxLength={128}
              autoComplete="new-password"
              placeholder={t("passwordHint")}
              value={password}
              onChange={(event) => setPassword(event.target.value)}
            />
            <button
              type="button"
              className={styles["reveal-button"]}
              aria-label={show ? t("hidePassword") : t("showPassword")}
              onClick={() => setShow((value) => !value)}
            >
              <Icon name={show ? "eye-off" : "eye"} size={17} />
            </button>
          </span>
          {password.length > 0 && (
            <span
              className={styles.strength}
              data-level={level}
              role="meter"
              aria-valuemin={0}
              aria-valuemax={4}
              aria-valuenow={level}
            >
              <span />
              <span />
              <span />
              <span />
            </span>
          )}
        </label>
        <label>
          {t("confirmPassword")}
          <input
            type={show ? "text" : "password"}
            name="confirm_password"
            required
            minLength={12}
            maxLength={128}
            autoComplete="new-password"
            value={confirm}
            onChange={(event) => setConfirm(event.target.value)}
          />
          {mismatch && (
            <span className={styles["field-hint"]} role="alert">
              {t("passwordsMismatch")}
            </span>
          )}
        </label>
        {error && <ErrorState message={error} />}
        <button
          className="button primary full"
          disabled={
            phase === "submitting" ||
            password.length < 12 ||
            password !== confirm ||
            mismatch
          }
        >
          {phase === "submitting" ? t("setupBusy") : t("setupSubmit")}
          <Icon name="arrow" size={18} />
        </button>
      </form>
      <Link className="text-link" href="/">
        {t("backToLogin")}
      </Link>
    </RecoveryShell>
  );
}
