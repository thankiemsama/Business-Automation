"use client";
import { FeatureSections } from "@/shared/ui/feature-sections";
import Link from "next/link";
import { useCallback } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import {
  Badge,
  EmptyState,
  ErrorState,
  Loading,
  PageHeading,
  dateLabel,
} from "@/shared/ui/primitives";
import { OperationsReport } from "./operations-report";
import { Icon, type IconName } from "@/shared/ui/icon";
import styles from "./dashboard-overview.module.css";
const metrics: {
  key: "customers" | "conversations" | "open_tickets" | "messages";
  label: string;
  note: string;
  icon: IconName;
  color: string;
  href: string;
}[] = [
  {
    key: "customers",
    label: "Tổng khách hàng",
    note: "Những kết nối của doanh nghiệp",
    icon: "users",
    color: "blue",
    href: "/customers",
  },
  {
    key: "conversations",
    label: "Hội thoại",
    note: "Trên tất cả các kênh liên hệ",
    icon: "chat",
    color: "purple",
    href: "/conversations",
  },
  {
    key: "open_tickets",
    label: "Phiếu cần xử lý",
    note: "Đang mở hoặc đang xử lý",
    icon: "ticket",
    color: "orange",
    href: "/tickets",
  },
  {
    key: "messages",
    label: "Tin nhắn",
    note: "Khách hàng, nhân viên và trợ lý",
    icon: "bolt",
    color: "green",
    href: "/conversations",
  },
];
export function DashboardPage() {
  return (
    <>
      <PageHeading
        title="Tổng quan"
        description="Theo dõi hàng đợi hỗ trợ và hoạt động của đội ngũ."
      >
        <Link href="/conversations" className="button primary">
          <Icon name="chat" size={17} /> Mở hộp hội thoại
        </Link>
      </PageHeading>
      <FeatureSections
        label="Tổng quan và báo cáo"
        sections={[
          {
            id: "overview",
            label: "Tổng quan",
            content: <DashboardOverview />,
          },
          {
            id: "reports",
            label: "Báo cáo hỗ trợ",
            content: <OperationsReport />,
          },
        ]}
      />
    </>
  );
}

