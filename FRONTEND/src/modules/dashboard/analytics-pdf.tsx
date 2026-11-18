import { join } from "node:path";
import {
  Circle,
  Document,
  Font,
  Line,
  Page,
  Polyline,
  StyleSheet,
  Svg,
  Text,
  View,
  type DocumentProps,
} from "@react-pdf/renderer";
import type { ReactElement } from "react";
import type { AnalyticsReport } from "@/shared/api/operations";

const directory = join(
  process.cwd(),
  "node_modules",
  "@expo-google-fonts",
  "noto-sans",
);
Font.register({
  family: "Nexa Noto",
  fonts: [
    {
      src: join(directory, "400Regular", "NotoSans_400Regular.ttf"),
      fontWeight: 400,
    },
    {
      src: join(directory, "700Bold", "NotoSans_700Bold.ttf"),
      fontWeight: 700,
    },
  ],
});
Font.registerHyphenationCallback((word) => [word]);

const styles = StyleSheet.create({
  page: {
    fontFamily: "Nexa Noto",
    fontSize: 9,
    color: "#183234",
    padding: 32,
    paddingBottom: 46,
  },
  brand: { fontWeight: 700, fontSize: 11, color: "#12766b", marginBottom: 8 },
  title: { fontWeight: 700, fontSize: 23, marginBottom: 4 },
  muted: { color: "#537174", marginBottom: 9 },
  section: { fontSize: 13, fontWeight: 700, marginTop: 14, marginBottom: 5 },
  row: {
    flexDirection: "row",
    borderBottomWidth: 0.5,
    borderBottomColor: "#d3e0df",
    paddingVertical: 5,
  },
  cell: { flex: 1, paddingHorizontal: 3 },
  head: { backgroundColor: "#e1f1ed", fontWeight: 700 },
  footer: {
    position: "absolute",
    bottom: 22,
    left: 32,
    right: 32,
    textAlign: "right",
    color: "#537174",
    fontSize: 8,
  },
});

const labels: Record<string, string> = {
  web: "Website",
  email: "Email",
  phone: "Điện thoại",
  unknown: "Chưa rõ",
  low: "Thấp",
  normal: "Bình thường",
  high: "Cao",
  urgent: "Khẩn cấp",
};
const num = (value: number) =>
  new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 1 }).format(value);
const pct = (a: number, b: number) => `${num((100 * a) / b)}%`;

