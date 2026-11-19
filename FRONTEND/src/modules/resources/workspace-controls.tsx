"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { emptyFilters } from "./use-collection-filters";
import { errorMessage } from "@/shared/api/http-client";
import {
  parseTags,
  type SavedView,
  type WorkKind,
} from "@/shared/api/workspace";

function useMembers() {
  const { auth } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => auth.members(signal),
    [auth],
  );
  return useQuery(load, "workspace-refresh");
}

export function ViewControls({
  current,
  onApply,
}: {
  current: SavedView;
  onApply: (view: SavedView) => void;
}) {
  const { workspace } = useServices();
  const load = useCallback(
    (signal: AbortSignal) => workspace.views(signal),
    [workspace],
  );
  const views = useQuery(load, "workspace-refresh");
  const members = useMembers();
  const [selected, setSelected] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const filterCount =
    current.tags.length +
    Number(Boolean(current.priority)) +
    Number(Boolean(current.assignee));
  function resetView() {
    setSelected("");
    setName("");
    onApply({
      ...current,
      ...emptyFilters,
      search: "",
      status: "",
      assignment: "",
    });
  }
  async function change(action: () => Promise<unknown>) {
    setBusy(true);
    setError("");
    try {
      await action();
      views.reload();
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <details className="workspace-filters">
      <summary>
        Bộ lọc và chế độ xem đã lưu
        {current.kind === "tickets" && filterCount > 0 && (
          <span className="count-pill">{filterCount}</span>
        )}
      </summary>
      <div className="form-stack">
        <form
          className="form-stack"
          key={JSON.stringify({
            tags: current.tags,
            priority: current.priority,
            assignee: current.assignee,
          })}
          onSubmit={(event) => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            onApply({
              ...current,
              tags: parseTags(String(data.get("tags") || "")),
              priority: String(data.get("priority") || ""),
              assignee: String(data.get("assignee") || ""),
            });
          }}
        >
          <label>
            Nhãn
            <input
              name="tags"
              defaultValue={current.tags.join(", ")}
              placeholder="Ví dụ: vip, hoàn tiền"
            />
          </label>
          {current.kind === "tickets" && (
            <label>
              Ưu tiên
              <select name="priority" defaultValue={current.priority}>
                <option value="">Mọi ưu tiên</option>
                <option value="low">Thấp</option>
                <option value="normal">Bình thường</option>
                <option value="high">Cao</option>
                <option value="urgent">Khẩn cấp</option>
              </select>
            </label>
          )}
          <label>
            Người phụ trách
            <select name="assignee" defaultValue={current.assignee}>
              <option value="">Mọi người</option>
              {members.data?.map((member) => (
                <option key={member.id} value={member.id}>
                  {member.name}
                </option>
              ))}
            </select>
          </label>
          <button className="button secondary compact">Áp dụng bộ lọc</button>
          {current.kind === "tickets" &&
            Boolean(
              filterCount ||
              current.search ||
              current.status ||
              current.assignment,
            ) && (
              <button
                className="button secondary compact"
                type="button"
                onClick={resetView}
              >
                Xóa bộ lọc
              </button>
            )}
        </form>
        <div className="saved-view-controls">
          <select
            aria-label="Chế độ xem đã lưu"
            value={selected}
            onChange={(event) => {
              const id = event.target.value;
              setSelected(id);
              const view = views.data?.find((item) => item.id === id);
              if (view) {
                setName(view.name);
                onApply(view);
              } else {
                resetView();
              }
            }}
          >
            <option value="">Chế độ xem mặc định</option>
            {views.data
              ?.filter((view) => view.kind === current.kind)
              .map((view) => (
                <option key={view.id} value={view.id}>
                  {view.name}
                </option>
              ))}
          </select>
          <label>
            Tên chế độ xem
            <input
              value={name}
              maxLength={80}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          <div className="toolbar-controls">
            <button
              type="button"
              className="button secondary compact"
              disabled={busy || !name.trim()}
              onClick={() =>
                void change(async () => {
                  const view = await workspace.saveView({
                    ...current,
                    name,
                    id: undefined,
                  });
                  setSelected(view.id ?? "");
                })
              }
            >
              Lưu mới
            </button>
            {selected && (
              <>
                <button
                  type="button"
                  className="button secondary compact"
                  disabled={busy || !name.trim()}
                  onClick={() =>
                    void change(() =>
                      workspace.saveView({ ...current, name, id: selected }),
                    )
                  }
                >
                  Cập nhật
                </button>
                <button
                  type="button"
                  className="button secondary compact"
                  disabled={busy}
                  onClick={() =>
                    void change(async () => {
                      await workspace.removeView(selected);
                      setSelected("");
                      setName("");
                    })
                  }
                >
                  Xóa
                </button>
              </>
            )}
          </div>
        </div>
        {(error || views.error || members.error) && (
          <p role="alert">{error || views.error || members.error}</p>
        )}
      </div>
    </details>
  );
}

export function BulkToolbar({
  kind,
  ids,
  clear,
  changed,
}: {
  kind: WorkKind;
  ids: string[];
  clear: () => void;
  changed: () => void;
}) {
  const { workspace } = useServices();
  const members = useMembers();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  async function run(action: string, assignee = "") {
    setError("");
    setNotice("");
    setBusy(true);
    try {
      const result = await workspace.bulk(kind, ids, action, assignee);
      const label = kind === "tickets" ? "phiếu" : "hội thoại";
      setNotice(`Đã cập nhật ${result.updated} ${label}.`);
      clear();
      changed();
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="bulk-toolbar">
      {ids.length > 0 && (
        <>
          <strong>Đã chọn {ids.length}</strong>
          <button
            type="button"
            className="button secondary compact"
            disabled={busy || ids.length > 100}
            onClick={() => void run(kind === "tickets" ? "resolve" : "close")}
          >
            {kind === "tickets" ? "Giải quyết đã chọn" : "Đóng đã chọn"}
          </button>
          <select
            aria-label="Phân công các mục đã chọn"
            value=""
            disabled={busy || ids.length > 100}
            onChange={(event) => {
              const value = event.target.value;
              if (value) void run("assign", value === "unassign" ? "" : value);
            }}
          >
            <option value="">Phân công đã chọn</option>
            <option value="unassign">Bỏ phân công</option>
            {members.data
              ?.filter((member) => !member.disabled)
              .map((member) => (
                <option key={member.id} value={member.id}>
                  {member.name}
                </option>
              ))}
          </select>
          <button
            type="button"
            className="text-link"
            disabled={busy}
            onClick={clear}
          >
            Bỏ chọn
          </button>
        </>
      )}
      {ids.length > 100 && (
        <span role="alert">Chọn tối đa 100 mục mỗi lần.</span>
      )}
      {error && <p role="alert">{error}</p>}
      {notice && <p role="status">{notice}</p>}
    </div>
  );
}

export function ExportLinks({
  kind,
  since,
  to,
}: {
  kind: "reports" | "tickets" | "customers";
  since?: string;
  to?: string;
}) {
  return (
    <div className="toolbar-controls">
      {["csv", "xlsx"].map((format) => {
        const query = new URLSearchParams({ kind, format });
        if (since) query.set("since", since);
        if (to) query.set("to", to);
        return (
          <a
            key={format}
            className="button secondary compact"
            href={`/api/platform/reports/export?${query}`}
            download
          >
            Xuất {format === "xlsx" ? "Excel" : "CSV"}
          </a>
        );
      })}
    </div>
  );
}
