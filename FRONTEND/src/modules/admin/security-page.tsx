"use client";
import { FeatureSections } from "@/shared/ui/feature-sections";
import { useState, type FormEvent } from "react";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";
import { ErrorState, PageHeading } from "@/shared/ui/primitives";

const api = new FetchHttpClient();
type Active = {
  id: string;
  created_at: string;
  expires_at: string;
  current: boolean;
};
const loadMFA = (signal: AbortSignal) =>
  api.request<{ enabled: boolean }>("account/mfa", { signal });
const loadSessions = (signal: AbortSignal) =>
  api.request<Active[]>("account/sessions", { signal });

export function SecurityPage() {
  const mfa = useQuery(loadMFA),
    sessions = useQuery(loadSessions);
  const [secret, setSecret] = useState("");
  const [recovery, setRecovery] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const fields = new FormData(form);
    void run(async () => {
      const result = await api.request<{
        enabled: boolean;
        secret?: string;
        recovery_codes?: string[];
      }>("account/mfa", {
        method: "POST",
        body: JSON.stringify({
          action: secret ? "enable" : mfa.data?.enabled ? "disable" : "setup",
          password: String(fields.get("password") || ""),
          code: String(fields.get("code") || ""),
        }),
      });
      setSecret(result.secret || "");
      setRecovery(result.recovery_codes || []);
      form.reset();
      mfa.reload();
      sessions.reload();
    });
  }
  return (
    <>
      <PageHeading
        eyebrow="TÀI KHOẢN"
        title="Bảo mật và phiên đăng nhập"
        description="Thiết lập xác thực hai lớp và thu hồi phiên đang hoạt động."
      />
      {(error || mfa.error || sessions.error) && (
        <ErrorState message={error || mfa.error || sessions.error || ""} />
      )}
      <FeatureSections
        label="Cài đặt bảo mật"
        sections={[
          {
            id: "mfa",
            label: "Xác thực hai lớp",
            content: (
              <section className="panel team-form form-stack">
                <h2>Xác thực hai lớp (TOTP)</h2>
                {recovery.length > 0 && (
                  <div className="subtle-note">
                    <strong>Lưu mã khôi phục dùng một lần</strong>
                    <ul>
                      {recovery.map((code) => (
                        <li key={code}>
                          <code>{code}</code>
                        </li>
                      ))}
                    </ul>
                    <button
                      className="text-link"
                      onClick={() => setRecovery([])}
                    >
                      Đã lưu
                    </button>
                  </div>
                )}
                {mfa.data && (
                  <form onSubmit={submit} className="form-stack">
                    {secret ? (
                      <p>
                        Thêm khóa vào ứng dụng xác thực (6 số, 30 giây):{" "}
                        <code>{secret}</code>
                      </p>
                    ) : (
                      <label>
                        Mật khẩu hiện tại
                        <input
                          name="password"
                          type="password"
                          autoComplete="current-password"
                          required
                        />
                      </label>
                    )}
                    {(secret || mfa.data.enabled) && (
                      <label>
                        Mã từ ứng dụng hoặc mã khôi phục
                        <input
                          name="code"
                          autoComplete="one-time-code"
                          maxLength={20}
                          required
                        />
                      </label>
                    )}
                    <button className="button primary" disabled={busy}>
                      {secret
                        ? "Xác nhận bật TOTP"
                        : mfa.data.enabled
                          ? "Tắt TOTP"
                          : "Tạo khóa TOTP"}
                    </button>
                    {secret && (
                      <button
                        type="button"
                        className="button secondary"
                        onClick={() => setSecret("")}
                      >
                        Hủy thiết lập
                      </button>
                    )}
                  </form>
                )}
              </section>
            ),
          },
          {
            id: "sessions",
            label: "Phiên đăng nhập",
            content: (
              <section className="panel team-form form-stack">
                <h2>Phiên đang hoạt động</h2>
                {sessions.data?.map((session) => (
                  <div className="integration-item" key={session.id}>
                    <strong>
                      {session.current ? "Phiên hiện tại" : "Phiên khác"}
                    </strong>
                    <span>
                      Hết hạn{" "}
                      {new Date(session.expires_at).toLocaleString("vi-VN")}
                    </span>
                    {!session.current && (
                      <button
                        className="text-link"
                        disabled={busy}
                        onClick={() =>
                          void run(async () => {
                            await api.request("account/sessions/revoke", {
                              method: "POST",
                              body: JSON.stringify({ id: session.id }),
                            });
                            sessions.reload();
                          })
                        }
                      >
                        Thu hồi
                      </button>
                    )}
                  </div>
                ))}
              </section>
            ),
          },
        ]}
      />
    </>
  );
}
