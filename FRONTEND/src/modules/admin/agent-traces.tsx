"use client";

import { FetchHttpClient } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";
import { ErrorState, Loading } from "@/shared/ui/primitives";

type Trace = {
  run_id: string;
  tool: string;
  outcome: string;
  model?: string;
  policy: string;
  at: string;
};
const api = new FetchHttpClient();
const load = (signal: AbortSignal) =>
  api.request<Trace[]>("ai/ticket-agent/traces", { signal });
const date = new Intl.DateTimeFormat("vi-VN", {
  dateStyle: "short",
  timeStyle: "short",
  timeZone: "Asia/Ho_Chi_Minh",
});
const outcomes: Record<string, string> = {
  completed: "Đã hoàn thành",
  proposed: "Đã đề xuất",
  handed_off: "Đã chuyển xử lý",
  applied: "Đã thực hiện",
  auto_applied: "Tự thực hiện",
  failed: "Lỗi",
};

export function AgentTraces() {
  const result = useQuery(load);
  return (
    <section className="panel team-form form-stack">
      <h2>Dấu vết công cụ AI agent</h2>
      <button
        type="button"
        className="button secondary compact"
        onClick={result.reload}
        disabled={result.loading}
      >
        Làm mới
      </button>
      {result.error && (
        <ErrorState message={result.error} retry={result.reload} />
      )}
      {result.loading && <Loading />}
      {result.data?.length === 0 && (
        <p>Chưa có dấu vết trong 7 ngày gần đây.</p>
      )}
      {!!result.data?.length && (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Thời điểm (ICT)</th>
                <th>Lượt chạy</th>
                <th>Công cụ</th>
                <th>Kết quả</th>
                <th>Model / chính sách</th>
              </tr>
            </thead>
            <tbody>
              {result.data.map((row, i) => (
                <tr key={`${row.run_id}:${row.at}:${row.tool}:${i}`}>
                  <td>{date.format(new Date(row.at))}</td>
                  <td>
                    <code title={row.run_id}>{row.run_id.slice(0, 10)}</code>
                  </td>
                  <td>
                    <code>{row.tool}</code>
                  </td>
                  <td>{outcomes[row.outcome] ?? row.outcome}</td>
                  <td>
                    {row.model ?? "—"} · <code>{row.policy}</code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
