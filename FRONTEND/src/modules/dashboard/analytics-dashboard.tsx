"use client";
import Link from "next/link";
import { useCallback, useState } from "react";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { loadAnalytics } from "@/shared/api/operations";
import { useQuery } from "@/shared/hooks/use-query";
import { ErrorState, Loading } from "@/shared/ui/primitives";

const colors = [
  "#157e73",
  "#4f70be",
  "#ad5c30",
  "#8a55a2",
  "#aa4664",
  "#637450",
  "#336a79",
];
const display: Record<string, string> = {
  completed: "Đã đánh giá",
  partial: "Đánh giá một phần",
  unsupported: "Tệp chưa hỗ trợ",
  pending: "Đang chờ",
  quota_wait: "Đang chờ quota",
  running: "Đang xử lý",
  failed: "Đánh giá thất bại",
  cancelled: "Đã hủy",
  image: "Ảnh",
  text: "Văn bản",
  attachment: "Tệp đính kèm",
  voice_transcript: "Bản chép lời",
  cancellation: "Nguy cơ hủy",
  unfulfilled_promise: "Cam kết chưa được xử lý",
  procedure: "Quy trình",
  prohibited_language: "Ngôn từ",
  complaint: "Nguy cơ khiếu nại",
  unaddressed_emotion: "Cảm xúc chưa được xử lý",
  unknown: "Chưa rõ",
  web: "Website",
  email: "Email",
  zalo: "Zalo OA",
  whatsapp: "WhatsApp",
  phone: "Điện thoại",
  low: "Thấp",
  normal: "Bình thường",
  high: "Cao",
  urgent: "Khẩn cấp",
};
const vn = new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 1 });
const rate = (passed: number, total: number) =>
  `${vn.format((100 * passed) / total)}%`;
type Ranking = "resolved" | "response" | "csat";
const tooltip = {
  background: "var(--surface)",
  border: "1px solid var(--line)",
  color: "var(--ink)",
};

