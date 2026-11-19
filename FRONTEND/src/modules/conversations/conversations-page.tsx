"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
import { useQuery } from "@/shared/hooks/use-query";
import {
  toggleSelection,
  useCollectionFilters,
} from "../resources/use-collection-filters";
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
import { BulkToolbar, ViewControls } from "../resources/workspace-controls";
import { AssignmentFilter } from "../team/assignment";
import { ConversationThread } from "./conversation-thread";
import styles from "./conversations-page.module.css";

const channelLabels = { web: "Website", email: "Email", phone: "Điện thoại" };
const shortDate = new Intl.DateTimeFormat("vi-VN", {
  day: "2-digit",
  month: "2-digit",
});

export function ConversationsPage() {
  const user = useUser();
  const { conversations } = useServices();
  const {
    filters,
    setFilters,
    checked,
    setChecked,
    page,
    setPage,
    q,
    setQ,
    assignment,
    setAssignment,
    status,
    setStatus,
    query,
  } = useCollectionFilters(10);
  const [selected, setSelected] = useState<Conversation | null>(null);
  const [editor, setEditor] = useState<{ entity?: Conversation } | null>(null);
  const load = useCallback(
    (signal: AbortSignal) => conversations.list(query, signal),
    [conversations, query],
  );
  const result = useQuery(load, "workspace-refresh");
  const selectedId = selected?.id;
  const loadActive = useCallback(
    (signal: AbortSignal) =>
      selectedId
        ? conversations.get(selectedId, signal)
        : Promise.resolve(null),
    [conversations, selectedId],
  );
  const current = useQuery(loadActive, "workspace-refresh");
  const active =
    (current.data?.id === selectedId ? current.data : null) ??
    result.data?.items.find((item) => item.id === selectedId) ??
    selected;
  return (
    <div className={styles.page}>
      <PageHeading
        title="Hộp hội thoại"
        description="Theo dõi và trả lời tin nhắn hỗ trợ khách hàng."
      >
        {user.role === "owner" && (
          <a
            className="button secondary"
            href={`/chat/${user.tenant_id}`}
            target="_blank"
            rel="noreferrer"
          >
            Mở widget hỗ trợ
          </a>
        )}
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
      <section
        aria-label="Không gian hội thoại"
        className={`inbox panel ${styles.inbox} ${active ? "has-selection" : ""}`}
      >
        <div className="inbox-list">
          <div className="inbox-list-header">
            <h2>
              Hội thoại{" "}
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
            <div className={styles.assignment}>
              <AssignmentFilter
                value={assignment}
                onChange={(value) => {
                  setAssignment(value);
                  setPage(1);
                }}
              />
            </div>
            <ViewControls
              current={{
                ...filters,
                kind: "conversations",
                name: "",
                status,
                assignment,
                search: q,
              }}
              onApply={(view) => {
                setFilters({
                  priority: view.priority,
                  tags: view.tags,
                  assignee: view.assignee,
                });
                setQ(view.search);
                setStatus(view.status);
                setAssignment(view.assignment);
                setPage(1);
                setChecked([]);
              }}
            />
            <BulkToolbar
              kind="conversations"
              ids={checked}
              clear={() => setChecked([])}
              changed={() => {
                setPage(1);
                result.reload();
              }}
            />
            <div className="filter-tabs">
              {[
                ["", "Tất cả"],
                ["open", "Đang mở"],
                ["closed", "Đã đóng"],
              ].map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  aria-pressed={status === value}
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
                description={
                  q || status || assignment
                    ? "Thử thay đổi từ khóa hoặc bộ lọc."
                    : "Tạo hội thoại để bắt đầu hỗ trợ."
                }
              />
            ) : (
              result.data.items.map((item) => (
                <div className="selectable-conversation" key={item.id}>
                  <input
                    aria-label={`Chọn ${item.subject || item.id}`}
                    type="checkbox"
                    checked={checked.includes(item.id)}
                    onChange={() =>
                      setChecked((ids) => toggleSelection(ids, item.id))
                    }
                  />
                  <button
                    type="button"
                    aria-pressed={active?.id === item.id}
                    className={`conversation-item ${styles.item} ${active?.id === item.id ? "selected" : ""}`}
                    onClick={() => setSelected(item)}
                  >
                    <span
                      className={styles.channel}
                      data-channel={item.channel}
                    >
                      <Icon
                        name={item.channel === "web" ? "globe" : "chat"}
                        size={17}
                      />
                    </span>
                    <span className={styles.summary}>
                      <span className={styles.subject}>
                        <strong>
                          {item.subject || "Hội thoại chưa có chủ đề"}
                        </strong>
                        {item.unread && (
                          <span
                            className={styles.unread}
                            role="img"
                            aria-label="Chưa đọc"
                            title="Chưa đọc"
                          />
                        )}
                      </span>
                      <span className={styles.metadata}>
                        {channelLabels[item.channel]}{" "}
                        <span aria-hidden="true">·</span> #{item.id.slice(0, 8)}
                      </span>
                      {Boolean(item.tags?.length) && (
                        <span className="keywords">
                          {item.tags?.map((tag) => (
                            <span key={tag}>{tag}</span>
                          ))}
                        </span>
                      )}
                    </span>
                    <span className={styles.status}>
                      <time dateTime={item.updated_at}>
                        {shortDate.format(new Date(item.updated_at))}
                      </time>
                      <Badge value={item.status} />
                    </span>
                  </button>
                </div>
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
        {active ? (
          <>
            <button
              className="inbox-back text-link"
              onClick={() => setSelected(null)}
            >
              ← Danh sách hội thoại
            </button>
            <ConversationThread
              key={active.id}
              conversation={active}
              onEdit={() => setEditor({ entity: active })}
              onAssigned={setSelected}
              onChanged={() => {
                result.reload();
                current.reload();
              }}
            />
          </>
        ) : (
          <div className="thread-placeholder">
            <div className={styles.emptyMark} aria-hidden="true">
              <Icon name="chat" size={38} />
            </div>
            <h2>Chưa chọn hội thoại</h2>
            <p>Chọn một hội thoại từ danh sách để xem tin nhắn và trả lời.</p>
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
            current.reload();
          }}
        />
      )}
    </div>
  );
}
