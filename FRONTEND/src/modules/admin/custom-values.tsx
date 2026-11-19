"use client";
import { useCallback, useState, type FormEvent } from "react";
import { useQuery } from "@/shared/hooks/use-query";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";

const api = new FetchHttpClient();
type Kind = "customers" | "tickets" | "conversations";
type Field = {
  id: string;
  kind: Kind;
  name: string;
  type: "text" | "number" | "checkbox";
  enabled: boolean;
};
export const loadFields = (signal: AbortSignal) =>
  api.request<Field[]>("admin/fields", { signal });
const kinds = {
  customers: "Khách hàng",
  tickets: "Phiếu hỗ trợ",
  conversations: "Hội thoại",
};

export function FieldsAdmin() {
  const fields = useQuery(loadFields);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function save(value: Partial<Field>) {
    setBusy(true);
    setError("");
    try {
      await api.request("admin/fields", {
        method: "POST",
        body: JSON.stringify(value),
      });
      fields.reload();
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    void save({
      name: String(data.get("name")),
      kind: String(data.get("kind")) as Kind,
      type: String(data.get("type")) as Field["type"],
      enabled: true,
    });
  }
  return (
    <section className="panel team-form form-stack">
      <h2>Trường tùy chỉnh</h2>
      {(error || fields.error) && <p role="alert">{error || fields.error}</p>}
      {fields.data?.map((field) => (
        <div key={field.id} className="integration-item">
          <span>
            {field.name} · {kinds[field.kind]}
          </span>
          <button
            className="text-link"
            onClick={() => void save({ ...field, enabled: !field.enabled })}
            disabled={busy}
          >
            {field.enabled ? "Vô hiệu hóa" : "Bật lại"}
          </button>
        </div>
      ))}
      <form className="form-stack" onSubmit={create}>
        <label>
          Tên trường
          <input name="name" maxLength={80} required />
        </label>
        <label>
          Tài nguyên
          <select name="kind">
            {Object.entries(kinds).map(([kind, name]) => (
              <option key={kind} value={kind}>
                {name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Kiểu
          <select name="type">
            <option value="text">Văn bản</option>
            <option value="number">Số</option>
            <option value="checkbox">Có/Không</option>
          </select>
        </label>
        <button className="button primary" disabled={busy}>
          Tạo trường
        </button>
      </form>
    </section>
  );
}
export function CustomValues({ kind, id }: { kind: Kind; id: string }) {
  const fields = useQuery(loadFields);
  const path = `admin/custom/${kind}/${encodeURIComponent(id)}`;
  const load = useCallback(
    (signal: AbortSignal) =>
      api.request<Record<string, string>>(path, { signal }),
    [path],
  );
  const values = useQuery(load);
  const active =
    fields.data?.filter((field) => field.enabled && field.kind === kind) ?? [];
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  if (!active.length) return null;
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setSaved(false);
    setError("");
    const data = new FormData(event.currentTarget);
    const payload = Object.fromEntries(
      active.map((field) => [field.id, String(data.get(field.id) || "")]),
    );
    void api
      .request(path, { method: "PUT", body: JSON.stringify(payload) })
      .then(() => {
        setSaved(true);
        values.reload();
      })
      .catch((error) => setError(errorMessage(error)))
      .finally(() => setBusy(false));
  }
  return (
    <section className="panel team-form form-stack">
      <h2>Thông tin tùy chỉnh</h2>
      {(error || values.error) && <p role="alert">{error || values.error}</p>}
      {saved && <p role="status">Đã lưu.</p>}
      {values.data && (
        <form
          className="form-stack"
          key={JSON.stringify(values.data)}
          onSubmit={submit}
        >
          {active.map((field) => (
            <label key={field.id}>
              {field.name}
              {field.type === "checkbox" ? (
                <select
                  name={field.id}
                  defaultValue={values.data?.[field.id] || ""}
                >
                  <option value="">Chưa nhập</option>
                  <option value="true">Có</option>
                  <option value="false">Không</option>
                </select>
              ) : (
                <input
                  name={field.id}
                  maxLength={1000}
                  defaultValue={values.data?.[field.id] || ""}
                  type={field.type}
                  step={field.type === "number" ? "any" : undefined}
                />
              )}
            </label>
          ))}
          <button className="button secondary" disabled={busy}>
            Lưu thông tin
          </button>
        </form>
      )}
    </section>
  );
}
