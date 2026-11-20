"use client";
import Link from "next/link";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
import { errorMessage } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";
import {
  toggleSelection,
  useCollectionFilters,
} from "./use-collection-filters";
import type { ResourceKind, ResourceMap } from "@/shared/domain/models";
import {
  Avatar,
  Badge,
  EmptyState,
  ErrorState,
  Loading,
  PageHeading,
  Pagination,
  dateLabel,
} from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
import { ResourceEditor } from "./resource-editor";
import { ResourceDetails } from "./resource-details";
import { resourceModules } from "./registry";
import { BulkToolbar, ExportLinks, ViewControls } from "./workspace-controls";
import { AIKnowledgeSearch } from "../ai/assist";
import { Assignment, AssignmentFilter } from "../team/assignment";
import { ticketStatuses } from "./module-definition";
import styles from "./tickets-page.module.css";

export function ResourcePage<K extends Exclude<ResourceKind, "conversations">>({
  kind,
}: {
  kind: K;
}) {
  const services = useServices();
  const user = useUser();
  const definition = resourceModules[kind];
  const repository = services.resources[kind];
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
  } = useCollectionFilters(12);
  const [editor, setEditor] = useState<{ entity?: ResourceMap[K] } | null>(
    null,
  );
  const [actionError, setActionError] = useState("");
  const [notice, setNotice] = useState("");
  const [viewing, setViewing] = useState<ResourceMap[K] | null>(null);
  const loader = useCallback(
    (signal: AbortSignal) => repository.list(query, signal),
    [repository, query],
  );
  const result = useQuery(loader, "workspace-refresh");
  const canWrite = !definition.ownerOnly || user.role === "owner";
  const isTickets = kind === "tickets";
  const isAutomations = kind === "automations";
  const hasFilters = Boolean(
    q ||
    status ||
    assignment ||
    filters.priority ||
    filters.assignee ||
    filters.tags.length,
  );
  return (
    <>
      <PageHeading
        eyebrow="KHÔNG GIAN LÀM VIỆC"
        title={definition.title}
        description={definition.description}
      >
        <button
          className="button secondary"
          onClick={result.reload}
          aria-label="Làm mới"
        >
          <Icon name="refresh" size={17} />
        </button>
        {(kind === "tickets" || kind === "customers") && (
          <ExportLinks kind={kind} />
        )}
        {isAutomations && canWrite && (
          <Link className="button secondary" href="/workflows">
            <Icon name="bolt" size={17} /> Workflow nhiều bước
          </Link>
        )}
        {canWrite && (
          <button className="button primary" onClick={() => setEditor({})}>
            <Icon name="plus" size={18} />
            Tạo {definition.singular}
          </button>
        )}
      </PageHeading>
      {actionError && <ErrorState message={actionError} />}
      {notice && (
        <div className="notice" role="status">
          <Icon name="check" size={17} />
          {notice}
          <button aria-label="Ẩn thông báo" onClick={() => setNotice("")}>
            <Icon name="close" size={16} />
          </button>
        </div>
      )}
      {kind === "knowledge" && (
        <details className="panel knowledge-tools">
          <summary>Tìm trong kho bằng AI</summary>
          <AIKnowledgeSearch
            onSelect={async (id) => {
              try {
                const item = await repository.get(id);
                setViewing(item);
              } catch (error) {
                setActionError(errorMessage(error));
              }
            }}
          />
        </details>
      )}
      {kind === "knowledge" && (
        <div className="subtle-note">
          <Icon name="info" size={17} />
          Phản hồi tự động tìm nội dung theo từ khóa trong các bài đang bật. Khi
          chưa có câu trả lời phù hợp, nhân viên sẽ cần hỗ trợ tiếp.
        </div>
      )}
      {!canWrite && (
        <div className="subtle-note">
          <Icon name="shield" size={17} />
          Bạn có quyền xem. Chỉ quản trị viên được tạo và chỉnh sửa nội dung
          này.
        </div>
      )}
      <section
        className={`panel resource-panel ${isTickets ? styles.tickets : isAutomations ? styles.automations : ""}`}
      >
        <div className="panel-toolbar">
          <div className="tab-label">
            {(isTickets || isAutomations) && (
              <Icon name={definition.icon} size={18} />
            )}
            {isTickets
              ? "Danh sách phiếu"
              : isAutomations
                ? "Quy tắc từ khóa"
                : `Tất cả ${definition.singular}`}
            <span className="count-pill">{result.data?.total ?? "—"}</span>
          </div>
          <div className="toolbar-controls">
            <label className="search-field">
              <Icon name="search" size={17} />
              <input
                aria-label={`Tìm ${definition.singular}`}
                placeholder={`Tìm ${definition.singular}…`}
                value={q}
                onChange={(event) => {
                  setQ(event.target.value);
                  setPage(1);
                }}
              />
            </label>
            {kind === "tickets" && (
              <AssignmentFilter
                value={assignment}
                onChange={(value) => {
                  setAssignment(value);
                  setPage(1);
                }}
              />
            )}
            {kind === "tickets" && (
              <select
                aria-label="Lọc trạng thái"
                value={status}
                onChange={(event) => {
                  setStatus(event.target.value);
                  setPage(1);
                }}
              >
                <option value="">Mọi trạng thái</option>
                {ticketStatuses.map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </select>
            )}
          </div>
        </div>
        {kind === "tickets" && (
          <div className="workspace-filter-bar">
            <ViewControls
              current={{
                ...filters,
                kind: "tickets",
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
              kind="tickets"
              ids={checked}
              clear={() => setChecked([])}
              changed={() => {
                setPage(1);
                result.reload();
              }}
            />
          </div>
        )}
        {result.error ? (
          <ErrorState message={result.error} retry={result.reload} />
        ) : result.loading ? (
          <Loading />
        ) : !result.data?.items.length ? (
          <EmptyState
            icon={definition.icon}
            title={
              hasFilters
                ? "Không tìm thấy kết quả"
                : `Chưa có ${definition.singular}`
            }
            description={
              hasFilters
                ? "Thử thay đổi từ khóa hoặc bộ lọc của bạn."
                : `Thêm ${definition.singular} đầu tiên để bắt đầu.`
            }
          >
            {canWrite && !hasFilters && (
              <button
                className="button secondary"
                onClick={() => setEditor({})}
              >
                <Icon name="plus" size={16} />
                Tạo {definition.singular}
              </button>
            )}
          </EmptyState>
        ) : (
          <div
            className="table-scroll"
            tabIndex={isTickets ? 0 : undefined}
            role={isTickets ? "region" : undefined}
            aria-label={isTickets ? "Danh sách phiếu hỗ trợ" : undefined}
          >
            <table>
              <thead>
                <tr>
                  {kind === "tickets" && (
                    <th>
                      <input
                        type="checkbox"
                        aria-label="Chọn tất cả phiếu trên trang"
                        checked={Boolean(
                          result.data?.items.length &&
                          result.data.items.every((item) =>
                            checked.includes(item.id),
                          ),
                        )}
                        onChange={(event) => {
                          const pageIDs =
                            result.data?.items.map((item) => item.id) ?? [];
                          setChecked((ids) =>
                            event.target.checked
                              ? [...new Set([...ids, ...pageIDs])]
                              : ids.filter((id) => !pageIDs.includes(id)),
                          );
                        }}
                      />
                    </th>
                  )}
                  <th>
                    {kind === "customers"
                      ? "Khách hàng"
                      : kind === "automations"
                        ? "Quy tắc"
                        : "Tiêu đề"}
                  </th>
                  <th>
                    {kind === "customers"
                      ? "Liên hệ"
                      : kind === "tickets"
                        ? "Độ ưu tiên"
                        : kind === "automations"
                          ? "Từ khóa kích hoạt"
                          : "Nội dung"}
                  </th>
                  <th>
                    {kind === "customers" ? "Ngày tham gia" : "Trạng thái"}
                  </th>
                  <th>
                    {kind === "customers"
                      ? ""
                      : isTickets
                        ? "Cập nhật và SLA"
                        : "Cập nhật"}
                  </th>
                  <th>
                    <span className={isTickets ? undefined : "sr-only"}>
                      {isTickets ? "Phụ trách và thao tác" : "Thao tác"}
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {result.data.items.map((item) => (
                  <tr
                    key={item.id}
                    data-selected={
                      isTickets && checked.includes(item.id) ? "" : undefined
                    }
                  >
                    {kind === "tickets" && (
                      <td>
                        <input
                          type="checkbox"
                          aria-label={`Chọn phiếu ${item.id.slice(0, 8)}`}
                          checked={checked.includes(item.id)}
                          onChange={() =>
                            setChecked((ids) => toggleSelection(ids, item.id))
                          }
                        />
                      </td>
                    )}
                    <td>
                      <div className="entity-cell">
                        {kind === "customers" ? (
                          <Avatar name={"name" in item ? item.name : ""} />
                        ) : (
                          <span className={`entity-icon ${kind}`}>
                            <Icon name={definition.icon} size={19} />
                          </span>
                        )}
                        <div>
                          {kind === "tickets" && "subject" in item ? (
                            <Link
                              className="text-link"
                              href={`/tickets/${encodeURIComponent(item.id)}`}
                            >
                              {item.subject}
                            </Link>
                          ) : (
                            <strong>
                              {"name" in item
                                ? item.name
                                : "subject" in item
                                  ? item.subject
                                  : ""}
                            </strong>
                          )}
                          <small>#{item.id.slice(0, 8).toUpperCase()}</small>
                          {"tags" in item && Boolean(item.tags?.length) && (
                            <div className="keywords">
                              {item.tags?.map((tag) => (
                                <span key={tag}>{tag}</span>
                              ))}
                            </div>
                          )}
                        </div>
                      </div>
                    </td>
                    <td>
                      {kind === "customers" ? (
                        <div className="contact-cell">
                          <span>
                            {("email" in item && item.email) || "Chưa có email"}
                          </span>
                          <small>
                            {("phone" in item && item.phone) ||
                              "Chưa có số điện thoại"}
                          </small>
                        </div>
                      ) : "priority" in item ? (
                        <div className={styles.priority}>
                          <Badge value={item.priority} />{" "}
                          {"overdue" in item && item.overdue && (
                            <strong role="status">Quá hạn SLA</strong>
                          )}
                          {"sla_risk_reason" in item &&
                            item.sla_risk_reason && (
                              <strong
                                title={item.sla_risk_reason}
                                role="status"
                              >
                                Nguy cơ trễ SLA
                              </strong>
                            )}
                          {"due_soon" in item && item.due_soon && (
                            <strong role="status">Sắp đến hạn SLA</strong>
                          )}
                        </div>
                      ) : "keywords" in item ? (
                        <div className="keywords">
                          {item.keywords.map((keyword) => (
                            <span key={keyword}>{keyword}</span>
                          ))}
                        </div>
                      ) : "content" in item ? (
                        <p className="cell-excerpt">{item.content}</p>
                      ) : (
                        "—"
                      )}
                    </td>
                    <td>
                      {kind === "customers" ? (
                        dateLabel(item.created_at)
                      ) : "status" in item && item.status === "draft" ? (
                        <span className="badge badge-closed">Bản nháp</span>
                      ) : "status" in item && item.status ? (
                        <Badge value={item.status} />
                      ) : "enabled" in item ? (
                        <span
                          className={`badge ${item.enabled ? "badge-open" : "badge-closed"}`}
                        >
                          <span />
                          {item.enabled ? "Đang bật" : "Đã tắt"}
                        </span>
                      ) : (
                        "—"
                      )}
                    </td>
                    <td>
                      {kind !== "customers" && (
                        <time dateTime={item.updated_at}>
                          {dateLabel(item.updated_at)}
                        </time>
                      )}
                      {"response_due_at" in item && item.response_due_at && (
                        <div className={styles.deadlines}>
                          <small>
                            Phản hồi{" "}
                            <time dateTime={item.response_due_at}>
                              {new Date(item.response_due_at).toLocaleString(
                                "vi-VN",
                              )}
                            </time>
                          </small>
                          {item.resolution_due_at && (
                            <small>
                              Xử lý{" "}
                              <time dateTime={item.resolution_due_at}>
                                {new Date(
                                  item.resolution_due_at,
                                ).toLocaleString("vi-VN")}
                              </time>
                            </small>
                          )}
                        </div>
                      )}
                    </td>
                    <td>
                      {"priority" in item &&
                        "response_due_at" in item &&
                        item.response_due_at && (
                          <div className={styles.actions}>
                            {!item.conversation_id &&
                              !item.first_response_at &&
                              item.status !== "resolved" && (
                                <button
                                  type="button"
                                  className="button secondary compact"
                                  onClick={async () => {
                                    setActionError("");
                                    try {
                                      await repository.recordResponse(item.id);
                                      result.reload();
                                    } catch (error) {
                                      setActionError(errorMessage(error));
                                    }
                                  }}
                                >
                                  Ghi nhận đã phản hồi
                                </button>
                              )}
                            {item.escalated_at ? (
                              <p className={styles.escalated}>Đã chuyển cấp</p>
                            ) : (
                              item.overdue && (
                                <button
                                  type="button"
                                  className="button secondary compact"
                                  onClick={async () => {
                                    setActionError("");
                                    try {
                                      await repository.escalate(item.id);
                                      result.reload();
                                    } catch (error) {
                                      setActionError(errorMessage(error));
                                    }
                                  }}
                                >
                                  Chuyển cấp
                                </button>
                              )
                            )}
                          </div>
                        )}
                      <div
                        className={
                          isTickets
                            ? styles.rowControls
                            : isAutomations
                              ? styles.ruleControls
                              : undefined
                        }
                      >
                        {"priority" in item && (
                          <Assignment
                            assignee={item.assignee_id}
                            save={async (id, claim) => {
                              await repository.assign(item.id, id, claim);
                              result.reload();
                            }}
                          />
                        )}
                        {canWrite ? (
                          <button
                            className={
                              isAutomations
                                ? "button secondary compact"
                                : "icon-button"
                            }
                            title="Chỉnh sửa"
                            aria-label={`Chỉnh sửa ${"name" in item ? item.name : "subject" in item ? item.subject : ""}`}
                            onClick={() => setEditor({ entity: item })}
                          >
                            <Icon name="edit" size={17} />
                            {isAutomations && "Chỉnh sửa"}
                          </button>
                        ) : (
                          <button
                            className="button secondary compact"
                            onClick={() => setViewing(item)}
                          >
                            Xem chi tiết
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {result.data && (
          <Pagination
            page={page}
            total={result.data.total}
            limit={12}
            onChange={setPage}
          />
        )}
      </section>
      {viewing && (
        <ResourceDetails
          kind={kind}
          entity={viewing}
          onClose={() => setViewing(null)}
        />
      )}
      {editor && (
        <ResourceEditor
          kind={kind}
          repository={repository}
          entity={editor.entity}
          onClose={() => setEditor(null)}
          onSaved={() => {
            setEditor(null);
            setNotice("Đã lưu thông tin thành công.");
            result.reload();
          }}
        />
      )}
    </>
  );
}
