"use client";

import { useCallback, useEffect, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import { Badge, Loading } from "@/shared/ui/primitives";
import { publicChannels } from "@/shared/api/platform";

const stateLabels = {
  pending: "Đang chờ gửi",
  sending: "Đang gửi",
  sent: "Nhà cung cấp đã tiếp nhận",
  failed: "DLQ · Cần xử lý",
};

const failureLabels: Record<string, string> = {
  rate_limited: "Nhà cung cấp giới hạn tần suất",
  delivery_unknown: "Chưa xác định nhà cung cấp đã tiếp nhận hay chưa",
  provider_rejected: "Nhà cung cấp từ chối tin nhắn",
  channel_unavailable: "Kênh hoặc tài khoản gửi không khả dụng",
};

const channelName = (channel: string) =>
  channel === "zalo"
    ? "Zalo OA"
    : channel === "whatsapp"
      ? "WhatsApp"
      : channel.toUpperCase();

export function Channels() {
  const { platform } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => platform.channels(signal),
    [platform],
  );
  const channels = useQuery(load, "workspace-refresh");
  const [channel, setChannel] = useState<string>(publicChannels[0]);
  const [settings, setSettings] = useState<{
    channel: string;
    enabled: boolean;
    content: string;
  }>();
  const [recipient, setRecipient] = useState("");
  const [testText, setTestText] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const live =
    channel === "zalo" || channel === "whatsapp" || channel === "sms";
  const selected =
    settings?.channel === channel ? settings : channels.data?.[channel];
  const enabled = selected?.enabled ?? false;
  const content = selected?.content ?? "";
  const loadDeliveries = useCallback(
    (signal: AbortSignal) =>
      live ? platform.channelDeliveries(channel, signal) : Promise.resolve([]),
    [channel, live, platform],
  );
  const {
    data: deliveries,
    error: deliveryError,
    reload: reloadDeliveries,
  } = useQuery(loadDeliveries);
  useEffect(() => {
    if (
      !live ||
      !deliveries?.some(
        (job) => job.status === "pending" || job.status === "sending",
      )
    )
      return;
    const timer = window.setTimeout(reloadDeliveries, 4000);
    return () => window.clearTimeout(timer);
  }, [deliveries, live, reloadDeliveries]);

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="integrations-grid">
      <div className="form-stack">
        <h3>Cấu hình kênh</h3>
        <label>
          Kênh
          <select
            value={channel}
            disabled={busy}
            onChange={(event) => {
              setChannel(event.target.value);
              setSettings(undefined);
              setError("");
              setRecipient("");
            }}
          >
            {publicChannels.map((item) => (
              <option key={item} value={item}>
                {channelName(item)}
              </option>
            ))}
          </select>
        </label>
        <label>
          Thông tin cấu hình (không nhập khóa bí mật)
          <textarea
            value={content}
            onChange={(event) =>
              setSettings({ channel, enabled, content: event.target.value })
            }
            placeholder={
              channel === "zalo"
                ? '{"account_id":"OA ID","app_id":"App ID"}'
                : channel === "sms"
                  ? '{"account_id":"+12025550123"}'
                  : '{"account_id":"Phone Number ID"}'
            }
          />
        </label>
        <label className="check-row">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(event) =>
              setSettings({ channel, content, enabled: event.target.checked })
            }
          />
          {live ? "Bật kênh" : "Đánh dấu đã chuẩn bị cấu hình"}
        </label>
        <button
          className="button primary"
          disabled={busy || !channels.data}
          onClick={() =>
            void run(async () => {
              await platform.saveChannel(channel, {
                name: channel,
                content,
                enabled,
              });
              channels.reload();
            })
          }
        >
          Lưu cấu hình
        </button>
        {live && (
          <section className="form-stack integration-subsection">
            <h4>Gửi tin kiểm thử · {channelName(channel)}</h4>
            <label>
              {channel === "zalo"
                ? "UID Zalo nhận tin kiểm thử"
                : "Số điện thoại nhận tin, gồm mã quốc gia"}
              <input
                value={recipient}
                inputMode={channel === "sms" ? "tel" : "numeric"}
                onChange={(event) => setRecipient(event.target.value)}
              />
            </label>
            <label>
              Nội dung tin kiểm thử
              <textarea
                value={testText}
                onChange={(event) => setTestText(event.target.value)}
              />
            </label>
            <button
              className="button secondary"
              disabled={
                busy ||
                channels.data?.[channel]?.status !== "configured" ||
                !recipient.trim() ||
                !testText.trim()
              }
              onClick={() =>
                void run(async () => {
                  await platform.testChannel(channel, recipient, testText);
                  reloadDeliveries();
                })
              }
            >
              Gửi tin kiểm thử
            </button>
          </section>
        )}
        {(error || channels.error || deliveryError) && (
          <p role="alert">{error || channels.error || deliveryError}</p>
        )}
      </div>
      <section className="workspace-results">
        <h3>Trạng thái kết nối</h3>
        {channels.loading && <Loading />}
        {channels.data && (
          <div className="integration-channel-list">
            {publicChannels.map((item) => {
              const status = channels.data?.[item];
              let label = "chưa cấu hình";
              if (status?.enabled) {
                label =
                  status.status === "configured"
                    ? "sẵn sàng kiểm thử"
                    : status.status === "credentials_required"
                      ? "thiếu cấu hình kết nối"
                      : "đã lưu cấu hình, chưa kết nối";
              }
              return (
                <div
                  key={item}
                  className="integration-item"
                  data-current={item === channel ? "" : undefined}
                >
                  <strong>{channelName(item)}</strong>
                  <Badge value={label} />
                </div>
              );
            })}
          </div>
        )}
        {live && (
          <section className="form-stack integration-subsection">
            <div className="integration-actions">
              <h4>Hàng đợi gửi · {channelName(channel)}</h4>
              <button
                className="button secondary compact"
                onClick={reloadDeliveries}
              >
                Làm mới
              </button>
            </div>
            {deliveries?.length === 0 && (
              <p>Chưa có lượt gửi đang chờ, tin kiểm thử hoặc lỗi.</p>
            )}
            {deliveries?.map((job) => (
              <div key={job.id} className="integration-item">
                <Badge value={stateLabels[job.status]} />
                <small>
                  <code>{job.id.slice(0, 12)}</code> ·{" "}
                  {new Date(job.updated_at).toLocaleString("vi-VN")}
                </small>
                {job.email_last_error && (
                  <small>
                    {failureLabels[job.email_last_error] ??
                      job.email_last_error}
                  </small>
                )}
                {job.status === "failed" && (
                  <button
                    className="button secondary compact"
                    disabled={busy}
                    onClick={() => {
                      if (
                        job.email_last_error === "delivery_unknown" &&
                        !window.confirm(
                          "Hãy đối soát trên kênh trước khi gửi lại. Thử lại có thể tạo tin nhắn trùng. Tiếp tục?",
                        )
                      )
                        return;
                      void run(async () => {
                        await platform.retryChannel(channel, job.id);
                        reloadDeliveries();
                      });
                    }}
                  >
                    Thử lại
                  </button>
                )}
              </div>
            ))}
          </section>
        )}
      </section>
    </div>
  );
}
