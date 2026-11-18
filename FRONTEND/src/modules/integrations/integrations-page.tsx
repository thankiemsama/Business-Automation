"use client";
import { useCallback, useState } from "react";
import Link from "next/link";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import {
  publicChannels,
  webhookEvents,
  type Webhook,
} from "@/shared/api/platform";
import { PageHeading, Badge } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";

// Integrations hub: outgoing webhooks, public API keys, multi-channel
// configuration, and the public OpenAPI document. Owner-only.
export function IntegrationsPage() {
  const user = useUser();
  if (user.role !== "owner")
    return (
      <PageHeading
        title="Tích hợp"
        eyebrow="VẬN HÀNH"
        description="Chỉ quản trị viên được cấu hình tích hợp."
      />
    );
  return (
    <>
      <PageHeading
        title="Tích hợp & Kết nối"
        eyebrow="VẬN HÀNH"
        description="Kết nối Nexa với hệ thống của bạn qua webhook, API công khai và đa kênh."
      />
      <section className="panel team-form">
        <h2>
          <Icon name="globe" size={18} /> API công khai
        </h2>
        <p>
          Tạo API key cho bên thứ ba (Zapier, n8n, app riêng). Key có phạm vi
          đọc, giới hạn tốc độ riêng và xác thực bằng HMAC.
        </p>
        <Link className="button secondary" href="/api-docs">
          Xem OpenAPI / Swagger
        </Link>
        <ApiKeys />
      </section>
      <section className="panel team-form">
        <h2>
          <Icon name="bolt" size={18} /> Webhook đi (outgoing)
        </h2>
        <p>
          Nhận sự kiện <code>ticket.created</code>, <code>message.new</code>,{" "}
          <code>sla.breached</code>, <code>csat.submitted</code>. Payload được
          ký bằng HMAC-SHA256 (header <code>X-Nexa-Signature</code>).
        </p>
        <Webhooks />
      </section>
      <section className="panel team-form">
        <h2>
          <Icon name="chat" size={18} /> Đa kênh (multi-channel)
        </h2>
        <p>
          Cấu hình kết nối các kênh hỗ trợ. Mỗi kênh có trình adapter riêng và
          được lưu theo tenant.
        </p>
        <Channels />
      </section>
    </>
  );
}

function Webhooks() {
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

function ApiKeys() {
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

function Channels() {
  const { platform } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => platform.channels(signal),
    [platform],
  );
  const channels = useQuery(load, "workspace-refresh");
  const [channel, setChannel] = useState<string>(publicChannels[0]);
  const [enabled, setEnabled] = useState(false);
  const [content, setContent] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function save() {
    setBusy(true);
    setError("");
    try {
      await platform.saveChannel(channel, { name: channel, content, enabled });
      channels.reload();
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
          Kênh
          <select
            value={channel}
            onChange={(e) => {
              setChannel(e.target.value);
              setContent(
                (channels.data?.[e.target.value] as { content?: string })
                  ?.content ?? "",
              );
            }}
          >
            {publicChannels.map((item) => (
              <option key={item} value={item}>
                {item.toUpperCase()}
              </option>
            ))}
          </select>
        </label>
        <label>
          Thông tin cấu hình (không nhập khóa bí mật)
          <textarea
            value={content}
            onChange={(e) => setContent(e.target.value)}
            placeholder='{"account_id":"..."}'
          />
        </label>
        <label>
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          Đánh dấu đã chuẩn bị cấu hình
        </label>
        <button className="button primary" disabled={busy} onClick={save}>
          Lưu cấu hình
        </button>
        {(error || channels.error) && (
          <p role="alert">{error || channels.error}</p>
        )}
      </div>
      <div className="workspace-results">
        {publicChannels.map((item) => {
          const status = channels.data?.[item];
          return (
            <div key={item} className="integration-item">
              <strong>{item.toUpperCase()}</strong>
              <span>
                {status?.enabled ? (
                  <Badge value="đã lưu cấu hình, chưa kết nối" />
                ) : (
                  <Badge value="chưa cấu hình" />
                )}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
