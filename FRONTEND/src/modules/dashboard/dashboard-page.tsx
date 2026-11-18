"use client";
import Link from "next/link";
import { useCallback } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
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
  const services = useServices();
  const user = useUser();
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
    <>
      <PageHeading
        eyebrow="MỘT NGÀY LÀM VIỆC HIỆU QUẢ"
        title={`Xin chào, ${user.name.split(" ").slice(-1)[0]} 👋`}
        description="Mọi kết nối và công việc của bạn, trong một góc nhìn."
      >
        <button
          className="button secondary"
          onClick={() => {
            dashboard.reload();
            tickets.reload();
          }}
        >
          <Icon name="refresh" size={17} />
          Làm mới
        </button>
        <Link href="/conversations" className="button primary">
          <Icon name="chat" size={17} />
          Mở hộp hội thoại
        </Link>
      </PageHeading>
      <section className="welcome-banner">
        <div>
          <span className="banner-eyebrow">KẾT NỐI TỐT HƠN, MỖI NGÀY</span>
          <h2>
            Chăm sóc bằng sự thấu hiểu.
            <br />
            <span>Vận hành bằng sự đơn giản.</span>
          </h2>
          <p>Đội ngũ, khách hàng và quy trình — cùng chung một nhịp.</p>
          <Link href="/automations">
            Khám phá tự động hóa <Icon name="arrow" size={17} />
          </Link>
        </div>
        <div className="connection-art" aria-hidden="true">
          <div className="orbit orbit-one" />
          <div className="orbit orbit-two" />
          <span className="art-center">
            <span className="brand-mark">n</span>
          </span>
          <span className="art-node node-one">
            <Icon name="chat" size={26} />
          </span>
          <span className="art-node node-two">
            <Icon name="users" size={25} />
          </span>
          <span className="art-node node-three">
            <Icon name="bolt" size={25} />
          </span>
          <span className="art-dot dot-one" />
          <span className="art-dot dot-two" />
        </div>
      </section>
      {dashboard.error && (
        <ErrorState message={dashboard.error} retry={dashboard.reload} />
      )}
      <section className="metric-grid">
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
      <OperationsReport />
      <div className="dashboard-columns">
        <section className="panel">
          <div className="panel-header">
            <div>
              <h2>Phiếu đang mở</h2>
              <p>Ưu tiên những yêu cầu đang chờ bạn.</p>
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
                  href="/tickets"
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
              <p>Mỗi yêu cầu đều có một lời giải.</p>
            </div>
            <Icon name="ticket" size={20} />
          </div>
          <div className="progress-content">
            <div className="progress-number">
              <strong>{data ? `${rate}%` : "—"}</strong>
              <span>phiếu đã giải quyết</span>
            </div>
            <div className="progress-track">
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
            <div className="progress-note">
              <Icon name="info" size={17} />
              <span>
                {data?.tickets
                  ? "Dựa trên toàn bộ phiếu hỗ trợ của doanh nghiệp."
                  : "Tiến độ sẽ xuất hiện khi có phiếu hỗ trợ đầu tiên."}
              </span>
            </div>
          </div>
        </section>
      </div>
      <section className="quick-section">
        <div className="section-title">
          <h2>Không gian vận hành</h2>
          <span>Công cụ đúng cho từng công việc</span>
        </div>
        <div className="quick-grid">
          {[
            {
              href: "/customers",
              icon: "users" as const,
              title: "Kết nối khách hàng",
              text: "Lưu thông tin để mỗi tương tác thêm gần gũi.",
              label: "Quản lý khách hàng",
            },
            {
              href: "/knowledge",
              icon: "book" as const,
              title: "Câu trả lời nhất quán",
              text: `${data?.knowledge ?? "—"} bài kiến thức trong thư viện của bạn.`,
              label: "Mở kho kiến thức",
            },
            {
              href: "/automations",
              icon: "bolt" as const,
              title: "Bớt việc lặp lại",
              text: `${data?.automations ?? "—"} quy tắc giúp đội ngũ chủ động hơn.`,
              label: "Xem quy tắc",
            },
          ].map((item) => (
            <Link key={item.href} href={item.href} className="quick-card">
              <span className="quick-icon">
                <Icon name={item.icon} size={21} />
              </span>
              <h3>{item.title}</h3>
              <p>{item.text}</p>
              <span className="text-link">
                {item.label}
                <Icon name="arrow" size={16} />
              </span>
            </Link>
          ))}
        </div>
      </section>
    </>
  );
}
