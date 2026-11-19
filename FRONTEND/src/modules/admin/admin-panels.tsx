"use client";
import { useState, type FormEvent } from "react";
import { errorMessage } from "@/shared/api/http-client";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { ErrorState, Loading } from "@/shared/ui/primitives";
import { RelationSelect } from "../resources/relation-select";
const flags = {
  ai: "OpenAI",
  workflows: "Workflow nhiều bước",
  public_api: "API công khai",
};

function useAdminAction() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  async function run(action: () => Promise<unknown>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      setNotice("Thao tác đã hoàn tất.");
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return { busy, error, notice, run };
}
function ActionFeedback({ error, notice }: { error?: string; notice: string }) {
  return (
    <>
      {error && <ErrorState message={error} />}
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
    </>
  );
}

export function AccessPanel() {
  const { admin } = useServices();
  const settings = useQuery(admin.settings);
  const { busy, error, notice, run } = useAdminAction();
  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const fields = new FormData(event.currentTarget);
    const string = (key: string) => String(fields.get(key) || "").trim();
    void run(async () => {
      await admin.saveSettings({
        display_name: string("display_name"),
        logo_url: string("logo_url"),
        accent: string("accent"),
        host: string("host"),
        allowlist: string("allowlist")
          .split(/[\n,]+/)
          .map((value) => value.trim())
          .filter(Boolean),
        flags: Object.fromEntries(
          Object.keys(flags).map((flag) => [flag, fields.has(flag)]),
        ),
      });
      settings.reload();
      window.dispatchEvent(new Event("nexa-brand-refresh"));
    });
  }

  return (
    <section className="panel team-form form-stack">
      <h2>Truy cập và thương hiệu</h2>
      <ActionFeedback error={error || settings.error} notice={notice} />
      {settings.loading && <Loading />}
      {settings.data && (
        <form
          className="form-stack"
          onSubmit={save}
          key={JSON.stringify(settings.data)}
        >
          <label>
            Tên hiển thị
            <input
              name="display_name"
              defaultValue={settings.data.display_name}
              maxLength={80}
            />
          </label>
          <label>
            URL logo HTTPS
            <input
              type="url"
              name="logo_url"
              defaultValue={settings.data.logo_url}
            />
          </label>
          <label>
            Màu thương hiệu (#RRGGBB)
            <input
              pattern="#[0-9a-fA-F]{6}"
              name="accent"
              defaultValue={settings.data.accent}
            />
          </label>
          <label>
            Tên miền đã trỏ về ứng dụng
            <input
              name="host"
              defaultValue={settings.data.host}
              placeholder="support.example.com"
            />
          </label>
          <label>
            Mạng được phép (CIDR, mỗi dòng một mục)
            <textarea
              name="allowlist"
              defaultValue={settings.data.allowlist?.join("\n")}
              placeholder="203.0.113.0/24"
            />
          </label>
          <fieldset>
            <legend>Tính năng</legend>
            {Object.entries(flags).map(([key, name]) => (
              <label key={key}>
                <input
                  type="checkbox"
                  name={key}
                  defaultChecked={settings.data?.flags?.[key] !== false}
                />{" "}
                {name}
              </label>
            ))}
          </fieldset>
          <button className="button primary" disabled={busy}>
            Lưu cài đặt
          </button>
        </form>
      )}
    </section>
  );
}
export function BackupPanel() {
  const [customer, setCustomer] = useState("");
  const exportQuery = customer.trim()
    ? "?" + new URLSearchParams({ customer_id: customer.trim() })
    : "";
  return (
    <section className="panel team-form form-stack">
      <h2>Xuất và sao lưu dữ liệu</h2>
      <label>
        Mã khách hàng (để trống nếu xuất dữ liệu doanh nghiệp)
        <input
          value={customer}
          onChange={(event) => setCustomer(event.target.value)}
        />
      </label>
      <div className="toolbar-controls">
        <a
          className="button secondary"
          href={`/api/platform/admin/export${exportQuery}`}
          download
        >
          Xuất JSON
        </a>
        <a
          className="button secondary"
          href="/api/platform/admin/backup"
          download
        >
          Sao lưu mã hóa
        </a>
      </div>
    </section>
  );
}
export function BillingPanel() {
  const { admin } = useServices();
  const usage = useQuery(admin.usage);
  const billing = useQuery(admin.billing);
  const { busy, error, notice, run } = useAdminAction();
  async function billingLink(plan: string) {
    const result = await admin.billingLink(plan);
    window.location.assign(result.url);
  }
  return (
    <section className="panel team-form form-stack">
      <h2>Mức sử dụng và gói dịch vụ</h2>
      <ActionFeedback
        error={error || usage.error || billing.error}
        notice={notice}
      />
      {(usage.loading || billing.loading) && <Loading />}
      {usage.data && (
        <p>
          Tháng {usage.data.month} · Gói {usage.data.plan.toUpperCase()} ·{" "}
          {usage.data.messages} tin nhắn · {usage.data.tickets} phiếu ·{" "}
          {usage.data.requests} yêu cầu đã xác thực
        </p>
      )}
      <div className="toolbar-controls">
        {(["pro", "enterprise"] as const).map((plan) => (
          <button
            key={plan}
            className="button secondary"
            disabled={
              busy || !billing.data?.[plan] || usage.data?.plan !== "free"
            }
            onClick={() => void run(() => billingLink(plan))}
          >
            {plan === "pro" ? "Đăng ký Pro" : "Đăng ký Enterprise"}
          </button>
        ))}
        <button
          className="button secondary"
          disabled={busy}
          onClick={() => void run(() => billingLink("portal"))}
        >
          Cổng thanh toán Stripe
        </button>
      </div>
    </section>
  );
}
export function CRMPanel() {
  const { admin } = useServices();
  const crm = useQuery(admin.crm);
  const [crmCustomer, setCRMCustomer] = useState("");
  const { busy, error, notice, run } = useAdminAction();
  return (
    <section className="panel team-form form-stack">
      <h2>Đồng bộ CRM</h2>
      <ActionFeedback error={error || crm.error} notice={notice} />
      {crm.loading && <Loading />}
      <form
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          const fields = new FormData(event.currentTarget);
          void run(() =>
            admin.syncContact(
              crmCustomer,
              String(fields.get("provider") ?? ""),
            ),
          );
        }}
      >
        <label>
          CRM
          <select name="provider" required>
            <option value="">Chọn nhà cung cấp</option>
            {["hubspot", "salesforce", "odoo"].map((key) => (
              <option key={key} value={key} disabled={!crm.data?.[key]}>
                {key}
                {!crm.data?.[key] ? " · chưa cấu hình" : ""}
              </option>
            ))}
          </select>
        </label>
        <label htmlFor="crm-customer">Khách hàng</label>
        <RelationSelect
          id="crm-customer"
          kind="customers"
          value={crmCustomer}
          onChange={setCRMCustomer}
          required
          disabled={busy}
        />
        <button className="button primary" disabled={busy || !crmCustomer}>
          Đồng bộ liên hệ
        </button>
      </form>
    </section>
  );
}
export function AuditPanel() {
  const { admin } = useServices();
  const audit = useQuery(admin.audit);
  return (
    <section className="panel team-form form-stack">
      <h2>Nhật ký thay đổi</h2>
      {audit.error && <ErrorState message={audit.error} retry={audit.reload} />}
      {audit.loading && <Loading />}
      {audit.data?.items.map((item) => (
        <details key={item.id}>
          <summary>
            {new Date(item.created_at).toLocaleString("vi-VN")} · {item.status}{" "}
            · {item.name}
          </summary>
          <p>
            Người thực hiện: {item.author_id} · Mục: {item.conversation_id}
          </p>
          <pre className="admin-diff">
            {JSON.stringify(JSON.parse(item.content), null, 2)}
          </pre>
        </details>
      ))}
    </section>
  );
}
