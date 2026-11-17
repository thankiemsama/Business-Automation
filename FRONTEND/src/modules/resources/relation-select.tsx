"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { ErrorState } from "@/shared/ui/primitives";
export function RelationSelect({
  id,
  kind,
  value,
  onChange,
  required,
  disabled,
}: {
  id: string;
  kind: "customers" | "conversations";
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  disabled?: boolean;
}) {
  const services = useServices();
  const [q, setQ] = useState("");
  const [page, setPage] = useState(1);
  const loader = useCallback(
    async (signal: AbortSignal) => {
      const result = await services[kind].list({ page, limit: 20, q }, signal);
      return {
        ...result,
        items: result.items.map((item) => ({
          id: item.id,
          label:
            "name" in item
              ? item.name
              : item.subject || `Hội thoại ${item.id.slice(0, 8)}`,
        })),
      };
    },
    [services, kind, page, q],
  );
  const result = useQuery(loader);
  return (
    <div className="relation-select">
      {!disabled && (
        <input
          aria-label="Tìm lựa chọn"
          placeholder={
            kind === "customers"
              ? "Tìm theo tên hoặc email…"
              : "Tìm theo chủ đề…"
          }
          value={q}
          onChange={(event) => {
            setQ(event.target.value);
            setPage(1);
          }}
        />
      )}
      <select
        id={id}
        required={required}
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      >
        <option value="">
          {result.loading ? "Đang tải…" : "Chọn một mục"}
        </option>
        {value && !result.data?.items.some((item) => item.id === value) && (
          <option value={value}>Đã chọn · {value.slice(0, 10)}</option>
        )}
        {result.data?.items.map((item) => (
          <option key={item.id} value={item.id}>
            {item.label}
          </option>
        ))}
      </select>
      {result.error && (
        <ErrorState message={result.error} retry={result.reload} />
      )}
      {!disabled && result.data && result.data.total > 20 && (
        <div className="relation-pages">
          <button
            type="button"
            disabled={page <= 1}
            onClick={() => setPage(page - 1)}
          >
            ← Trước
          </button>
          <span>
            {page} / {Math.ceil(result.data.total / 20)}
          </span>
          <button
            type="button"
            disabled={page * 20 >= result.data.total}
            onClick={() => setPage(page + 1)}
          >
            Sau →
          </button>
        </div>
      )}
    </div>
  );
}
