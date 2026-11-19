"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";

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
      <div className="form-stack">
        <label>
          Tên key
          <input value={name} onChange={(e) => setName(e.target.value)} />
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
        <button className="button primary" disabled={busy} onClick={create}>
          Tạo API key
        </button>
        {created && (
          <p role="status">
            Key (hiển thị một lần): <code>nexa_{created}</code>
          </p>
        )}
        {(error || keys.error) && <p role="alert">{error || keys.error}</p>}
      </div>
      <div className="workspace-results">
        {(keys.data ?? []).map((key) => (
          <div key={key.id} className="integration-item">
            <strong>{key.name}</strong>
            <span>
              <code>{key.prefix}</code> · {key.scopes.join(", ")} ·{" "}
              {key.rate_per_min} req/phút
            </span>
            <button
              className="text-link"
              onClick={() => {
                void platform.revokeApiKey(key.id).then(() => keys.reload());
              }}
            >
              Thu hồi
            </button>
          </div>
        ))}
        {!keys.data?.length && <p className="muted">Chưa có API key nào.</p>}
      </div>
    </div>
  );
}
