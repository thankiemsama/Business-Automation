"use client";

import { useCallback, useRef, useState } from "react";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";
import { useServices } from "@/shared/providers/services-provider";
import { loadFields } from "./custom-values";

type Batch = {
  id: string;
  created_at: string;
  total: number;
  applied: number;
  skipped: number;
  failed: number;
  pending: number;
};

const api = new FetchHttpClient();
const path = "operations/field-extraction";
const loadBatches = (signal: AbortSignal) =>
  api.request<Batch[]>(path, { signal });
const displayTime = new Intl.DateTimeFormat("vi-VN", {
  dateStyle: "short",
  timeStyle: "short",
  timeZone: "Asia/Ho_Chi_Minh",
});

function toggle(selected: string[], id: string) {
  return selected.includes(id)
    ? selected.filter((item) => item !== id)
    : [...selected, id];
}

export function FieldExtractionPanel() {
  const { tickets } = useServices();
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [ticketIDs, setTicketIDs] = useState<string[]>([]);
  const [fieldIDs, setFieldIDs] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const retry = useRef<{ selection: string; id: string } | null>(null);
  const loadTickets = useCallback(
    (signal: AbortSignal) =>
      tickets.list({ q: query, page, limit: 10 }, signal),
    [tickets, query, page],
  );
  const list = useQuery(loadTickets);
  const fields = useQuery(loadFields);
  const batches = useQuery(loadBatches, undefined, 10000);
  const activeFields = fields.data?.filter(
    (field) => field.kind === "tickets" && field.enabled,
  );

  async function submit() {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const selection = JSON.stringify({ ticketIDs, fieldIDs });
      if (retry.current?.selection !== selection) {
        retry.current = { selection, id: crypto.randomUUID() };
      }
      await api.request(path, {
        method: "POST",
        body: JSON.stringify({
          request_id: retry.current.id,
          ticket_ids: ticketIDs,
          field_ids: fieldIDs,
        }),
      });
      retry.current = null;
      setTicketIDs([]);
      setNotice("Đã xếp hàng đợt trích xuất.");
      batches.reload();
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel team-form form-stack">
      <h2>Trích xuất trường tùy chỉnh từ phiếu</h2>
      <p>AI đọc tiêu đề và mô tả, rồi điền các trường đang trống.</p>
      <fieldset disabled={busy}>
        <legend>Trường tùy chỉnh · {fieldIDs.length}/10</legend>
        {activeFields?.length === 0 && (
          <p>Chưa có trường đang bật cho phiếu.</p>
        )}
        {activeFields?.map((field) => (
          <label key={field.id} className="check-row">
            <input
              type="checkbox"
              checked={fieldIDs.includes(field.id)}
              disabled={fieldIDs.length >= 10 && !fieldIDs.includes(field.id)}
              onChange={() => setFieldIDs((value) => toggle(value, field.id))}
            />
            {field.name}
          </label>
        ))}
      </fieldset>
      <fieldset disabled={busy}>
        <legend>Phiếu hỗ trợ · {ticketIDs.length}/50</legend>
        <input
          aria-label="Tìm phiếu hỗ trợ"
          placeholder="Tìm theo tiêu đề"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setPage(1);
          }}
        />
        {list.data?.items.length === 0 && <p>Không có phiếu phù hợp.</p>}
        {list.data?.items.map((ticket) => (
          <label key={ticket.id} className="check-row">
            <input
              type="checkbox"
              checked={ticketIDs.includes(ticket.id)}
              disabled={
                ticketIDs.length >= 50 && !ticketIDs.includes(ticket.id)
              }
              onChange={() => setTicketIDs((value) => toggle(value, ticket.id))}
            />
            {ticket.subject}
          </label>
        ))}
        <div className="toolbar-controls">
          <button
            type="button"
            className="text-link"
            disabled={page === 1}
            onClick={() => setPage(page - 1)}
          >
            Trang trước
          </button>
          <span>Trang {page}</span>
          <button
            type="button"
            className="text-link"
            disabled={!list.data || page * 10 >= list.data.total}
            onClick={() => setPage(page + 1)}
          >
            Trang sau
          </button>
          {ticketIDs.length > 0 && (
            <button
              type="button"
              className="text-link"
              onClick={() => setTicketIDs([])}
            >
              Bỏ chọn phiếu
            </button>
          )}
        </div>
      </fieldset>
      <button
        type="button"
        className="button primary"
        disabled={busy || !ticketIDs.length || !fieldIDs.length}
        onClick={() => void submit()}
      >
        {busy ? "Đang xếp hàng…" : "Bắt đầu trích xuất"}
      </button>
      {(error || fields.error || list.error) && (
        <p role="alert">{error || fields.error || list.error}</p>
      )}
      {notice && <p role="status">{notice}</p>}
      <h3>Tiến độ các đợt trích xuất</h3>
      {batches.error && <p role="alert">{batches.error}</p>}
      {batches.data?.length === 0 && <p>Chưa có đợt trích xuất.</p>}
      {batches.data?.map((batch) => (
        <div key={batch.id} className="form-stack">
          <span>{displayTime.format(new Date(batch.created_at))}</span>
          <progress
            aria-label="Số phiếu đã xử lý"
            max={batch.total}
            value={batch.total - batch.pending}
          />
          <span>
            Đã điền {batch.applied} · Bỏ qua {batch.skipped} · Lỗi{" "}
            {batch.failed} · Đang chờ {batch.pending}
          </span>
        </div>
      ))}
    </section>
  );
}
