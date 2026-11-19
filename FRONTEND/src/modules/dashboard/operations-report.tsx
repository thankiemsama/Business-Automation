"use client";
import Link from "next/link";
import { useUser } from "@/shared/providers/workspace-provider";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { AnalyticsDashboard } from "./analytics-dashboard";
import { ExportLinks } from "../resources/workspace-controls";
import { ErrorState, Loading } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
import styles from "./operations-report.module.css";

const dayFormat = new Intl.DateTimeFormat("en-CA", {
  timeZone: "Asia/Ho_Chi_Minh",
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});
const number = new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 1 });

function recentRange(days: number) {
  const now = Date.now();
  return {
    from: dayFormat.format(new Date(now - (days - 1) * 86_400_000)),
    to: dayFormat.format(new Date(now)),
  };
}

export function OperationsReport() {
  const user = useUser();
  const { dashboard } = useServices();
  const [{ from, to }, setRange] = useState(() => recentRange(30));
  const loader = useCallback(
    (signal: AbortSignal) => dashboard.report(from, to, signal),
    [dashboard, from, to],
  );
  const report = useQuery(loader, "workspace-refresh");
  const total = report.data?.total;
  const metrics = [
    { label: "Phiếu mới", value: total?.opened, icon: "ticket" },
    { label: "Đã xử lý", value: total?.resolved, icon: "check" },
    { label: "Tồn hiện tại", value: total?.backlog, icon: "grid" },
    {
      label: "Hài lòng / 5",
      value: total?.ratings ? total.average_rating : undefined,
      icon: "chat",
    },
  ] as const;
  const rows = report.data
    ? [
        { ...report.data.total, id: "total", name: "Toàn đội" },
        ...report.data.agents,
      ]
    : [];
  return (
    <section className={styles.report} aria-labelledby="support-report-heading">
      <div className={`panel ${styles.controls}`}>
        <div className={styles.heading}>
          <h2 id="support-report-heading">Báo cáo hỗ trợ</h2>
          <div className={styles.exports}>
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
              <Icon name="download" size={15} /> Xuất PDF
            </a>
          </div>
        </div>
        <div className={styles.dates}>
          <div
            className={styles.presets}
            role="group"
            aria-label="Khoảng ngày nhanh"
          >
            {[7, 30].map((days) => {
              const range = recentRange(days);
              return (
                <button
                  type="button"
                  key={days}
                  aria-pressed={from === range.from && to === range.to}
                  onClick={() => setRange(recentRange(days))}
                >
                  {days} ngày qua
                </button>
              );
            })}
          </div>
          <label>
            Từ ngày
            <input
              type="date"
              value={from}
              max={to}
              onChange={(event) => {
                const value = event.target.value;
                if (value && value <= to) setRange({ from: value, to });
              }}
            />
          </label>
          <label>
            Đến ngày
            <input
              type="date"
              value={to}
              min={from}
              onChange={(event) => {
                const value = event.target.value;
                if (value && value >= from) setRange({ from, to: value });
              }}
            />
          </label>
        </div>
      </div>
      <dl
        className={styles.metrics}
        aria-label="Chỉ số toàn đội"
        aria-busy={report.loading}
      >
        {metrics.map(({ label, value, icon }) => (
          <div key={label} className={`panel ${styles.metric}`}>
            <dt>
              <span className={styles.metricIcon}>
                <Icon name={icon} size={19} />
              </span>
              {label}
            </dt>
            <dd>{value === undefined ? "—" : number.format(value)}</dd>
          </div>
        ))}
      </dl>
      <AnalyticsDashboard from={from} to={to} />
      <section
        className={`panel ${styles.card}`}
        aria-labelledby="report-agents-heading"
      >
        <h3 id="report-agents-heading">Kết quả theo nhân viên</h3>
        {report.error ? (
          <ErrorState message={report.error} retry={report.reload} />
        ) : report.loading ? (
          <Loading />
        ) : (
          <div
            className="table-scroll"
            role="region"
            tabIndex={0}
            aria-labelledby="report-agents-heading"
          >
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
                  <tr
                    key={row.id}
                    data-total={row.id === "total" ? "" : undefined}
                  >
                    <th scope="row">{row.name}</th>
                    <td>{number.format(row.opened)}</td>
                    <td>{number.format(row.backlog)}</td>
                    <td>
                      {row.responses
                        ? number.format(row.average_response_minutes)
                        : "—"}
                    </td>
                    <td>{number.format(row.resolved)}</td>
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
    </section>
  );
}
