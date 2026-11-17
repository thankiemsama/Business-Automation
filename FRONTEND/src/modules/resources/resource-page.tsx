"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
import { useQuery } from "@/shared/hooks/use-query";
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
import { ticketStatuses } from "./module-definition";

export function ResourcePage<K extends Exclude<ResourceKind, "conversations">>({
  kind,
}: {
  kind: K;
}) {
  const services = useServices();
  const user = useUser();
  const definition = resourceModules[kind];
  const repository = services.resources[kind];
  const [q, setQ] = useState("");
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const [editor, setEditor] = useState<{ entity?: ResourceMap[K] } | null>(
    null,
  );
  const [notice, setNotice] = useState("");
  const [viewing, setViewing] = useState<ResourceMap[K] | null>(null);
  const loader = useCallback(
    (signal: AbortSignal) =>
      repository.list({ page, limit: 12, q, status }, signal),
    [repository, page, q, status],
  );
  const result = useQuery(loader);
  const canWrite = !definition.ownerOnly || user.role === "owner";
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
        {canWrite && (
          <button className="button primary" onClick={() => setEditor({})}>
            <Icon name="plus" size={18} />
            Tạo {definition.singular}
          </button>
        )}
      </PageHeading>
      {notice && (
        <div className="notice" role="status">
          <Icon name="check" size={17} />
          {notice}
          <button aria-label="Ẩn thông báo" onClick={() => setNotice("")}>
            <Icon name="close" size={16} />
          </button>
        </div>
      )}
      {kind === "automations" && (
        <div className="module-banner">
          <span className="banner-icon">
            <Icon name="bolt" size={24} />
          </span>
          <div>
            <strong>Đúng tín hiệu. Đúng thời điểm.</strong>
            <p>
              Khi tin nhắn mới của khách hàng chứa từ khóa, một phiếu hỗ trợ
              được tạo tự động.
            </p>
          </div>
          <span className="banner-pill">TỪ KHÓA → PHIẾU HỖ TRỢ</span>
        </div>
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
      <section className="panel resource-panel">
        <div className="panel-toolbar">
          <div className="tab-label">
            Tất cả {definition.singular}
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
        {result.error ? (
          <ErrorState message={result.error} retry={result.reload} />
        ) : result.loading ? (
          <Loading />
        ) : !result.data?.items.length ? (
          <EmptyState
            icon={definition.icon}
            title={
              q || status
                ? "Không tìm thấy kết quả"
                : `Chưa có ${definition.singular}`
            }
            description={
              q || status
                ? "Thử thay đổi từ khóa hoặc bộ lọc của bạn."
                : `Thêm ${definition.singular} đầu tiên để bắt đầu.`
            }
          >
            {canWrite && !q && !status && (
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
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
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
                  <th>{kind === "customers" ? "" : "Cập nhật"}</th>
                  <th>
                    <span className="sr-only">Thao tác</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {result.data.items.map((item) => (
                  <tr key={item.id}>
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
                          <strong>
                            {"name" in item
                              ? item.name
                              : "subject" in item
                                ? item.subject
                                : ""}
                          </strong>
                          <small>#{item.id.slice(0, 8).toUpperCase()}</small>
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
                        <Badge value={item.priority} />
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
                      ) : "status" in item ? (
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
                      {kind !== "customers" && dateLabel(item.updated_at)}
                    </td>
                    <td>
                      {canWrite ? (
                        <button
                          className="icon-button"
                          aria-label={`Chỉnh sửa ${"name" in item ? item.name : "subject" in item ? item.subject : ""}`}
                          onClick={() => setEditor({ entity: item })}
                        >
                          <Icon name="edit" size={17} />
                        </button>
                      ) : (
                        <button
                          className="button secondary compact"
                          onClick={() => setViewing(item)}
                        >
                          Xem chi tiết
                        </button>
                      )}
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
