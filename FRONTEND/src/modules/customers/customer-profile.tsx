"use client";
import { FeatureSections } from "@/shared/ui/feature-sections";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useUser } from "@/shared/providers/workspace-provider";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import { Badge, ErrorState, Loading } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
import styles from "./customer-profile.module.css";

const eventLabels: Record<string, string> = {
  message: "Tin nhắn",
  ticket: "Phiếu hỗ trợ",
  conversation: "Hội thoại",
  note: "Ghi chú nội bộ",
};
const dateTime = new Intl.DateTimeFormat("vi-VN", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "Asia/Ho_Chi_Minh",
});

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
    return <ErrorState message={profile.error} retry={profile.reload} />;
  const data = profile.data;
  if (!data) return <Loading />;

  return (
    <div className={styles.profile}>
      <FeatureSections
        label="Hồ sơ khách hàng"
        sections={[
          {
            id: "profile",
            label: "Tổng quan",
            content: (
              <div className={styles.columns}>
                <section className={`panel ${styles.card}`}>
                  <h3>Chỉ số hỗ trợ</h3>
                  <dl className={styles.metrics}>
                    <div>
                      <dt>Phiếu hỗ trợ</dt>
                      <dd>{data.total_tickets}</dd>
                    </div>
                    <div>
                      <dt>Đang mở</dt>
                      <dd>{data.open_tickets}</dd>
                    </div>
                    <div>
                      <dt>Hài lòng / 5</dt>
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

                <section className={`panel ${styles.card}`}>
                  <h3>Lịch sử hỗ trợ</h3>
                  <dl className={styles.history}>
                    <div>
                      <dt>Phiếu đã xử lý</dt>
                      <dd>
                        {data.ltv.resolved} / {data.ltv.tickets}
                      </dd>
                    </div>
                    <div>
                      <dt>Hoạt động gần nhất</dt>
                      <dd>
                        {data.ltv.last_active ? (
                          <time dateTime={data.ltv.last_active}>
                            {dateTime.format(new Date(data.ltv.last_active))}
                          </time>
                        ) : (
                          "—"
                        )}
                      </dd>
                    </div>
                  </dl>
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
                  {!!data.ai_impressions?.length && (
                    <>
                      <h4>Ghi nhận về khách hàng từ các phiếu gần đây</h4>
                      <ul>
                        {data.ai_impressions.map((impression) => (
                          <li
                            key={`${impression.ticket_id}:${impression.created_at}`}
                          >
                            <Link
                              href={`/tickets/${encodeURIComponent(impression.ticket_id)}`}
                            >
                              {impression.text}
                            </Link>
                          </li>
                        ))}
                      </ul>
                    </>
                  )}
                </section>
              </div>
            ),
          },
          {
            id: "activity",
            label: "Hoạt động",
            content: (
              <section className={`panel ${styles.card}`}>
                <h3>Nhật ký hoạt động đa kênh</h3>
                {!data.timeline?.length ? (
                  <p className="muted">Chưa có hoạt động.</p>
                ) : (
                  <ol className={styles.timeline}>
                    {data.timeline?.map((event, index) => (
                      <li key={index}>
                        <div className={styles.eventMeta}>
                          <span>{eventLabels[event.kind] ?? event.kind}</span>
                          {event.channel && <Badge value={event.channel} />}
                          <time dateTime={event.created_at}>
                            {dateTime.format(new Date(event.created_at))}
                          </time>
                        </div>
                        {event.subject && <strong>{event.subject}</strong>}
                        {event.content && <p>{event.content}</p>}
                      </li>
                    ))}
                  </ol>
                )}
              </section>
            ),
          },
          {
            id: "notes",
            label: "Ghi chú nội bộ",
            content: (
              <div className={styles.columns}>
                <form
                  className={`panel ${styles.card}`}
                  onSubmit={(event) => {
                    event.preventDefault();
                    void addNote();
                  }}
                >
                  <h3>Thêm ghi chú</h3>
                  <label>
                    Nội dung nội bộ
                    <textarea
                      value={note}
                      onChange={(event) => setNote(event.target.value)}
                      placeholder="Ghi lại thông tin để đội ngũ theo dõi…"
                    />
                  </label>
                  <button
                    className="button primary"
                    disabled={busy || !note.trim()}
                  >
                    <Icon name="plus" size={15} />{" "}
                    {busy ? "Đang lưu…" : "Thêm ghi chú"}
                  </button>
                  {error && <p role="alert">{error}</p>}
                </form>
                <section className={`panel ${styles.card}`}>
                  <h3>Ghi chú nội bộ</h3>
                  <div className={styles.notes}>
                    {data.notes?.map((item) => (
                      <article key={item.id}>
                        <p>{item.content}</p>
                        <time dateTime={item.created_at}>
                          {dateTime.format(new Date(item.created_at))}
                        </time>
                      </article>
                    ))}
                    {!data.notes?.length && (
                      <p className="muted">Chưa có ghi chú nào.</p>
                    )}
                  </div>
                </section>
              </div>
            ),
          },
          ...(role === "owner" && candidates.length
            ? [
                {
                  id: "merge",
                  label: "Gộp hồ sơ",
                  content: (
                    <section className={`panel ${styles.card}`}>
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
