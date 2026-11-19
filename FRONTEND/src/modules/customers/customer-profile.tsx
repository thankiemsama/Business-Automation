"use client";
import { FeatureSections } from "@/shared/ui/feature-sections";
import { useRouter } from "next/navigation";
import { useUser } from "@/shared/providers/workspace-provider";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import { Badge } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";

// Customer 360°: a cross-channel, cross-record view of one customer with a
// full activity timeline, lifetime-value signals, notes, and quick actions.
export function CustomerProfilePanel({ id }: { id: string }) {
  const router = useRouter();
  const { role } = useUser();
  const { platform, customers } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => platform.profile(id, signal),
    [platform, id],
  );
  const profile = useQuery(load, "workspace-refresh");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [mergeTarget, setMergeTarget] = useState("");
  const [mergeNote, setMergeNote] = useState("");
  const [mergeBusy, setMergeBusy] = useState(false);
  const peerEmail = profile.data?.customer.email ?? "";
  const peerLoad = useCallback(
    (signal: AbortSignal) =>
      role === "owner"
        ? customers.list({ q: peerEmail, limit: 20 }, signal)
        : Promise.resolve(undefined),
    [customers, peerEmail, role],
  );
  const peers = useQuery(peerLoad);
  const candidates =
    peers.data?.items.filter(
      (customer) =>
        customer.id !== id &&
        (!peerEmail ||
          !customer.email ||
          customer.email.trim().toLowerCase() ===
            peerEmail.trim().toLowerCase()),
    ) ?? [];

  async function merge() {
    if (!mergeTarget || mergeTarget === id) return;
    setMergeBusy(true);
    setMergeNote("");
    try {
      await platform.mergeCustomers(id, mergeTarget);
      router.replace(`/customers/${encodeURIComponent(mergeTarget)}`);
    } catch (err) {
      setMergeNote(errorMessage(err));
    } finally {
      setMergeBusy(false);
    }
  }

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
      <FeatureSections
        label="Hồ sơ khách hàng"
        sections={[
          {
            id: "profile",
            label: "Tổng quan",
            content: (
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
                        {data.average_rating
                          ? data.average_rating.toFixed(1)
                          : "—"}
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
                    Đã xử lý <strong>{data.ltv.resolved}</strong>/
                    {data.ltv.tickets} phiếu · Hoạt động cuối cùng:{" "}
                    <strong>
                      {data.ltv.last_active
                        ? new Date(data.ltv.last_active).toLocaleDateString(
                            "vi-VN",
                          )
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
            ),
          },
          {
            id: "activity",
            label: "Hoạt động",
            content: (
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
            ),
          },
          {
            id: "notes",
            label: "Ghi chú nội bộ",
            content: (
              <section className="panel">
                <h3>Ghi chú nội bộ</h3>
                <div className="c360-notes">
                  {data.notes?.map((item) => (
                    <div key={item.id} className="c360-note">
                      <p>{item.content}</p>
                      <small>
                        {new Date(item.created_at).toLocaleString("vi-VN")}
                      </small>
                    </div>
                  ))}
                  {!data.notes?.length && (
                    <p className="muted">Chưa có ghi chú nào.</p>
                  )}
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
            ),
          },
          ...(role === "owner" && candidates.length
            ? [
                {
                  id: "merge",
                  label: "Gộp hồ sơ",
                  content: (
                    <section className="panel">
                      <h3>Nhận diện &amp; gộp hồ sơ</h3>
                      <p className="muted">
                        Các hồ sơ khác cùng email hoặc trùng danh bạ. Gộp sẽ
                        chuyển toàn bộ hội thoại, phiếu và ghi chú từ hồ sơ này
                        sang hồ sơ đích; hồ sơ hiện tại được đánh dấu là đã gộp.
                      </p>
                      <div className="form-stack">
                        <label>
                          Gộp hồ sơ này vào:
                          <select
                            value={mergeTarget}
                            onChange={(event) =>
                              setMergeTarget(event.target.value)
                            }
                            disabled={mergeBusy}
                          >
                            <option value="">— Chọn hồ sơ đích —</option>
                            {candidates.map((c) => (
                              <option key={c.id} value={c.id}>
                                {c.name}
                                {c.email ? ` (${c.email})` : ""}
                              </option>
                            ))}
                          </select>
                        </label>
                        <button
                          className="button secondary"
                          disabled={mergeBusy || !mergeTarget}
                          onClick={() => void merge()}
                        >
                          <Icon name="plus" size={15} /> Gộp hồ sơ khách
                        </button>
                        {mergeNote && <p role="status">{mergeNote}</p>}
                      </div>
                    </section>
                  ),
                },
              ]
            : []),
        ]}
      />
    </div>
  );
}
