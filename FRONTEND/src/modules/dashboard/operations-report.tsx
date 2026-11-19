"use client";
import Link from "next/link";
import { useUser } from "@/shared/providers/workspace-provider";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { AnalyticsDashboard } from "./analytics-dashboard";
import { ExportLinks } from "../resources/workspace-controls";
import { ErrorState, Loading } from "@/shared/ui/primitives";

const dayFormat = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Ho_Chi_Minh",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});
const number = new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 1 });

export function OperationsReport() {
  const user = useUser();
  const { dashboard } = useServices();
  const [from, setFrom] = useState(() =>
    dayFormat.format(new Date(Date.now() - 29 * 86400000)),
  );
  const [to, setTo] = useState(() => dayFormat.format(new Date()));
  const loader = useCallback(
    (signal: AbortSignal) => dashboard.report(from, to, signal),
    [dashboard, from, to],
  );
  const report = useQuery(loader, "workspace-refresh");
  const rows = report.data
    ? [
        { ...report.data.total, id: "total", name: "Toàn đội" },
        ...report.data.agents,
      ]
    : [];
  return (
    <section className="panel">
      <div className="panel-header">
        <h2>Báo cáo hỗ trợ</h2>
        <div className="toolbar-controls">
          {user.role === "owner" && (
            <Link
              className="button secondary compact"
              href="/workflows?section=schedules#report-schedules"
            >
              Lịch gửi báo cáo
            </Link>
          )}
          <ExportLinks kind="reports" since={from} to={to} />
          <a
            className="button secondary compact"
            href={`/api/reports/pdf?${new URLSearchParams({ from, to })}`}
            download
          >
            Xuất PDF
          </a>
          <label>
            Từ ngày{" "}
            <input
              type="date"
              value={from}
              max={to}
              onChange={(event) => {
                if (event.target.value) setFrom(event.target.value);
              }}
            />
          </label>
          <label>
            Đến ngày{" "}
            <input
              type="date"
              value={to}
              min={from}
              onChange={(event) => {
                if (event.target.value) setTo(event.target.value);
              }}
            />
          </label>
        </div>
      </div>
      <AnalyticsDashboard from={from} to={to} />
      {report.error ? (
        <ErrorState message={report.error} retry={report.reload} />
      ) : report.loading ? (
        <Loading />
      ) : (
        <div className="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Nhân viên</th>
                <th>Phiếu mới</th>
                <th>Tồn hiện tại</th>
                <th>Phản hồi đầu (phút)</th>
                <th>Đã xử lý</th>
                <th>Xử lý (phút)</th>
                <th>Hài lòng / 5</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <tr key={row.id}>
                  <th scope="row">{row.name}</th>
                  <td>{row.opened}</td>
                  <td>{row.backlog}</td>
                  <td>
                    {row.responses
                      ? number.format(row.average_response_minutes)
                      : "—"}
                  </td>
                  <td>{row.resolved}</td>
                  <td>
                    {row.resolved
                      ? number.format(row.average_resolution_minutes)
                      : "—"}
                  </td>
                  <td>
                    {row.ratings
                      ? `${number.format(row.average_rating)} (${row.ratings})`
                      : "—"}
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
