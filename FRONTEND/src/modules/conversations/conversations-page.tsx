"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
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
import {
  BulkToolbar,
  ViewControls,
  emptyFilters,
  toggleSelection,
} from "../resources/workspace-controls";
import { AssignmentFilter } from "../team/assignment";
import { ConversationThread } from "./conversation-thread";
export function ConversationsPage() {
  const user = useUser();
  const { conversations } = useServices();
  const [filters, setFilters] = useState(emptyFilters);
  const [checked, setChecked] = useState<string[]>([]);
  const [page, setPage] = useState(1);
  const [q, setQ] = useState("");
  const [assignment, setAssignment] = useState("");
  const [status, setStatus] = useState("");
  const [selected, setSelected] = useState<Conversation | null>(null);
  const [editor, setEditor] = useState<{ entity?: Conversation } | null>(null);
  const load = useCallback(
    (signal: AbortSignal) =>
      conversations.list(
        {
          page,
          limit: 10,
          q,
          status,
          assignment,
          priority: filters.priority,
          tag: filters.tags,
          assignee: filters.assignee,
        },
        signal,
      ),
    [conversations, page, q, status, assignment, filters],
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
    current.data ??
    result.data?.items.find((item) => item.id === selectedId) ??
    selected;
  return (
    <>
      <PageHeading
        eyebrow="HỘP THƯ CHUNG"
        title="Hộp hội thoại"
        description="Mỗi cuộc trò chuyện là cơ hội để thấu hiểu hơn."
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
            <AssignmentFilter
              value={assignment}
              onChange={(value) => {
                setAssignment(value);
                setPage(1);
              }}
            />
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
                    <strong>
                      {item.subject || "Hội thoại chưa có chủ đề"}{" "}
                      {item.unread && (
                        <span className="count-pill">Chưa đọc</span>
                      )}
                    </strong>
                    <span className="conversation-meta">
                      #{item.id.slice(0, 8)}
                      <span>
                        {new Intl.DateTimeFormat("vi-VN", {
                          day: "2-digit",
                          month: "2-digit",
                        }).format(new Date(item.updated_at))}
                      </span>
                    </span>
                    {Boolean(item.tags?.length) && (
                      <span className="keywords">
                        {item.tags?.map((tag) => (
                          <span key={tag}>{tag}</span>
                        ))}
                      </span>
                    )}
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
            current.reload();
          }}
        />
      )}
    </>
  );
}
