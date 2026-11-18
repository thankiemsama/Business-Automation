import { renderToBuffer } from "@react-pdf/renderer";
import { cookies } from "next/headers";
import type { AnalyticsReport } from "@/shared/api/operations";
import { AnalyticsPDF } from "@/modules/dashboard/analytics-pdf";

export const runtime = "nodejs";
let activeExports = 0;

export async function GET(request: Request) {
  const token = (await cookies()).get("nexa_session")?.value;
  if (!token) return new Response("Cần đăng nhập", { status: 401 });
  const query = new URL(request.url).searchParams;
  const from = query.get("from") ?? "";
  const to = query.get("to") ?? "";
  if (![from, to].every((date) => /^\d{4}-\d{2}-\d{2}$/.test(date))) {
    return new Response("Khoảng ngày không hợp lệ", { status: 400 });
  }
  if (activeExports >= 3)
    return new Response("Đang xử lý các tệp PDF khác; hãy thử lại.", {
      status: 429,
      headers: { "Retry-After": "5" },
    });
  activeExports++;
  try {
    const params = new URLSearchParams({ from, to });
    const backend = process.env.BACKEND_URL || "http://127.0.0.1:8080";
    const headers: Record<string, string> = {
      Authorization: `Bearer ${token}`,
    };
    const ip = request.headers.get("x-nexa-ingress-ip");
    const secret = process.env.NEXA_PROXY_SECRET;
    if (process.env.NEXA_TRUST_INGRESS_IP === "true" && ip && secret) {
      headers["X-Nexa-Proxy-Secret"] = secret;
      headers["X-Nexa-Client-IP"] = ip;
    }
    const upstream = await fetch(
      `${backend}/api/v1/reports/analytics?${params}`,
      {
        headers,
        cache: "no-store",
        redirect: "error",
        signal: AbortSignal.timeout(15000),
      },
    );
    if (!upstream.ok)
      return new Response("Không tải được dữ liệu báo cáo", {
        status: upstream.status,
      });
    const { data } = (await upstream.json()) as { data: AnalyticsReport };
    const document = AnalyticsPDF({ data, from, to });
    const buffer = await renderToBuffer(document);
    return new Response(new Uint8Array(buffer), {
      headers: {
        "Content-Type": "application/pdf",
        "Content-Disposition": `attachment; filename="bao-cao-nexa-${from}-${to}.pdf"`,
        "Cache-Control": "no-store",
      },
    });
  } catch {
    return new Response("Không tạo được tệp báo cáo", { status: 502 });
  } finally {
    activeExports--;
  }
}
