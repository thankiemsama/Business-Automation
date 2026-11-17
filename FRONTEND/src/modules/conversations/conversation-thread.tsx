"use client";
import { useCallback, useState, type FormEvent } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import type { Conversation } from "@/shared/domain/models";
import {
  Avatar,
  Badge,
  EmptyState,
  ErrorState,
  Loading,
  Pagination,
} from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
export function ConversationThread({
  conversation,
  onEdit,
  onChanged,
}: {
  conversation: Conversation;
  onEdit: () => void;
  onChanged: () => void;
}) {
  const { conversations, customers } = useServices();
  const [page, setPage] = useState(1);
  const [content, setContent] = useState("");
  const [sender, setSender] = useState<"agent" | "customer">("agent");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const load = useCallback(
    (signal: AbortSignal) =>
      conversations.messages(conversation.id, { page, limit: 30 }, signal),
    [conversations, conversation.id, page],
  );
  const customerLoad = useCallback(
    (signal: AbortSignal) => customers.get(conversation.customer_id, signal),
    [customers, conversation.customer_id],
  );
  const messages = useQuery(load);
  const customer = useQuery(customerLoad);
  const closed = conversation.status === "closed";
  async function mutate(action: () => Promise<unknown>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      messages.reload();
      onChanged();
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  async function send(event: FormEvent) {
    event.preventDefault();
    await mutate(async () => {
      await conversations.send(conversation.id, content, sender);
      setContent("");
      const total = (messages.data?.total || 0) + 1;
      setPage(Math.ceil(total / 30));
    });
  }
  async function reply() {
    await mutate(async () => {
      const reply = await conversations.reply(conversation.id);
      setNotice(
        reply.needs_human
          ? "Chưa có kiến thức phù hợp. Nhân viên cần tiếp tục hỗ trợ khách hàng."
          : `Đã phản hồi từ bài kiến thức #${reply.source_id?.slice(0, 8)}.`,
      );
      setPage(Math.ceil(((messages.data?.total || 0) + 1) / 30));
    });
  }
  return (
    <>
      <section className="thread">
        <div className="thread-header">
          <Avatar name={customer.data?.name || "KH"} />
          <div>
            <h2>{conversation.subject || "Hội thoại khách hàng"}</h2>
            <span>
              {customer.data?.name ||
                `Khách hàng #${conversation.customer_id.slice(0, 8)}`}
            </span>
          </div>
          <Badge value={conversation.status} />
          <button
            className="icon-button"
            onClick={() => {
              messages.reload();
              customer.reload();
            }}
            aria-label="Làm mới hội thoại"
          >
            <Icon name="refresh" size={17} />
          </button>
          <button
            className="icon-button"
            onClick={onEdit}
            aria-label="Sửa hội thoại"
          >
            <Icon name="edit" size={17} />
          </button>
        </div>
        <div className="thread-messages" aria-live="polite">
          {messages.loading ? (
            <Loading />
          ) : messages.error ? (
            <ErrorState message={messages.error} retry={messages.reload} />
          ) : !messages.data?.items.length ? (
            <EmptyState
              icon="chat"
              title="Bắt đầu cuộc trò chuyện"
              description="Gửi lời chào đầu tiên hoặc ghi nhận tin nhắn của khách hàng."
            />
          ) : (
            <>
              <div className="thread-start">
                Lịch sử hội thoại · <Badge value={conversation.channel} />
              </div>
              {messages.data.items.map((message) => (
                <div
                  key={message.id}
                  className={`message ${message.sender === "customer" ? "inbound" : "outbound"}`}
                >
                  <span className="message-author">
                    {message.sender === "customer"
                      ? customer.data?.name || "Khách hàng"
                      : message.sender === "assistant"
                        ? "Trợ lý kiến thức"
                        : "Nhân viên"}
                  </span>
                  <p>{message.content}</p>
                  <time>
                    {new Intl.DateTimeFormat("vi-VN", {
                      hour: "2-digit",
                      minute: "2-digit",
                      day: "2-digit",
                      month: "2-digit",
                    }).format(new Date(message.created_at))}
                  </time>
                </div>
              ))}
            </>
          )}
        </div>
        {messages.data && messages.data.total > 30 && (
          <Pagination
            page={page}
            total={messages.data.total}
            limit={30}
            onChange={setPage}
          />
        )}
        {notice && (
          <div className="subtle-note" role="status">
            <Icon name="info" size={17} />
            {notice}
          </div>
        )}
        {error && <ErrorState message={error} />}
        {closed ? (
          <div className="closed-notice">
            <Icon name="check" size={18} />
            Hội thoại đã đóng.
            <button className="text-link" onClick={onEdit}>
              Mở lại hội thoại
            </button>
          </div>
        ) : (
          <form className="composer" onSubmit={send}>
            <div className="composer-toolbar">
              <label>
                Gửi với vai trò
                <select
                  value={sender}
                  onChange={(event) =>
                    setSender(event.target.value as "agent" | "customer")
                  }
                  disabled={busy}
                >
                  <option value="agent">Nhân viên</option>
                  <option value="customer">Khách hàng (ghi nhận)</option>
                </select>
              </label>
              <button
                className="button secondary compact"
                type="button"
                disabled={busy}
                onClick={reply}
              >
                <Icon name="bolt" size={15} />
                Phản hồi từ kiến thức
              </button>
            </div>
            <textarea
              aria-label="Nội dung tin nhắn"
              placeholder="Nhập nội dung tin nhắn…"
              value={content}
              onChange={(event) => setContent(event.target.value)}
              required
              maxLength={16000}
              disabled={busy}
            />
            <div className="composer-footer">
              <span>Tin nhắn được lưu trong hội thoại.</span>
              <button
                className="button primary compact"
                disabled={busy || !content.trim()}
              >
                <Icon name="send" size={16} />
                {busy ? "Đang gửi…" : "Gửi tin nhắn"}
              </button>
            </div>
          </form>
        )}
      </section>
      <aside className="customer-detail">
        <h3>Thông tin khách hàng</h3>
        {customer.loading ? (
          <Loading />
        ) : customer.error ? (
          <ErrorState message={customer.error} retry={customer.reload} />
        ) : (
          customer.data && (
            <>
              <div className="customer-profile">
                <Avatar name={customer.data.name} />
                <strong>{customer.data.name}</strong>
                <span>Khách hàng</span>
              </div>
              <dl>
                <dt>Email</dt>
                <dd>{customer.data.email || "Chưa cung cấp"}</dd>
                <dt>Điện thoại</dt>
                <dd>{customer.data.phone || "Chưa cung cấp"}</dd>
                <dt>Kênh liên hệ</dt>
                <dd>
                  <Badge value={conversation.channel} />
                </dd>
                <dt>Mã hội thoại</dt>
                <dd className="mono">{conversation.id}</dd>
              </dl>
            </>
          )
        )}
        <div className="detail-note">
          <Icon name="shield" size={19} />
          <p>Thông tin chỉ hiển thị trong doanh nghiệp của bạn.</p>
        </div>
      </aside>
    </>
  );
}
