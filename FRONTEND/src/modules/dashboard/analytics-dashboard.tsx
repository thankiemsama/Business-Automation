"use client";
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
  unknown: "Chưa rõ",
  web: "Website",
  email: "Email",
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
  if (analysis.error)
    return <ErrorState message={analysis.error} retry={analysis.reload} />;
  if (!analysis.data) return <Loading />;
  const { daily, channels, priorities, sla, cohorts, leaderboard } =
    analysis.data;
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
      <section className="panel analytics-card">
        <h3>Tuân thủ SLA theo tuần và nhân viên</h3>
        <p className="muted">
          Tính theo hạn phản hồi và hạn xử lý đã đến. Mỗi phiếu có tối đa hai
          mốc.
        </p>
        {sla.length ? (
          <div className="table-scroll">
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
        <div className="table-scroll">
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
          <div className="table-scroll">
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