function DashboardOverview() {
  const services = useServices();
  const load = useCallback(
    (signal: AbortSignal) => services.dashboard.get(signal),
    [services],
  );
  const ticketLoad = useCallback(
    (signal: AbortSignal) =>
      services.tickets.list({ status: "open", limit: 5 }, signal),
    [services],
  );
  const dashboard = useQuery(load, "workspace-refresh");
  const tickets = useQuery(ticketLoad, "workspace-refresh");
  const data = dashboard.data;
  const resolved = data ? data.tickets - data.open_tickets : 0;
  const rate = data?.tickets ? Math.round((resolved / data.tickets) * 100) : 0;
  return (
    <div className={styles.overview}>
      <div className="overview-toolbar">
        <h2>Chỉ số hỗ trợ</h2>
        <button
          className="button secondary compact"
          onClick={() => {
            dashboard.reload();
            tickets.reload();
          }}
        >
          <Icon name="refresh" size={16} /> Làm mới
        </button>
      </div>
      {dashboard.error && (
        <ErrorState message={dashboard.error} retry={dashboard.reload} />
      )}
      <section
        className="metric-grid"
        aria-label="Chỉ số hỗ trợ"
        aria-busy={dashboard.loading}
      >
        {metrics.map((metric) => (
          <Link href={metric.href} className="metric-card" key={metric.key}>
            <div className="metric-top">
              <span>{metric.label}</span>
              <span className={`metric-icon ${metric.color}`}>
                <Icon name={metric.icon} size={20} />
              </span>
            </div>
            <strong>
              {dashboard.loading
                ? "—"
                : (data?.[metric.key]?.toLocaleString("vi-VN") ?? "—")}
            </strong>
            <div className="metric-bottom">
              <span>{metric.note}</span>
              <Icon name="arrow" size={15} />
            </div>
          </Link>
        ))}
      </section>
      <div className="dashboard-columns">
        <section className="panel">
          <div className="panel-header">
            <div>
              <h2>Phiếu đang mở</h2>
              {tickets.data && (
                <p>
                  {tickets.data.total.toLocaleString("vi-VN")} phiếu đang mở
                </p>
              )}
            </div>
            <Link href="/tickets" className="text-link">
              Xem tất cả <Icon name="arrow" size={15} />
            </Link>
          </div>
          {tickets.error ? (
            <ErrorState message={tickets.error} retry={tickets.reload} />
          ) : tickets.loading ? (
            <Loading />
          ) : !tickets.data?.items.length ? (
            <EmptyState
              icon="check"
              title="Mọi việc đã ổn thỏa"
              description="Chưa có phiếu hỗ trợ nào ở trạng thái đang mở."
            />
          ) : (
            <div className="dashboard-ticket-list">
              {tickets.data.items.map((ticket) => (
                <Link
                  href={`/tickets/${encodeURIComponent(ticket.id)}`}
                  key={ticket.id}
                  className="dashboard-ticket"
                >
                  <span className="ticket-list-icon">
                    <Icon name="ticket" size={19} />
                  </span>
                  <div>
                    <strong>{ticket.subject}</strong>
                    <span>
                      #{ticket.id.slice(0, 8)} · {dateLabel(ticket.created_at)}
                    </span>
                  </div>
                  <Badge value={ticket.priority} />
                  <Icon name="chevron" size={16} />
                </Link>
              ))}
              <p className="list-footnote">
                Hiển thị {tickets.data.items.length} / {tickets.data.total}{" "}
                phiếu đang mở, theo thứ tự tạo.
              </p>
            </div>
          )}
        </section>
        <section className="panel health-panel">
          <div className="panel-header">
            <div>
              <h2>Tiến độ hỗ trợ</h2>
              <p>Trên toàn bộ phiếu hỗ trợ của doanh nghiệp</p>
            </div>
            <Icon name="ticket" size={20} />
          </div>
          <div className="progress-content">
            <div className="progress-number">
              <strong>{data ? `${rate}%` : "—"}</strong>
              <span>phiếu đã giải quyết</span>
            </div>
            <div
              className="progress-track"
              role={data?.tickets ? "meter" : undefined}
              aria-label={
                data?.tickets ? "Tỷ lệ phiếu đã giải quyết" : undefined
              }
              aria-valuenow={data?.tickets ? rate : undefined}
            >
              <span style={{ width: `${rate}%` }} />
            </div>
            <div className="progress-stats">
              <div>
                <span className="status-dot" />
                <strong>{data ? resolved : "—"}</strong> Đã giải quyết
              </div>
              <div>
                <span className="status-dot orange-dot" />
                <strong>{data?.open_tickets ?? "—"}</strong> Cần xử lý
              </div>
            </div>
            {data && !data.tickets && (
              <div className="progress-note">
                <Icon name="info" size={17} />
                <span>Tiến độ sẽ xuất hiện khi có phiếu hỗ trợ đầu tiên.</span>
              </div>
            )}
          </div>
        </section>
      </div>
      <section className="quick-section">
        <div className="section-title">
          <h2>Truy cập nhanh</h2>
        </div>
        <div className="quick-grid">
          {[
            {
              href: "/customers",
              icon: "users" as const,
              title: "Khách hàng",
              text: `${data?.customers ?? "—"} hồ sơ khách hàng`,
            },
            {
              href: "/knowledge",
              icon: "book" as const,
              title: "Kho kiến thức",
              text: `${data?.knowledge ?? "—"} bài kiến thức`,
            },
            {
              href: "/automations",
              icon: "bolt" as const,
              title: "Tự động hóa",
              text: `${data?.automations ?? "—"} quy tắc`,
            },
          ].map((item) => (
            <Link key={item.href} href={item.href} className="quick-card">
              <span className="quick-icon">
                <Icon name={item.icon} size={21} />
              </span>
              <div>
                <h3>{item.title}</h3>
                <p>{item.text}</p>
              </div>
              <Icon name="arrow" size={16} />
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
