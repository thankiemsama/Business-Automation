"use client";
import { useState, type FormEvent } from "react";
import {
  FetchHttpClient,
  ApiError,
  errorMessage,
} from "@/shared/api/http-client";
import Link from "next/link";

export default function SSOComplete() {
  const [codeRequired, setCodeRequired] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const fields = new FormData(event.currentTarget);
    setBusy(true);
    setError("");
    try {
      const token = location.hash.slice(1);
      await new FetchHttpClient().request("auth/oidc-exchange", {
        method: "POST",
        body: JSON.stringify({ token, code: String(fields.get("code") || "") }),
      });
      history.replaceState(null, "", location.pathname);
      location.replace("/");
    } catch (error) {
      if (error instanceof ApiError && error.status === 409)
        setCodeRequired(true);
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <main className="panel team-form">
      <h1>Đăng nhập SSO</h1>
      <form className="form-stack" onSubmit={submit}>
        {codeRequired && (
          <label>
            Mã xác thực
            <input
              name="code"
              required
              autoComplete="one-time-code"
              maxLength={20}
            />
          </label>
        )}
        {error && <p role="alert">{error}</p>}
        <button className="button primary" disabled={busy}>
          Hoàn tất đăng nhập
        </button>
        <Link href="/">Quay lại</Link>
      </form>
    </main>
  );
}
