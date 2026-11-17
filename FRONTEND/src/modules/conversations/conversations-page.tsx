"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import type { Conversation } from "@/shared/domain/models";
import {
  Badge,
  EmptyState,
  ErrorState,
  Loading,
  PageHeading,
  Pagination,
} from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
import { ResourceEditor } from "../resources/resource-editor";
import { ConversationThread } from "./conversation-thread";
export function ConversationsPage() {
  const { conversations } = useServices();
  const [page, setPage] = useState(1);
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [selected, setSelected] = useState<Conversation | null>(null);
  const [editor, setEditor] = useState<{ entity?: Conversation } | null>(null);
  const load = useCallback(
    (signal: AbortSignal) =>
      conversations.list({ page, limit: 10, q, status }, signal),
    [conversations, page, q, status],
  );
  const result = useQuery(load);
  return (
    <>
      <PageHeading
        eyebrow="HỘP THƯ CHUNG"
        title="Hộp hội thoại"
        description="Mỗi cuộc trò chuyện là cơ hội để thấu hiểu hơn."
      >
        <button
          className="button secondary"
          aria-label="Làm mới danh sách"
          onClick={result.reload}
        >
          <Icon name="refresh" size={17} />
        </button>
        <button className="button primary" onClick={() => setEditor({})}>
          <Icon name="plus" size={17} />
          Tạo hội thoại
        </button>
      </PageHeading>
      <section className={`inbox panel ${selected ? "has-selection" : ""}`}>
        <div className="inbox-list">
          <div className="inbox-list-header">
            <h2>
              Tất cả hội thoại{" "}
              <span className="count-pill">{result.data?.total ?? "—"}</span>
            </h2>
            <label className="search-field">
              <Icon name="search" size={16} />
              <input
                aria-label="Tìm hội thoại"
                placeholder="Tìm theo chủ đề…"
                value={q}
                onChange={(event) => {
                  setQ(event.target.value);
                  setPage(1);
                }}
              />
            </label>
            <div className="filter-tabs">
              {[
                ["", "Tất cả"],
                ["open", "Đang mở"],
                ["closed", "Đã đóng"],
              ].map(([value, label]) => (
                <button
                  key={value}
                  className={status === value ? "active" : ""}
                  onClick={() => {
                    setStatus(value);
                    setPage(1);
                  }}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div className="inbox-items">
            {result.loading ? (
              <Loading />
            ) : result.error ? (
              <ErrorState message={result.error} retry={result.reload} />
            ) : !result.data?.items.length ? (
              <EmptyState
                icon="chat"
                title="Chưa có hội thoại"
                description="Tạo hội thoại để bắt đầu hỗ trợ."
              />
            ) : (
              result.data.items.map((item) => (
                <button
                  className={`conversation-item ${selected?.id === item.id ? "selected" : ""}`}
                  key={item.id}
                  onClick={() => setSelected(item)}
                >
                  <span className="conversation-top">
                    <span className="channel-icon">
                      <Icon
                        name={item.channel === "web" ? "globe" : "chat"}
                        size={16}
                      />
                    </span>
                    <span>
                      {item.channel === "web"
                        ? "Website"
                        : item.channel === "email"
                          ? "Email"
                          : "Điện thoại"}
                    </span>
                    <Badge value={item.status} />
                  </span>
                  <strong>{item.subject || "Hội thoại chưa có chủ đề"}</strong>
                  <span className="conversation-meta">
                    #{item.id.slice(0, 8)}
                    <span>
                      {new Intl.DateTimeFormat("vi-VN", {
                        day: "2-digit",
                        month: "2-digit",
                      }).format(new Date(item.updated_at))}
                    </span>
                  </span>
                </button>
              ))
            )}
          </div>
          {result.data && (
            <Pagination
              page={page}
              total={result.data.total}
              limit={10}
              onChange={setPage}
            />
          )}
        </div>
        {selected ? (
          <>
            <button
              className="inbox-back text-link"
              onClick={() => setSelected(null)}
            >
              ← Danh sách hội thoại
            </button>
            <ConversationThread
              key={selected.id}
              conversation={selected}
              onEdit={() => setEditor({ entity: selected })}
              onChanged={result.reload}
            />
          </>
        ) : (
          <div className="thread-placeholder">
            <div className="placeholder-art">
              <Icon name="chat" size={46} />
              <span>
                <Icon name="check" size={19} />
              </span>
            </div>
            <h2>Một kết nối đang chờ bạn</h2>
            <p>
              Chọn một hội thoại để xem lịch sử,
              <br />
              gửi phản hồi và hỗ trợ khách hàng.
            </p>
            <span className="placeholder-label">
              LẮNG NGHE TỐT HƠN. KẾT NỐI GẦN HƠN.
            </span>
          </div>
        )}
      </section>
      {editor && (
        <ResourceEditor
          kind="conversations"
          repository={conversations}
          entity={editor.entity}
          onClose={() => setEditor(null)}
          onSaved={(entity) => {
            setSelected(entity);
            setEditor(null);
            result.reload();
          }}
        />
      )}
    </>
  );
}
