"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import { Badge } from "@/shared/ui/primitives";
import { webhookEvents, type Webhook } from "@/shared/api/platform";

export function Webhooks() {
  const { platform } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => platform.webhooks(signal),
    [platform],
  );
  const hooks = useQuery(load, "workspace-refresh");
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState<string[]>([...webhookEvents]);
  const [created, setCreated] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  function toggle(event: string) {
    setEvents((current) =>
      current.includes(event)
        ? current.filter((item) => item !== event)
        : [...current, event],
    );
  }

  async function create() {
    setBusy(true);
    setError("");
    try {
      const result = await platform.createWebhook({ name, url, events });
      setCreated(result.secret ?? "");
      setName("");
      setUrl("");
      hooks.reload();
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
          Tên webhook
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label>
          URL nhận
          <input
            value={url}
            placeholder="https://example.com/hook"
            onChange={(e) => setUrl(e.target.value)}
          />
        </label>
        <fieldset>
          <legend>Sự kiện</legend>
          {webhookEvents.map((event) => (
            <label key={event}>
              <input
                type="checkbox"
                checked={events.includes(event)}
                onChange={() => toggle(event)}
              />
              <code>{event}</code>
            </label>
          ))}
        </fieldset>
        <button className="button primary" disabled={busy} onClick={create}>
          Tạo webhook
        </button>
        {created && (
          <p role="status">
            Bí mật ký (bảo quản an toàn): <code>{created}</code>
          </p>
        )}
        {(error || hooks.error) && <p role="alert">{error || hooks.error}</p>}
      </div>
      <div className="workspace-results">
        {(hooks.data ?? []).map((hook: Webhook) => (
          <div key={hook.id} className="integration-item">
            <strong>{hook.name}</strong>
            <span>{hook.url}</span>
            <div className="toolbar-controls">
              {hook.events.map((event) => (
                <Badge key={event} value={event} />
              ))}
              <button
                className="text-link"
                onClick={() => {
                  void platform.removeWebhook(hook.id).then(() => {
                    hooks.reload();
                  });
                }}
              >
                Xóa
              </button>
            </div>
          </div>
        ))}
        {!hooks.data?.length && <p className="muted">Chưa có webhook nào.</p>}
      </div>
    </div>
  );
}
