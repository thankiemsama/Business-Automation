"use client";
import { useState, type FormEvent } from "react";
import { useUser } from "@/shared/providers/workspace-provider";
import { useServices } from "@/shared/providers/services-provider";
import { errorMessage } from "@/shared/api/http-client";
import {
  Avatar,
  EmptyState,
  ErrorState,
  PageHeading,
} from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
import type { User } from "@/shared/domain/models";
export function TeamPage() {
  const user = useUser();
  const { auth } = useServices();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState<User | null>(null);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    setBusy(true);
    setError("");
    setCreated(null);
    try {
      const agent = await auth.addAgent({
        name: String(values.get("name")),
        email: String(values.get("email")),
        password: String(values.get("password")),
      });
      setCreated(agent);
      form.reset();
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  if (user.role !== "owner")
    return (
      <EmptyState
        icon="shield"
        title="Khu vực quản trị viên"
        description="Chỉ chủ doanh nghiệp được tạo tài khoản nhân viên."
      />
    );
  return (
    <>
      <PageHeading
        eyebrow="CÙNG NHAU LÀM TỐT HƠN"
        title="Đội ngũ"
        description="Thêm những người đồng hành vào không gian chăm sóc khách hàng."
      />
      <div className="team-grid">
        <section className="panel team-form">
          <div className="panel-header">
            <div>
              <h2>Tạo tài khoản nhân viên</h2>
              <p>Nhân viên sẽ đăng nhập bằng thông tin được cấp.</p>
            </div>
            <Icon name="users" />
          </div>
          <form className="form-stack" onSubmit={submit}>
            <label>
              Họ và tên
              <input
                name="name"
                required
                maxLength={200}
                placeholder="Tên nhân viên"
                autoComplete="name"
              />
            </label>
            <label>
              Email
              <input
                name="email"
                type="email"
                required
                maxLength={254}
                placeholder="nhanvien@congty.vn"
                autoComplete="email"
              />
            </label>
            <label>
              Mật khẩu ban đầu
              <input
                name="password"
                type="password"
                required
                minLength={12}
                maxLength={128}
                placeholder="Từ 12 đến 128 ký tự"
                autoComplete="new-password"
              />
            </label>
            {error && <ErrorState message={error} />}
            {created && (
              <div className="notice" role="status">
                <Icon name="check" size={18} />
                Đã tạo tài khoản cho {created.name} ({created.email}).
              </div>
            )}
            <button className="button primary" disabled={busy}>
              <Icon name="plus" size={17} />
              {busy ? "Đang tạo…" : "Tạo nhân viên"}
            </button>
          </form>
        </section>
        <aside className="panel team-permissions">
          <span className="permission-symbol">
            <Icon name="shield" size={27} />
          </span>
          <h2>Đúng người. Đúng quyền.</h2>
          <p>Nhân viên trong cùng doanh nghiệp có thể:</p>
          <ul>
            {[
              "Quản lý thông tin khách hàng",
              "Tiếp nhận và phản hồi hội thoại",
              "Tạo và xử lý phiếu hỗ trợ",
              "Xem tổng quan, kiến thức và quy tắc",
            ].map((item) => (
              <li key={item}>
                <Icon name="check" size={17} />
                {item}
              </li>
            ))}
          </ul>
          <div className="subtle-note">
            Kho kiến thức, quy tắc tự động và tài khoản nhân viên được quản lý
            bởi owner.
          </div>
          <div className="owner-card">
            <Avatar name={user.name} />
            <div>
              <strong>{user.name}</strong>
              <span>Chủ doanh nghiệp · Bạn</span>
            </div>
          </div>
        </aside>
      </div>
    </>
  );
}
