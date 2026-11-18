"use client";
import Link from "next/link";
import { useState, type FormEvent } from "react";
import { AuthRepository } from "@/shared/api/repositories";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";

const auth = new AuthRepository(new FetchHttpClient());
const actions = {
  "auth/password": ["Đổi mật khẩu", "Đã đổi mật khẩu. Hãy đăng nhập lại."],
  "team/invite": ["Mời qua email", "Đã gửi lời mời."],
  "auth/forgot": [
    "Khôi phục mật khẩu",
    "Nếu email có tài khoản đang hoạt động, bạn sẽ nhận được liên kết khôi phục.",
  ],
  "auth/redeem": [
    "Thiết lập mật khẩu",
    "Đã lưu mật khẩu. Bạn có thể đăng nhập.",
  ],
} as const;

export function AccountForm({ action }: { action: keyof typeof actions }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const [title, confirmation] = actions[action];
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const input = Object.fromEntries(
      Array.from(new FormData(form), ([key, value]) => [key, String(value)]),
    );
    setBusy(true);
    setError("");
    setDone(false);
    try {
      if (action === "auth/redeem")
        input.token = decodeURIComponent(window.location.hash.slice(1));
      await auth.account(action, input);
      if (action === "auth/redeem")
        history.replaceState(null, "", location.pathname);
      if (action === "auth/password") {
        await auth.logout().catch(() => {});
        window.location.reload();
      }
      form.reset();
      setDone(true);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="panel team-form">
      <h2>{title}</h2>
      <form className="form-stack" onSubmit={submit}>
        {action === "team/invite" && (
          <label>
            Họ và tên
            <input name="name" required maxLength={200} />
          </label>
        )}
        {(action === "team/invite" || action === "auth/forgot") && (
          <label>
            Email
            <input name="email" type="email" required maxLength={254} />
          </label>
        )}
        {action === "auth/password" && (
          <label>
            Mật khẩu hiện tại
            <input
              name="old_password"
              type="password"
              required
              maxLength={128}
              autoComplete="current-password"
            />
          </label>
        )}
        {(action === "auth/password" || action === "auth/redeem") && (
          <label>
            Mật khẩu mới
            <input
              name="password"
              type="password"
              required
              minLength={12}
              maxLength={128}
              autoComplete="new-password"
            />
          </label>
        )}
        <button
          className="button primary"
          disabled={busy || (done && action === "auth/redeem")}
        >
          {title}
        </button>
      </form>
      {error && <p role="alert">{error}</p>}
      {done && <p role="status">{confirmation}</p>}
      {(action === "auth/forgot" || action === "auth/redeem") && (
        <Link className="text-link" href="/">
          Về trang đăng nhập
        </Link>
      )}
    </section>
  );
}