function Table({
  columns,
  rows,
  weights,
}: {
  columns: string[];
  rows: string[][];
  weights?: number[];
}) {
  return (
    <View>
      <View wrap={false} style={[styles.row, styles.head]}>
        {columns.map((label, i) => (
          <Text key={i} style={[styles.cell, { flex: weights?.[i] ?? 1 }]}>
            {label}
          </Text>
        ))}
      </View>
      {rows.length ? (
        rows.map((row, i) => (
          <View wrap={false} key={i} style={styles.row}>
            {row.map((value, j) => (
              <Text key={j} style={[styles.cell, { flex: weights?.[j] ?? 1 }]}>
                {value}
              </Text>
            ))}
          </View>
        ))
      ) : (
        <Text style={styles.muted}>Chưa có dữ liệu trong kỳ.</Text>
      )}
    </View>
  );
}
export function AnalyticsPDF({
  data,
  from,
  to,
}: {
  data: AnalyticsReport;
  from: string;
  to: string;
}): ReactElement<DocumentProps> {
  const total = data.daily.reduce((sum, item) => sum + item.tickets, 0);
  const reviews = data.daily.reduce((sum, item) => sum + item.ratings, 0);
  const rating = data.daily.reduce(
    (sum, item) => sum + (item.csat ?? 0) * item.ratings,
    0,
  );
  const maxTickets = Math.max(1, ...data.daily.map((item) => item.tickets));
  const chartPoints = data.daily.map(
    (item, i) =>
      `${6 + (510 * i) / Math.max(1, data.daily.length - 1)},${61 - (52 * item.tickets) / maxTickets}`,
  );
  const points = chartPoints.join(" ");
  const single =
    chartPoints.length === 1 ? chartPoints[0].split(",").map(Number) : null;
  return (
    <Document title="Báo cáo hỗ trợ Nexa" language="vi">
      <Page size="A4" style={styles.page}>
        <Text style={styles.brand}>NEXA / HỖ TRỢ KHÁCH HÀNG</Text>
        <Text style={styles.title}>Báo cáo hỗ trợ</Text>
        <Text style={styles.muted}>
          {from} đến {to} · Giờ Việt Nam
        </Text>
        <Text>
          Phiếu mới: {total} | Lượt đánh giá: {reviews} | CSAT:{" "}
          {reviews ? num(rating / reviews) + " / 5" : "—"}
        </Text>
        <Text style={styles.section} minPresenceAhead={86}>
          Phiếu mới theo ngày
        </Text>
        <Text style={styles.muted}>
          Mức cao nhất trong kỳ:{" "}
          {maxTickets === 1 && total === 0 ? 0 : maxTickets} phiếu/ngày
        </Text>
        <Svg width={525} height={66} viewBox="0 0 525 66">
          <Line
            x1={6}
            y1={61}
            x2={516}
            y2={61}
            stroke="#c2d4d2"
            strokeWidth={1}
          />
          {single && (
            <Circle cx={single[0]} cy={single[1]} r={3} fill="#13796f" />
          )}
          {chartPoints.length > 1 && (
            <Polyline
              points={points}
              stroke="#13796f"
              strokeWidth={1.6}
              fill="none"
            />
          )}
        </Svg>
        <Text style={styles.section} minPresenceAhead={40}>
          Phân bổ theo kênh và độ ưu tiên
        </Text>
        <Table
          columns={["Nhóm", "Giá trị", "Số phiếu"]}
          rows={[
            ...data.channels.map((item) => [
              "Kênh",
              labels[item.name] || item.name,
              String(item.count),
            ]),
            ...data.priorities.map((item) => [
              "Ưu tiên",
              labels[item.name] || item.name,
              String(item.count),
            ]),
          ]}
        />
        <Text style={styles.section} minPresenceAhead={42}>
          Xếp hạng theo số phiếu đã giải quyết
        </Text>
        <Table
          columns={[
            "Nhân viên",
            "Phiếu đóng",
            "Phản hồi TB (phút)",
            "CSAT / 5",
          ]}
          weights={[2, 1, 1.5, 1.1]}
          rows={data.leaderboard.map((row) => [
            row.name,
            String(row.resolved),
            row.responses ? num(row.average_response_minutes) : "—",
            row.ratings ? `${num(row.average_rating)} (${row.ratings})` : "—",
          ])}
        />
        <Text style={styles.section} minPresenceAhead={42}>
          Tuân thủ SLA theo tuần
        </Text>
        <Table
          columns={[
            "Tuần bắt đầu",
            "Nhân viên",
            "Đúng hạn / số mốc",
            "Tuân thủ",
          ]}
          weights={[1, 2, 1.4, 1]}
          rows={data.sla.map((row) => [
            row.week,
            row.agent,
            `${row.passed} / ${row.total}`,
            pct(row.passed, row.total),
          ])}
        />
        <Text style={styles.section} minPresenceAhead={42}>
          Khách quay lại mở phiếu
        </Text>
        <Table
          columns={[
            "Tuần đầu",
            "Số khách",
            ...Array.from({ length: 9 }, (_, i) => `W${i}`),
          ]}
          weights={[2, 1.1]}
          rows={data.cohorts.map((row) => [
            row.week,
            String(row.size),
            ...row.active.map((count) =>
              count === null ? "—" : pct(count, row.size),
            ),
          ])}
        />
        <Text
          fixed
          style={styles.footer}
          render={({ pageNumber, totalPages }) =>
            `Nexa · Trang ${pageNumber}/${totalPages}`
          }
        />
      </Page>
    </Document>
  );
}