export function AnalyticsDashboard({ from, to }: { from: string; to: string }) {
  const load = useCallback(
    (signal: AbortSignal) => loadAnalytics(from, to, signal),
    [from, to],
  );
  const analysis = useQuery(load);
  const [ranking, setRanking] = useState<Ranking>("resolved");
  const [showQA, setShowQA] = useState(false);
  if (analysis.error)
    return <ErrorState message={analysis.error} retry={analysis.reload} />;
  if (!analysis.data) return <Loading />;
  const {
    daily,
    channels,
    priorities,
    sla,
    cohorts,
    leaderboard,
    ai,
    workforce,
    realtime_qa,
    multimodal_qa,
    csat_by_channel,
    response_minutes_by_priority,
  } = analysis.data;
  const tabs = realtime_qa ? (
    <div
      className="toolbar-controls"
      role="group"
      aria-label="Báo cáo phân tích"
    >
      <button
        type="button"
        className="button secondary compact"
        aria-pressed={!showQA}
        onClick={() => setShowQA(false)}
      >
        Tổng quan
      </button>
      <button
        type="button"
        className="button secondary compact"
        aria-pressed={showQA}
        onClick={() => setShowQA(true)}
      >
        Kiểm tra chất lượng
      </button>
    </div>
  ) : null;
  if (showQA && realtime_qa) {
    return (
      <div className="analytics-content">
        {tabs}
        <div className="analytics-grid">
          <QABreakdown
            heading="Theo loại rủi ro"
            counts={realtime_qa.by_kind}
          />
          <QABreakdown heading="Theo nhân viên" counts={realtime_qa.by_agent} />
          <QABreakdown heading="Theo ngày (ICT)" counts={realtime_qa.by_day} />
        </div>
        {multimodal_qa && (
          <section>
            <h2>QA tự động theo dữ liệu</h2>
            <div className="analytics-grid">
              <QABreakdown
                heading="Lượt đánh giá theo trạng thái"
                counts={multimodal_qa.statuses}
                countLabel="Lượt"
              />
              <QABreakdown
                heading="Lượt đánh giá theo dữ liệu"
                counts={multimodal_qa.modalities}
                countLabel="Lượt"
              />
              <QABreakdown
                heading="Rủi ro từ QA tự động"
                counts={multimodal_qa.risks}
              />
            </div>
            {multimodal_qa.alerts.length > 0 && (
              <ul className="form-stack">
                {multimodal_qa.alerts.map((alert) => (
                  <li key={alert.message_id}>
                    <Link
                      href={`/conversations/${encodeURIComponent(alert.conversation_id)}`}
                    >
                      Mở hội thoại
                    </Link>
                    {alert.labels.map((label) => (
                      <p key={label.kind}>
                        {display[label.kind] ?? label.kind} · {label.message}
                      </p>
                    ))}
                  </li>
                ))}
              </ul>
            )}
          </section>
        )}
      </div>
    );
  }
  const ranked = [...leaderboard].sort((a, b) => {
    if (ranking === "response")
      return (
        (a.responses ? a.average_response_minutes : Infinity) -
        (b.responses ? b.average_response_minutes : Infinity)
      );
    if (ranking === "csat")
      return (
        (b.ratings ? b.average_rating : 0) -
          (a.ratings ? a.average_rating : 0) || b.ratings - a.ratings
      );
    return b.resolved - a.resolved;
  });
  const byDay = daily.map((point) => ({
    ...point,
    label: point.day.slice(8) + "/" + point.day.slice(5, 7),
  }));
  const channel = channels.map((item) => ({
    ...item,
    label: display[item.name] || item.name,
  }));
  const priority = priorities.map((item) => ({
    ...item,
    label: display[item.name] || item.name,
  }));
  return (
    <div className="analytics-content">
      {tabs}
      {workforce && (
        <div className="analytics-grid">
          <section className="panel analytics-card">
            <h3>Dự báo phiếu mới · 7 ngày</h3>
            <p>
              Trung bình theo cùng thứ trong tuần, dùng tối đa 8 tuần lịch sử.
            </p>
            <table>
              <thead>
                <tr>
                  <th>Ngày (ICT)</th>
                  <th>Phiếu dự kiến</th>
                  <th>Ngày mẫu</th>
                </tr>
              </thead>
              <tbody>
                {workforce.forecast.map((item) => (
                  <tr key={item.day}>
                    <th scope="row">
                      {item.day.slice(8)}/{item.day.slice(5, 7)}
                    </th>
                    <td>
                      {item.expected == null
                        ? "Chưa đủ dữ liệu"
                        : vn.format(item.expected)}
                    </td>
                    <td>{vn.format(item.samples)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
          <section className="panel analytics-card">
            <h3>Phiếu AI chạm rồi nhân viên giải quyết</h3>
            <p>
              {workforce.ai_to_human.touched_resolved
                ? `${rate(workforce.ai_to_human.human_resolved, workforce.ai_to_human.touched_resolved)} · ${workforce.ai_to_human.human_resolved}/${workforce.ai_to_human.touched_resolved} phiếu`
                : "Chưa có phiếu đã giải quyết có dấu vết AI trong kỳ."}
            </p>
            <p>
              Mẫu số gồm các phiếu đã giải quyết có dấu vết AI trước lúc đóng.
              Phân nhóm theo kênh và ưu tiên.
            </p>
            {!!workforce.by_queue_type.length && (
              <div className="table-scroll">
                <table>
                  <thead>
                    <tr>
                      <th>Kênh / ưu tiên</th>
                      <th>Nhân viên / tổng</th>
                      <th>AI</th>
                      <th>Chưa rõ</th>
                    </tr>
                  </thead>
                  <tbody>
                    {workforce.by_queue_type.map((row) => (
                      <tr key={`${row.channel}:${row.priority}`}>
                        <th scope="row">
                          {display[row.channel ?? "unknown"] ?? row.channel} ·{" "}
                          {display[row.priority ?? "unknown"] ?? row.priority}
                        </th>
                        <td>
                          {row.human_resolved}/{row.touched_resolved} ·{" "}
                          {rate(row.human_resolved, row.touched_resolved)}
                        </td>
                        <td>{row.agent_resolved}</td>
                        <td>{row.unknown_resolver}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </div>
      )}
      <div className="analytics-grid">
        <section className="panel analytics-card">
          <h3>Phiếu mới theo ngày</h3>
          <div className="analytics-chart">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={byDay}>
                <CartesianGrid stroke="var(--line)" strokeDasharray="3 3" />
                <XAxis dataKey="label" tick={{ fontSize: 11 }} />
                <YAxis allowDecimals={false} width={32} />
                <Tooltip contentStyle={tooltip} />
                <Line
                  type="monotone"
                  isAnimationActive={false}
                  dataKey="tickets"
                  name="Phiếu"
                  stroke={colors[0]}
                  strokeWidth={2}
                  dot={byDay.length === 1}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </section>
        <section className="panel analytics-card">
          <h3>Phiếu theo kênh</h3>
          {channel.length ? (
            <div className="analytics-chart">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={channel}>
                  <CartesianGrid stroke="var(--line)" vertical={false} />
                  <XAxis dataKey="label" tick={{ fontSize: 11 }} />
                  <YAxis allowDecimals={false} width={32} />
                  <Tooltip contentStyle={tooltip} />
                  <Bar
                    dataKey="count"
                    name="Phiếu"
                    fill={colors[1]}
                    radius={[5, 5, 0, 0]}
                    isAnimationActive={false}
                  />
                </BarChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <p>Chưa có phiếu trong khoảng ngày này.</p>
          )}
        </section>
        <section className="panel analytics-card">
          <h3>Phiếu theo độ ưu tiên</h3>
          {priority.length ? (
            <div className="analytics-chart">
              <ResponsiveContainer width="100%" height="100%">
                <PieChart>
                  <Pie
                    data={priority}
                    dataKey="count"
                    nameKey="label"
                    outerRadius={74}
                    innerRadius={46}
                    isAnimationActive={false}
                  >
                    {priority.map((item, index) => (
                      <Cell
                        key={item.name}
                        fill={colors[index % colors.length]}
                      />
                    ))}
                  </Pie>
                  <Tooltip contentStyle={tooltip} />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <p>Chưa có phiếu trong khoảng ngày này.</p>
          )}
        </section>
        <section className="panel analytics-card">
          <h3>Xu hướng CSAT</h3>
          {daily.some((point) => point.ratings) ? (
            <div className="analytics-chart">
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={byDay}>
                  <CartesianGrid stroke="var(--line)" strokeDasharray="3 3" />
                  <XAxis dataKey="label" tick={{ fontSize: 11 }} />
                  <YAxis domain={[1, 5]} ticks={[1, 2, 3, 4, 5]} width={30} />
                  <Tooltip contentStyle={tooltip} />
                  <Line
                    dataKey="csat"
                    name="CSAT / 5"
                    stroke={colors[3]}
                    strokeWidth={2}
                    dot={{ r: 3 }}
                    isAnimationActive={false}
                  />
                </LineChart>
              </ResponsiveContainer>
            </div>
          ) : (
            <p>Chưa có lượt đánh giá trong khoảng ngày này.</p>
          )}
        </section>
      </div>
      <div className="analytics-grid">
        <section className="panel analytics-card">
          <h3>CSAT theo kênh</h3>
          {csat_by_channel.length === 0 ? (
            <p>Chưa có đánh giá</p>
          ) : (
            <ul>
              {csat_by_channel.map(({ name, value, samples }) => (
                <li key={name}>
                  {display[name] || name} ·{" "}
                  {value == null ? "—" : vn.format(value) + "/5"} ({samples}{" "}
                  đánh giá)
                </li>
              ))}
            </ul>
          )}
        </section>
        <section className="panel analytics-card">
          <h3>Phản hồi đầu tiên theo độ ưu tiên</h3>
          {response_minutes_by_priority.length === 0 ? (
            <p>Chưa có phản hồi ghi nhận</p>
          ) : (
            <ul>
              {response_minutes_by_priority.map(({ name, value, samples }) => (
                <li key={name}>
                  {display[name] || name || "Chưa rõ"} ·{" "}
                  {value == null ? "—" : vn.format(value) + " phút"} ({samples}{" "}
                  phiếu)
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
      <details className="panel analytics-card">
        <summary>Hiệu quả AI</summary>
        <dl>
          <dt>Phiếu được chấm điểm trong nhóm đã xếp hàng kiểm tra</dt>
          <dd>
            {ai.reviews_queued
              ? `${rate(ai.reviews_scored, ai.reviews_queued)} (${ai.reviews_scored}/${ai.reviews_queued})`
              : "Chưa có dữ liệu"}
          </dd>
          <dt>Lượt kiểm tra đang chờ hoặc đang chạy</dt>
          <dd>{vn.format(ai.reviews_pending ?? 0)}</dd>
          <dt>Lượt kiểm tra thất bại</dt>
          <dd>{vn.format(ai.reviews_failed ?? 0)}</dd>
          <dt>Gợi ý phân loại chuyển duyệt</dt>
          <dd>{vn.format(ai.triage_offered ?? 0)}</dd>
          <dt>Gợi ý được nhân viên áp dụng</dt>
          <dd>{vn.format(ai.triage_accepted ?? 0)}</dd>
          <dt>Lượt AI tự áp dụng phân loại</dt>
          <dd>{vn.format(ai.triage_auto_applied ?? 0)}</dd>
          <dt>Hành động agent đề xuất</dt>
          <dd>{vn.format(ai.agent_proposed ?? 0)}</dd>
          <dt>Hành động được duyệt và thực hiện</dt>
          <dd>{vn.format(ai.agent_applied ?? 0)}</dd>
          <dt>Phiếu tự giải quyết sau xác nhận của khách</dt>
          <dd>{vn.format(ai.agent_auto_resolved ?? 0)}</dd>
          <dt>Hội thoại bot tự giải quyết</dt>
          <dd>
            {ai.bot_cases
              ? `${rate(ai.auto_resolved, ai.bot_cases)} (${ai.auto_resolved}/${ai.bot_cases})`
              : "Chưa có dữ liệu"}
          </dd>
          <dt>Yêu cầu được giải quyết trước khi tạo phiếu</dt>
          <dd>
            {ai.avoided_tickets + ai.tickets
              ? `${rate(ai.avoided_tickets, ai.avoided_tickets + ai.tickets)} (${ai.avoided_tickets} yêu cầu)`
              : "Chưa có dữ liệu"}
          </dd>
          <dt>Chi phí AI ước tính mỗi phiếu</dt>
          <dd>
            {ai.cost_per_ticket_usd == null
              ? "Chưa có dữ liệu"
              : new Intl.NumberFormat("vi-VN", {
                  style: "currency",
                  currency: "USD",
                  maximumFractionDigits: 6,
                }).format(ai.cost_per_ticket_usd)}
          </dd>
        </dl>
        {ai.unpriced_requests > 0 && (
          <p>{ai.unpriced_requests} lượt AI chưa có đủ dữ liệu để tính giá.</p>
        )}
      </details>
      <section className="panel analytics-card">
        <h3>Tuân thủ SLA theo tuần và nhân viên</h3>
        <p className="muted">
          Tính theo hạn phản hồi và hạn xử lý đã đến. Mỗi phiếu có tối đa hai
          mốc.
        </p>
        {sla.length ? (
          <div
            className="table-scroll"
            role="region"
            tabIndex={0}
            aria-label="Tuân thủ SLA theo tuần và nhân viên"
          >
            <table>
              <thead>
                <tr>
                  <th>Tuần bắt đầu</th>
                  <th>Nhân viên</th>
                  <th>Đúng hạn / số mốc</th>
                  <th>Tuân thủ</th>
                </tr>
              </thead>
              <tbody>
                {[...sla].reverse().map((row) => (
                  <tr key={row.week + row.agent_id}>
                    <td>{row.week}</td>
                    <td>{row.agent}</td>
                    <td>
                      {row.passed} / {row.total}
                    </td>
                    <td>
                      <strong>{rate(row.passed, row.total)}</strong>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p>Chưa có mốc SLA đến hạn.</p>
        )}
      </section>
      <section className="panel analytics-card">
        <div className="analytics-heading">
          <h3>Xếp hạng nhân viên</h3>
          <select
            aria-label="Xếp hạng theo"
            value={ranking}
            onChange={(event) => setRanking(event.target.value as Ranking)}
          >
            <option value="resolved">Phiếu đã đóng</option>
            <option value="response">Phản hồi nhanh</option>
            <option value="csat">CSAT cao</option>
          </select>
        </div>
        <div
          className="table-scroll"
          role="region"
          tabIndex={0}
          aria-label="Xếp hạng nhân viên"
        >
          <table>
            <thead>
              <tr>
                <th>Nhân viên</th>
                <th>Phiếu đóng</th>
                <th>Phản hồi TB (phút)</th>
                <th>CSAT / 5</th>
              </tr>
            </thead>
            <tbody>
              {ranked.map((row) => (
                <tr key={row.id}>
                  <td>{row.name}</td>
                  <td>{row.resolved}</td>
                  <td>
                    {row.responses
                      ? vn.format(row.average_response_minutes)
                      : "—"}
                  </td>
                  <td>
                    {row.ratings
                      ? `${vn.format(row.average_rating)} (${row.ratings})`
                      : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
      <section className="panel analytics-card">
        <h3>Khách quay lại mở phiếu</h3>
        <p className="muted">
          Nhóm theo tuần mở phiếu đầu tiên. Mỗi ô là tỷ lệ khách mở phiếu trong
          tuần tương ứng. Dấu gạch là tuần chưa kết thúc.
        </p>
        {cohorts.length ? (
          <div
            className="table-scroll"
            role="region"
            tabIndex={0}
            aria-label="Khách quay lại mở phiếu"
          >
            <table>
              <thead>
                <tr>
                  <th>Tuần đầu</th>
                  <th>Số khách</th>
                  {Array.from({ length: 9 }, (_, n) => (
                    <th key={n}>W{n}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {[...cohorts].reverse().map((row) => (
                  <tr key={row.week}>
                    <th scope="row">{row.week}</th>
                    <td>{row.size}</td>
                    {row.active.map((count, i) => (
                      <td
                        key={i}
                        title={
                          count === null
                            ? "Tuần chưa kết thúc"
                            : `${count}/${row.size} khách`
                        }
                      >
                        {count === null ? "—" : rate(count, row.size)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p>Chưa có nhóm khách trong khoảng ngày này.</p>
        )}
      </section>
    </div>
  );
}

function QABreakdown({
  heading,
  counts,
  countLabel = "Cảnh báo",
}: {
  heading: string;
  counts: Record<string, number>;
  countLabel?: string;
}) {
  const rows = Object.entries(counts).sort(([a], [b]) =>
    a.localeCompare(b, "vi"),
  );
  return (
    <section className="panel analytics-card">
      <h3>{heading}</h3>
      {rows.length === 0 ? (
        <p>Chưa có cảnh báo trong kỳ.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th scope="col">{heading}</th>
              <th scope="col">{countLabel}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(([name, count]) => (
              <tr key={name}>
                <th scope="row">{display[name] ?? name}</th>
                <td>{vn.format(count)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
