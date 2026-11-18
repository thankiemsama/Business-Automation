"use client";
import { useCallback, useState, type FormEvent } from "react";
import { useUser } from "@/shared/providers/workspace-provider";
import { useServices } from "@/shared/providers/services-provider";
import { errorMessage } from "@/shared/api/http-client";
import { Avatar, ErrorState, PageHeading } from "@/shared/ui/primitives";
import { AccountForm } from "../auth/account-form";
import { Icon } from "@/shared/ui/icon";
import { useQuery } from "@/shared/hooks/use-query";
import type { User } from "@/shared/domain/models";
export function TeamPage() {
  const user = useUser();
  const { auth } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => auth.members(signal),
    [auth],
  );
  const members = useQuery(load);
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
      members.reload();
      form.reset();
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  if (user.role !== "owner") return <AccountForm action="auth/password" />;
  return (
    <>
      <PageHeading
        eyebrow="CÙNG NHAU LÀM TỐT HƠN"
        title="Đội ngũ"
        description="Thêm những người đồng hành vào không gian chăm sóc khách hàng."
      />
      <AccountForm action="auth/password" />
      <AccountForm action="team/invite" />
      {error && <ErrorState message={error} />}
      <section className="panel team-form">
        <h2>Thành viên</h2>
        {members.error && (
          <ErrorState message={members.error} retry={members.reload} />
        )}
        {members.data?.map((member) =>
          member.role === "agent" ? (
            <form
              key={member.id + member.name + member.email + member.disabled}
              className="form-stack"
              onSubmit={async (event) => {
                event.preventDefault();
                const values = new FormData(event.currentTarget);
                setBusy(true);
                setError("");
                try {
                  await auth.updateMember(member.id, {
                    name: String(values.get("name")),
                    email: String(values.get("email")),
                    disabled: values.has("disabled"),
                  });
                  members.reload();
                } catch (error) {
                  setError(errorMessage(error));
                } finally {
                  setBusy(false);
                }
              }}
            >
              <label>
                Họ và tên{" "}
                <input
                  name="name"
                  defaultValue={member.name}
                  required
                  maxLength={200}
                />
              </label>
              <label>
                Email{" "}
                <input
                  name="email"
                  type="email"
                  defaultValue={member.email}
                  required
                  maxLength={254}
                />
              </label>
              <label>
                <input
                  name="disabled"
                  type="checkbox"
                  defaultChecked={member.disabled}
                />{" "}
                Vô hiệu hóa
              </label>
              <button className="button secondary" disabled={busy}>
                Lưu thông tin
              </button>
            </form>
          ) : (
            <p key={member.id}>
              {member.name} · {member.email} · Chủ doanh nghiệp
            </p>
          ),
        )}
      </section>
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
