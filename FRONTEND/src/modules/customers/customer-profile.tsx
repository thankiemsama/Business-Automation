"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import { Badge } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";

// Customer 360°: a cross-channel, cross-record view of one customer with a
// full activity timeline, lifetime-value signals, notes, and quick actions.
export function CustomerProfilePanel({ id }: { id: string }) {
  const { platform } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => platform.profile(id, signal),
    [platform, id],
  );
  const profile = useQuery(load, "workspace-refresh");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function addNote() {
    if (!note.trim()) return;
    setBusy(true);
    setError("");
    try {
      await platform.addNote(id, note);
      setNote("");
      profile.reload();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  if (profile.error)
    return (
      <p role="alert">
        Không tải được hồ sơ khách hàng: {errorMessage(profile.error)}
      </p>
    );
  const data = profile.data;
  if (!data) return <p>Đang tải hồ sơ 360°…</p>;

  return (
    <div className="customer-360">
      <div className="customer-360-grid">
        <section className="panel c360-metric">
          <h3>Chỉ số nhanh</h3>
          <dl className="c360-stats">
            <div>
              <dt>Phiếu hỗ trợ</dt>
              <dd>{data.total_tickets}</dd>
            </div>
            <div>
              <dt>Đang mở</dt>
              <dd>{data.open_tickets}</dd>
            </div>
            <div>
              <dt>Đánh giá TB</dt>
              <dd>
                {data.average_rating ? data.average_rating.toFixed(1) : "—"}
              </dd>
            </div>
            <div>
              <dt>Hội thoại</dt>
              <dd>{data.conversations}</dd>
            </div>
          </dl>
        </section>

        <section className="panel c360-metric">
          <h3>Lịch sử hỗ trợ</h3>
          <p>
            Đã xử lý <strong>{data.ltv.resolved}</strong>/{data.ltv.tickets}{" "}
            phiếu · Hoạt động cuối cùng:{" "}
            <strong>
              {data.ltv.last_active
                ? new Date(data.ltv.last_active).toLocaleDateString("vi-VN")
                : "chưa rõ"}
            </strong>
          </p>
          <div className="toolbar-controls">
            <span>Kênh:</span>
            {data.channels.map((channel) => (
              <Badge key={channel} value={channel} />
            ))}
          </div>
          {data.tags?.length ? (
            <div className="keywords">
              {data.tags.map((tag) => (
                <span key={tag}>{tag}</span>
              ))}
            </div>
          ) : (
            <p className="muted">Chưa có nhãn nào.</p>
          )}
        </section>
      </div>

      <section className="panel">
        <h3>Ghi chú nội bộ</h3>
        <div className="c360-notes">
          {data.notes?.map((item) => (
            <div key={item.id} className="c360-note">
              <p>{item.content}</p>
              <small>{new Date(item.created_at).toLocaleString("vi-VN")}</small>
            </div>
          ))}
          {!data.notes?.length && <p className="muted">Chưa có ghi chú nào.</p>}
        </div>
        <div className="form-stack c360-note-form">
          <textarea
            value={note}
            onChange={(event) => setNote(event.target.value)}
            placeholder="Thêm ghi chú cho nhân viên (không gửi khách)…"
          />
          <button
            className="button secondary"
            disabled={busy}
            onClick={addNote}
          >
            <Icon name="plus" size={15} /> Thêm ghi chú
          </button>
          {error && <p role="alert">{error}</p>}
        </div>
      </section>

      <section className="panel">
        <h3>Nhật ký hoạt động đa kênh</h3>
        <ol className="c360-timeline">
          {data.timeline?.map((event, index) => (
            <li key={index} className={`c360-event ${event.kind}`}>
              <span className="c360-dot" />
              <div>
                <strong>
                  {event.kind}
                  {event.channel ? ` · ${event.channel}` : ""}
                </strong>{" "}
                — {event.subject}
                {event.content && <p>{event.content}</p>}
                <small>
                  {new Date(event.created_at).toLocaleString("vi-VN")}
                </small>
              </div>
            </li>
          ))}
          {!data.timeline?.length && (
            <p className="muted">Chưa có hoạt động.</p>
          )}
        </ol>
      </section>
    </div>
  );
}
