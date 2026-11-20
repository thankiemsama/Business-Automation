"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import { ErrorState, Loading } from "@/shared/ui/primitives";

export function ApiKeys() {
  const { platform } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => platform.apiKeys(signal),
    [platform],
  );
  const keys = useQuery(load, "workspace-refresh");
  const [name, setName] = useState("");
  const [scope, setScope] = useState("read");
  const [rate, setRate] = useState(60);
  const [created, setCreated] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function create() {
    setBusy(true);
    setError("");
    setCreated("");
    try {
      const result = await platform.createApiKey({
        name,
        scope,
        rate_per_min: rate,
      });
      setCreated(result.secret ?? "");
      setName("");
      keys.reload();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="integrations-grid">
      <form
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          void create();
        }}
      >
        <h3>Tạo khóa API</h3>
        <label>
          Tên key
          <input
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        <div className="field-row">
          <label>
            Phạm vi
            <select value={scope} onChange={(e) => setScope(e.target.value)}>
              <option value="read">Đọc (read)</option>
            </select>
          </label>
          <label>
            Tốc độ (req/phút)
            <input
              type="number"
              min={1}
              max={600}
              value={rate}
              onChange={(e) => setRate(Number(e.target.value))}
            />
          </label>
        </div>
        <button className="button primary" disabled={busy || !name.trim()}>
          Tạo khóa
        </button>
        {created && (
          <p className="integration-secret" role="status">
            Lưu khóa ngay. Khóa chỉ hiển thị một lần.{" "}
            <code>nexa_{created}</code>
          </p>
        )}
        {error && <p role="alert">{error}</p>}
      </form>
      <section className="workspace-results">
        <h3>Khóa đã cấp</h3>
        {keys.error ? (
          <ErrorState message={keys.error} retry={keys.reload} />
        ) : keys.loading ? (
          <Loading />
        ) : null}
        {(keys.data ?? []).map((key) => (
          <div key={key.id} className="integration-item">
            <strong>{key.name}</strong>
            <span>
              <code>{key.prefix}</code> · {key.scopes.join(", ")} ·{" "}
              {key.rate_per_min} req/phút
            </span>
            <button
              type="button"
              className="button secondary compact"
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  await platform.revokeApiKey(key.id);
                  keys.reload();
                } catch (err) {
                  setError(errorMessage(err));
                } finally {
                  setBusy(false);
                }
              }}
            >
              Thu hồi
            </button>
          </div>
        ))}
        {keys.data?.length === 0 && <p className="muted">Chưa có khóa API.</p>}
      </section>
    </div>
  );
}
