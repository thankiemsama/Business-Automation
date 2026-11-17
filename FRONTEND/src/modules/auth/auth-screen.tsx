"use client";
import { useState, type FormEvent } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { errorMessage } from "@/shared/api/http-client";
import type { User } from "@/shared/domain/models";
import { Brand, ErrorState } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
export function AuthScreen({ onSuccess }: { onSuccess: (user: User) => void }) {
  const { auth } = useServices();
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
      ...(mode === "register" ? { name: String(fields.get("name")) } : {}),
    };
    try {
      onSuccess(await auth[mode](credentials));
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="auth-page">
      <section className="auth-story">
        <Brand />
        <div className="auth-story-content">
          <span className="story-label">
            <span className="status-dot" /> CUSTOMER EXPERIENCE PLATFORM
          </span>
          <h1>
            Mỗi kết nối.
            <br />
            Một trải nghiệm
            <br />
            <em>tốt hơn.</em>
          </h1>
          <p>
            Một không gian để lắng nghe khách hàng, hỗ trợ đội ngũ và để những
            việc lặp lại được tự động hóa.
          </p>
          <div className="story-cards">
            <div>
              <Icon name="chat" />
              <span>Hội thoại tập trung</span>
            </div>
            <div>
              <Icon name="bolt" />
              <span>Tự động hóa thông minh</span>
            </div>
            <div>
              <Icon name="users" />
              <span>Khách hàng là trung tâm</span>
            </div>
          </div>
        </div>
        <span className="auth-footer">
          NEXA WORKSPACE{" "}
          <span>Được xây dựng cho những kết nối có ý nghĩa.</span>
        </span>
      </section>
      <section className="auth-form-side">
        <span className="auth-top-note">KHÔNG GIAN LÀM VIỆC CỦA BẠN</span>
        <div className="auth-form-wrap">
          <span className="auth-symbol">
            <Icon name="globe" size={26} />
          </span>
          <h2>
            {mode === "login" ? "Chào mừng trở lại" : "Bắt đầu cùng Nexa"}
          </h2>
          <p>
            {mode === "login"
              ? "Đăng nhập để tiếp tục chăm sóc khách hàng của bạn."
              : "Tạo tài khoản quản trị và không gian doanh nghiệp mới."}
          </p>
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
              Email công việc
              <input
                type="email"
                name="email"
                required
                autoComplete="email"
                placeholder="ban@congty.vn"
                maxLength={254}
              />
            </label>
            <label>
              Mật khẩu
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
            {error && <ErrorState message={error} />}
            <button className="button primary full" disabled={busy}>
              {busy
                ? "Đang xử lý…"
                : mode === "login"
                  ? "Đăng nhập"
                  : "Tạo không gian làm việc"}
              <Icon name="arrow" size={18} />
            </button>
          </form>
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
          <div className="auth-trust">
            <Icon name="shield" size={16} /> Kết nối an toàn. Dữ liệu riêng cho
            từng doanh nghiệp.
          </div>
        </div>
        <span className="auth-bottom-note">
          Ít thao tác hơn. Nhiều kết nối hơn.
        </span>
      </section>
    </div>
  );
}
