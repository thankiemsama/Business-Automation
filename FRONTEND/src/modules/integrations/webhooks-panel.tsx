"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import { Badge, ErrorState, Loading } from "@/shared/ui/primitives";
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
  const [showFailures, setShowFailures] = useState(false);

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
    <>
      <div
        className="toolbar-controls"
        role="group"
        aria-label="Quản lý webhook"
      >
        <button
          type="button"
          className="button secondary compact"
          aria-pressed={!showFailures}
          onClick={() => setShowFailures(false)}
        >
          Cấu hình
        </button>
        <button
          type="button"
          className="button secondary compact"
          aria-pressed={showFailures}
          onClick={() => setShowFailures(true)}
        >
          DLQ / Thử lại
        </button>
      </div>
      {showFailures ? <WebhookFailures /> : null}
      <div hidden={showFailures}>
        <div className="integrations-grid">
          <form
            className="form-stack"
            onSubmit={(event) => {
              event.preventDefault();
              void create();
            }}
          >
            <h3>Tạo webhook</h3>
            <label>
              Tên webhook
              <input
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </label>
            <label>
              URL nhận
              <input
                type="url"
                required
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
            <button
              className="button primary"
              disabled={busy || !name.trim() || !url.trim() || !events.length}
            >
              Tạo webhook
            </button>
            {created && (
              <p className="integration-secret" role="status">
                Lưu khóa ký ở nơi an toàn. <code>{created}</code>
              </p>
            )}
            {error && <p role="alert">{error}</p>}
          </form>
          <section className="workspace-results">
            <h3>Webhook đã cấu hình</h3>
            {hooks.error ? (
              <ErrorState message={hooks.error} retry={hooks.reload} />
            ) : hooks.loading ? (
              <Loading />
            ) : null}
            {(hooks.data ?? []).map((hook: Webhook) => (
              <div key={hook.id} className="integration-item">
                <strong>{hook.name}</strong>
                <span>{hook.url}</span>
                <div className="toolbar-controls">
                  {hook.events.map((event) => (
                    <Badge key={event} value={event} />
                  ))}
                  <button
                    type="button"
                    className="button secondary compact"
                    disabled={busy}
                    onClick={async () => {
                      setBusy(true);
                      setError("");
                      try {
                        await platform.removeWebhook(hook.id);
                        hooks.reload();
                      } catch (err) {
                        setError(errorMessage(err));
                      } finally {
                        setBusy(false);
                      }
                    }}
                  >
                    Xóa
                  </button>
                </div>
              </div>
            ))}
            {hooks.data?.length === 0 && (
              <p className="muted">Chưa có webhook nào.</p>
            )}
          </section>
        </div>
      </div>
    </>
  );
}

function WebhookFailures() {
  const { platform } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => platform.webhookDeadLetters(signal),
    [platform],
  );
  const failures = useQuery(load, "workspace-refresh");
  const [pending, setPending] = useState("");
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");

  async function replay(id: string) {
    setError("");
    setStatus("");
    setPending(id);
    try {
      await platform.replayWebhook(id);
      setStatus("Đã đưa lượt gửi trở lại hàng đợi.");
      failures.reload();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setPending("");
    }
  }

  return (
    <section className="workspace-results">
      <h3>Webhook gửi thất bại</h3>
      <p className="muted">
        Có thể gửi lại trong vòng 30 ngày kể từ khi lượt gửi thất bại.
      </p>
      {failures.error && (
        <ErrorState message={failures.error} retry={failures.reload} />
      )}
      {failures.loading && <Loading />}
      {status && <p role="status">{status}</p>}
      {error && <p role="alert">{error}</p>}
      {!failures.loading && failures.data?.length === 0 && (
        <p className="muted">Chưa có lượt gửi thất bại.</p>
      )}
      {(failures.data ?? []).map((item) => (
        <div className="integration-item" key={item.id}>
          <strong>{item.webhook_name || "Webhook"}</strong>
          <span>
            {item.event} · {item.attempts} lần thử ·{" "}
            {new Date(item.failed_at).toLocaleString("vi-VN")}
          </span>
          {item.replayable ? (
            <button
              type="button"
              className="button secondary compact"
              disabled={pending !== ""}
              onClick={() => void replay(item.id)}
            >
              {pending === item.id ? "Đang xếp hàng…" : "Thử lại"}
            </button>
          ) : (
            <span className="muted">Webhook đã bị tắt.</span>
          )}
        </div>
      ))}
    </section>
  );
}
