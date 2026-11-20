"use client";
import Link from "next/link";
import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
} from "react";
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
import { RealtimeQACoaching } from "./realtime-qa";
import { ConversationBrandCard } from "./brand-card";
import { Assignment } from "../team/assignment";
import { Icon } from "@/shared/ui/icon";
import styles from "./conversation-composer.module.css";
import profile from "./conversation-profile.module.css";

const maxAttachmentSize = 32 * 1024;
const composerModes = {
  agent: {
    placeholder: "Viết phản hồi cho khách hàng…",
    action: "Gửi tin nhắn",
  },
  note: {
    placeholder: "Viết ghi chú cho đội ngũ hỗ trợ…",
    action: "Lưu ghi chú",
  },
  customer: {
    placeholder: "Ghi lại nội dung khách hàng đã gửi…",
    action: "Ghi nhận tin nhắn",
  },
} as const;

export function ConversationThread({
  conversation,
  onEdit,
  onChanged,
  onAssigned,
  showDetails = true,
}: {
  conversation: Conversation;
  onEdit: () => void;
  onChanged: () => void;
  onAssigned: (conversation: Conversation) => void;
  showDetails?: boolean;
}) {
  const { conversations, customers, knowledge } = useServices();
  const [page, setPage] = useState(1);
  const [file, setFile] = useState<File | null>(null);
  const attachmentInput = useRef<HTMLInputElement>(null);
  const editor = useRef<HTMLTextAreaElement>(null);
  const [attachmentError, setAttachmentError] = useState("");
  const [content, setContent] = useState("");
  const [sender, setSender] = useState<"agent" | "customer" | "note">("agent");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [source, setSource] = useState<Knowledge | null>(null);
  const [draftSourceMsg, setDraftSourceMsg] = useState("");
  const [notice, setNotice] = useState("");
  const [profileOpen, setProfileOpen] = useState(false);
  const profileId = useId();
  const profileToggle = useRef<HTMLButtonElement>(null);
  const profileClose = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (profileOpen) profileClose.current?.focus();
  }, [profileOpen]);

  function closeProfile() {
    setProfileOpen(false);
    profileToggle.current?.focus();
  }
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
    sender === "agent" &&
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
  const mode = composerModes[sender];
  const canSend =
    !busy && !draftContextChanged && Boolean(content.trim() || file);
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
    if (!canSend) return;
    await mutate(async () => {
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
      setAttachmentError("");
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
        <header className={`thread-header ${profile.threadHeader}`}>
          <Avatar name={customer.data?.name || "KH"} />
          <div>
            <h2>{conversation.subject || "Hội thoại khách hàng"}</h2>
            <span>
              {customer.data?.name ||
                `Khách hàng #${conversation.customer_id.slice(0, 8)}`}
            </span>
          </div>
          <div className={profile.headerActions}>
            <Badge value={conversation.status} />
            <button
              ref={profileToggle}
              type="button"
              className={`icon-button ${profile.toggle}`}
              aria-label="Thông tin khách hàng"
              title="Thông tin khách hàng"
              aria-controls={profileId}
              aria-expanded={profileOpen}
              onClick={() => setProfileOpen(!profileOpen)}
            >
              <Icon name="users" size={18} />
            </button>
            <button
              className="icon-button"
              onClick={() => {
                messages.reload();
                customer.reload();
              }}
              aria-label="Làm mới hội thoại"
              title="Làm mới hội thoại"
            >
              <Icon name="refresh" size={17} />
            </button>
            <button
              className="icon-button"
              onClick={onEdit}
              aria-label="Sửa hội thoại"
              title="Sửa hội thoại"
            >
              <Icon name="edit" size={17} />
            </button>
          </div>
          {showDetails && (
            <Link
              className={profile.detailsLink}
              href={`/conversations/${encodeURIComponent(conversation.id)}`}
            >
              Chi tiết hội thoại <Icon name="chevron" size={13} />
            </Link>
          )}
        </header>
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
                  {message.brand_card && (
                    <ConversationBrandCard card={message.brand_card} />
                  )}
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
        {sender === "agent" && conversation.status !== "closed" && (
          <RealtimeQACoaching
            key={conversation.id}
            id={conversation.id}
            draft={content}
          />
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
          <form
            className={styles.composer}
            data-mode={sender}
            onSubmit={send}
            aria-busy={busy}
          >
            <div className={styles.toolbar}>
              <label className={styles.mode}>
                <span>Chế độ soạn</span>
                <select
                  value={sender}
                  onChange={(event) =>
                    setSender(event.target.value as keyof typeof composerModes)
                  }
                  disabled={busy}
                >
                  <option value="agent">Trả lời khách hàng</option>
                  <option value="note">Ghi chú nội bộ</option>
                  <option value="customer">Ghi nhận tin của khách</option>
                </select>
              </label>
              <button
                className={styles.tool}
                type="button"
                disabled={busy}
                onClick={reply}
              >
                <Icon name="bolt" size={16} />
                Soạn nháp từ kiến thức
              </button>
              <ReplyTemplates
                disabled={busy}
                onSelect={(content) => {
                  setDraftSourceMsg("");
                  setContent(content);
                  setSender("agent");
                  editor.current?.focus();
                }}
              />
            </div>
            <textarea
              ref={editor}
              className={styles.editor}
              aria-label="Nội dung tin nhắn"
              placeholder={mode.placeholder}
              value={content}
              onChange={(event) => setContent(event.target.value)}
              onKeyDown={(event) => {
                if (
                  event.key !== "Enter" ||
                  !(event.ctrlKey || event.metaKey) ||
                  event.nativeEvent.isComposing
                )
                  return;
                event.preventDefault();
                if (canSend) event.currentTarget.form?.requestSubmit();
              }}
              required={!file}
              maxLength={16000}
              disabled={busy}
            />
            {file && (
              <div className={styles.attachment}>
                <Icon name="attachment" size={16} />
                <span title={file.name}>{file.name}</span>
                <button
                  className={styles.tool}
                  type="button"
                  aria-label="Bỏ tệp đính kèm"
                  disabled={busy}
                  onClick={() => setFile(null)}
                >
                  <Icon name="close" size={15} />
                </button>
              </div>
            )}
            {attachmentError && (
              <p className={styles.attachmentError} role="alert">
                {attachmentError}
              </p>
            )}
            <input
              ref={attachmentInput}
              hidden
              type="file"
              accept="image/png,image/jpeg,application/pdf"
              disabled={busy}
              onChange={(event) => {
                const picked = event.target.files?.[0];
                event.target.value = "";
                if (!picked) return;
                if (picked.size > maxAttachmentSize) {
                  setFile(null);
                  setAttachmentError("Tệp vượt quá giới hạn 32 KiB.");
                  return;
                }
                setAttachmentError("");
                setFile(picked);
              }}
            />
            <VoiceInput
              disabled={busy}
              onText={(text) =>
                setContent((current) =>
                  current ? `${current}\n${text}` : text,
                )
              }
            />
            <div className={styles.footer}>
              <button
                className={styles.tool}
                type="button"
                title="PNG, JPEG hoặc PDF · tối đa 32 KiB"
                aria-label="Đính kèm tệp PNG, JPEG hoặc PDF, tối đa 32 KiB"
                disabled={busy}
                onClick={() => attachmentInput.current?.click()}
              >
                <Icon name="attachment" size={17} />
                Đính kèm
              </button>
              <button
                className={`button primary ${styles.send}`}
                type="submit"
                aria-keyshortcuts="Control+Enter Meta+Enter"
                disabled={!canSend}
              >
                <Icon name={sender === "agent" ? "send" : "check"} size={16} />
                {mode.action}
              </button>
            </div>
          </form>
        )}
      </section>
      <aside
        id={profileId}
        className={profile.sidebar}
        data-open={profileOpen}
        aria-labelledby={`${profileId}-title`}
        onKeyDown={(event) => {
          if (profileOpen && event.key === "Escape") closeProfile();
        }}
      >
        <header className={profile.heading}>
          <h3 id={`${profileId}-title`}>Thông tin khách hàng</h3>
          <button
            ref={profileClose}
            type="button"
            className={`icon-button ${profile.toggle}`}
            aria-label="Đóng thông tin khách hàng"
            onClick={closeProfile}
          >
            <Icon name="close" size={18} />
          </button>
        </header>
        {customer.loading ? (
          <Loading />
        ) : customer.error ? (
          <ErrorState message={customer.error} retry={customer.reload} />
        ) : (
          customer.data && (
            <>
              <div className={profile.identity}>
                <Avatar name={customer.data.name} />
                <div>
                  <strong>{customer.data.name}</strong>
                  <Link
                    href={`/customers/${encodeURIComponent(customer.data.id)}`}
                  >
                    Xem hồ sơ <Icon name="chevron" size={12} />
                  </Link>
                </div>
              </div>
              <dl className={profile.metadata}>
                <dt>Email</dt>
                <dd>
                  {customer.data.email ? (
                    <a
                      href={`mailto:${encodeURIComponent(customer.data.email).replace(/%40/g, "@")}`}
                    >
                      {customer.data.email}
                    </a>
                  ) : (
                    "Chưa cung cấp"
                  )}
                </dd>
                <dt>Điện thoại</dt>
                <dd>{customer.data.phone || "Chưa cung cấp"}</dd>
              </dl>
            </>
          )
        )}
        <section className={profile.assignment}>
          <h4>Phụ trách hội thoại</h4>
          <Assignment
            assignee={conversation.assignee_id}
            save={async (id, claim) => {
              onAssigned(
                await conversations.assign(conversation.id, id, claim),
              );
              onChanged();
            }}
          />
        </section>
        <dl className={`${profile.metadata} ${profile.conversation}`}>
          <dt>Kênh liên hệ</dt>
          <dd>
            <Badge value={conversation.channel} />
          </dd>
          <dt>Mã hội thoại</dt>
          <dd>
            <code>{conversation.id}</code>
          </dd>
        </dl>
      </aside>
    </>
  );
}
