"use client";
import Link from "next/link";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import type { Conversation, Knowledge } from "@/shared/domain/models";
import {
  Avatar,
  Badge,
  EmptyState,
  ErrorState,
  Loading,
  Pagination,
} from "@/shared/ui/primitives";
import { AISummary, VoiceInput } from "../ai/assist";
import { ReplyTemplates } from "./reply-templates";
import { Assignment } from "../team/assignment";
import { Icon } from "@/shared/ui/icon";
export function ConversationThread({
  conversation,
  onEdit,
  onChanged,
  onAssigned,
}: {
  conversation: Conversation;
  onEdit: () => void;
  onChanged: () => void;
  onAssigned: (conversation: Conversation) => void;
}) {
  const { conversations, customers, knowledge } = useServices();
  const [page, setPage] = useState(1);
  const [file, setFile] = useState<File | null>(null);
  const [fileVersion, setFileVersion] = useState(0);
  const [content, setContent] = useState("");
  const [sender, setSender] = useState<"agent" | "customer" | "note">("agent");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [source, setSource] = useState<Knowledge | null>(null);
  const [draftSourceMsg, setDraftSourceMsg] = useState("");
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
  const messages = useQuery(
    load,
    "workspace-refresh",
    conversation.channel === "email" ? 10_000 : undefined,
  );
  const visibleMessages = messages.data?.items ?? [];
  const draftSourceIndex = draftSourceMsg
    ? visibleMessages.findIndex((message) => message.id === draftSourceMsg)
    : -1;
  const draftContextChanged =
    draftSourceIndex >= 0 &&
    visibleMessages
      .slice(draftSourceIndex + 1)
      .some((message) => message.sender !== "note");
  const seen = Math.max(
    0,
    ...(messages.data?.items ?? []).map((message) =>
      message.sender === "customer" ? (message.sequence ?? 0) : 0,
    ),
  );
  useEffect(() => {
    if (seen) void conversations.read(conversation.id, seen).catch(() => {});
  }, [conversations, conversation.id, seen]);
  const customer = useQuery(customerLoad);
  const closed = conversation.status === "closed";
  async function mutate(action: () => Promise<unknown>) {
    setBusy(true);
    setError("");
    setNotice("");
    setSource(null);
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
      if (file && file.size > 32 * 1024) throw new Error("Tệp tối đa 32 KiB.");
      const attachment = file
        ? {
            attachment_name: file.name,
            attachment_data: btoa(
              String.fromCharCode(...new Uint8Array(await file.arrayBuffer())),
            ),
          }
        : undefined;
      const message = await conversations.send(
        conversation.id,
        content,
        sender,
        attachment,
        sender === "agent" ? draftSourceMsg || undefined : undefined,
      );
      if (message.status === "pending")
        setNotice("Tin nhắn đã được xếp hàng gửi.");
      setFile(null);
      setFileVersion((version) => version + 1);
      setContent("");
      setDraftSourceMsg("");
      const total = (messages.data?.total || 0) + 1;
      setPage(Math.ceil(total / 30));
    });
  }
  async function reply() {
    await mutate(async () => {
      const reply = await conversations.draft(conversation.id);
      if (!reply.needs_human) {
        setContent(reply.message.content);
        setSender("agent");
        setDraftSourceMsg(reply.source_message_id ?? "");
        if (reply.source_id) setSource(await knowledge.get(reply.source_id));
      }
      setNotice(
        reply.needs_human
          ? "Đã tạo phiếu để nhân viên tiếp tục hỗ trợ."
          : reply.source_id
            ? `Đã soạn bản nháp từ bài #${reply.source_id.slice(0, 8)}` +
              (reply.source_message_id
                ? ` · Tin nguồn #${reply.source_message_id.slice(0, 8)}. Nếu khách gửi thêm tin, hãy soạn lại trước khi gửi.`
                : ". Kiểm tra trước khi gửi.")
            : "Gợi ý AI tổng quát, chưa có nguồn nội bộ. Kiểm tra trước khi gửi.",
      );
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
        <Link
          className="text-link"
          href={`/conversations/${encodeURIComponent(conversation.id)}`}
        >
          Chi tiết và trường tùy chỉnh
        </Link>
        <AISummary
          kind="conversations"
          id={conversation.id}
          saved={conversation.ai_summary}
        />
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
                        : message.sender === "note"
                          ? "Ghi chú nội bộ"
                          : "Nhân viên"}
                  </span>
                  <p>{message.content}</p>
                  {message.sentiment === "angry" && (
                    <strong role="status">Khách có dấu hiệu tức giận</strong>
                  )}
                  {message.sentiment === "negative" && (
                    <small>Cảm xúc tiêu cực</small>
                  )}
                  {message.attachment_data && (
                    <a
                      download={message.attachment_name}
                      href={`data:application/octet-stream;base64,${message.attachment_data}`}
                    >
                      {message.attachment_name}
                    </a>
                  )}
                  {message.status === "pending" &&
                    conversation.channel === "email" && (
                      <button
                        type="button"
                        className="text-link"
                        disabled={busy}
                        onClick={() =>
                          void mutate(() =>
                            conversations.deliver(conversation.id, message.id),
                          )
                        }
                      >
                        Email đang chờ gửi · Kiểm tra lại
                      </button>
                    )}
                  {(message.status === "pending" ||
                    message.status === "sending") &&
                    conversation.channel !== "email" && (
                      <small>Đang chờ nhà cung cấp tiếp nhận</small>
                    )}
                  {message.status === "sent" && (
                    <small>Nhà cung cấp đã tiếp nhận tin nhắn</small>
                  )}
                  {message.status === "failed" && (
                    <small role="status">
                      Cần đối soát · Đã dừng gửi tự động
                    </small>
                  )}
                  {message.attachment_name && !message.attachment_data && (
                    <small>{message.attachment_name}</small>
                  )}
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
        {messages.data && messages.data.total > page * 30 && (
          <button
            className="text-link"
            type="button"
            onClick={() => setPage(Math.ceil((messages.data?.total ?? 1) / 30))}
          >
            Đến tin nhắn mới nhất
          </button>
        )}
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
        {source && (
          <details className="subtle-note">
            <summary>Nguồn · {source.subject}</summary>
            <p>{source.content}</p>
          </details>
        )}
        {draftSourceMsg && (
          <div className="subtle-note" role="status">
            <Icon name="info" size={17} />
            {draftContextChanged
              ? "Hội thoại có tin mới. Hãy soạn lại bản nháp trước khi gửi."
              : `Tin nguồn #${draftSourceMsg.slice(0, 8)}`}
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
            <ReplyTemplates
              onSelect={(content) => {
                setDraftSourceMsg("");
                setContent(content);
                setSender("agent");
              }}
            />
            <div className="composer-toolbar">
              <label>
                Gửi với vai trò
                <select
                  value={sender}
                  onChange={(event) =>
                    setSender(
                      event.target.value as "agent" | "customer" | "note",
                    )
                  }
                  disabled={busy}
                >
                  <option value="agent">Nhân viên</option>
                  <option value="note">Ghi chú nội bộ</option>
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
                Soạn nháp từ kiến thức
              </button>
            </div>
            <textarea
              aria-label="Nội dung tin nhắn"
              placeholder="Nhập nội dung tin nhắn…"
              value={content}
              onChange={(event) => setContent(event.target.value)}
              required={!file}
              maxLength={16000}
              disabled={busy}
            />
            <label>
              Tệp đính kèm (PNG, JPEG, PDF · tối đa 32 KiB)
              <input
                key={fileVersion}
                type="file"
                accept="image/png,image/jpeg,application/pdf"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                disabled={busy}
              />
            </label>
            <VoiceInput
              disabled={busy}
              onText={(text) =>
                setContent((current) =>
                  current ? `${current}\n${text}` : text,
                )
              }
            />
            <div className="composer-footer">
              <span>Tin nhắn được lưu trong hội thoại.</span>
              <button
                className="button primary compact"
                disabled={busy || (!content.trim() && !file)}
              >
                <Icon name="send" size={16} />
                {busy ? "Đang gửi…" : "Gửi tin nhắn"}
              </button>
            </div>
          </form>
        )}
      </section>
      <aside className="customer-detail">
        <Assignment
          assignee={conversation.assignee_id}
          save={async (id, claim) => {
            onAssigned(await conversations.assign(conversation.id, id, claim));
            onChanged();
          }}
        />
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
