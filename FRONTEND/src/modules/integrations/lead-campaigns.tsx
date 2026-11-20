"use client";

import { useCallback, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";
import { Loading } from "@/shared/ui/primitives";

type Campaign = {
  follow_up_tickets?: boolean;
  accepted?: number;
  replied?: number;
  id?: string;
  name: string;
  channel: "zalo" | "whatsapp";
  phone: string;
  welcome: string;
  follow_ups: string[];
  enabled: boolean;
  url?: string;
  visits?: number;
  leads?: number;
  engaged?: number;
};

const api = new FetchHttpClient();
const path = "lead-campaigns";
const loadCampaigns = (signal: AbortSignal) =>
  api.request<Campaign[]>(path, { signal });
const initial: Campaign = {
  name: "",
  channel: "zalo",
  phone: "",
  welcome: "",
  follow_ups: [],
  enabled: true,
};

function CampaignResults({ campaign }: { campaign: Campaign }) {
  const base = `${path}/${encodeURIComponent(campaign.id ?? "")}`;
  const loadQR = useCallback(
    (signal: AbortSignal) =>
      api.request<{ image: string }>(`${base}/qr`, { signal }),
    [base],
  );
  const loadLeads = useCallback(
    (signal: AbortSignal) =>
      api.request<
        {
          conversation_id: string;
          name: string;
          score: number;
          intent?: string;
          unsubscribed: boolean;
          sales_advice?: string;
        }[]
      >(`${base}/leads`, { signal }),
    [base],
  );
  const qr = useQuery(loadQR);
  const leads = useQuery(loadLeads, "workspace-refresh");
  return (
    <section className="workspace-results">
      <h3>Mã QR và kết quả</h3>
      {qr.loading && <Loading />}
      {qr.data && (
        <div className="integration-qr">
          <Image
            src={qr.data.image}
            alt={`Mã QR ${campaign.name}`}
            width={192}
            height={192}
            unoptimized
          />
          <a
            className="button secondary compact"
            href={qr.data.image}
            download="nexa-qr.png"
          >
            Tải mã QR
          </a>
        </div>
      )}
      {campaign.url && (
        <a
          className="text-link"
          href={campaign.url}
          target="_blank"
          rel="noreferrer"
        >
          Mở trang chiến dịch
        </a>
      )}
      <dl className="integration-metrics">
        <div>
          <dt>Lượt mở trang</dt>
          <dd>{campaign.visits ?? 0}</dd>
        </div>
        <div>
          <dt>Khách gửi mã</dt>
          <dd>{campaign.leads ?? 0}</dd>
        </div>
        <div>
          <dt>Khách đạt ≥70 điểm</dt>
          <dd>{campaign.engaged ?? 0}</dd>
        </div>
      </dl>
      <p>
        Khách phản hồi sau khi nhà cung cấp nhận gửi · {campaign.replied ?? 0}/
        {campaign.accepted ?? 0}
        {!!campaign.accepted &&
          ` · ${new Intl.NumberFormat("vi-VN", { style: "percent", maximumFractionDigits: 1 }).format((campaign.replied ?? 0) / campaign.accepted)}`}
      </p>
      <h4>Khách tiềm năng</h4>
      <p>Gửi mã được 30 điểm. Mỗi tin phản hồi thêm 20 điểm, tối đa 100.</p>
      {leads.loading && <Loading />}
      {leads.data?.length === 0 && <p>Chưa có khách gửi mã.</p>}
      <ul className="integration-leads">
        {leads.data?.map((lead) => (
          <li key={lead.conversation_id}>
            <Link href={`/conversations/${lead.conversation_id}`}>
              {lead.name || "Mở hội thoại"} · {lead.score}/100
            </Link>
            {lead.intent && ` · ${lead.intent}`}
            {lead.unsubscribed ? " · Đã ngừng lời nhắc" : ""}
            {lead.sales_advice && (
              <p>
                <strong>AI Sales</strong> · {lead.sales_advice}
              </p>
            )}
          </li>
        ))}
      </ul>
      {(qr.error || leads.error) && (
        <p role="alert">{qr.error || leads.error}</p>
      )}
    </section>
  );
}

export function LeadCampaigns() {
  const list = useQuery(loadCampaigns);
  const [campaign, setCampaign] = useState<Campaign>(initial);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const current = list.data?.find((item) => item.id === campaign.id);
  return (
    <div className="integrations-grid">
      <section className="form-stack">
        <h3>Thiết lập chiến dịch</h3>
        <label>
          Chiến dịch
          <select
            value={campaign.id ?? ""}
            disabled={busy}
            onChange={(event) => {
              setError("");
              setMessage("");
              setCampaign(
                list.data?.find((item) => item.id === event.target.value) ??
                  initial,
              );
            }}
          >
            <option value="">Chiến dịch mới</option>
            {list.data?.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
                {item.enabled ? "" : " (đã tạm dừng)"}
              </option>
            ))}
          </select>
        </label>
        <form
          key={campaign.id ?? "new"}
          className="form-stack"
          onSubmit={async (event) => {
            event.preventDefault();
            const data = new FormData(event.currentTarget);
            setBusy(true);
            setMessage("");
            setError("");
            try {
              const saved = await api.request<Campaign>(path, {
                method: "POST",
                body: JSON.stringify({
                  id: campaign.id,
                  channel: campaign.channel,
                  name: data.get("name"),
                  phone: data.get("phone") ?? "",
                  welcome: data.get("welcome"),
                  follow_ups: [data.get("follow1"), data.get("follow2")].map(
                    (value) => String(value ?? "").trim(),
                  ),
                  enabled: data.has("enabled"),
                  follow_up_tickets: data.has("follow_up_tickets"),
                }),
              });
              setCampaign(saved);
              setMessage("Đã lưu chiến dịch.");
              list.reload();
            } catch (failure) {
              setError(errorMessage(failure));
            } finally {
              setBusy(false);
            }
          }}
        >
          <label>
            Tên chiến dịch
            <input
              name="name"
              defaultValue={campaign.name}
              required
              maxLength={80}
            />
          </label>
          <label>
            Kênh
            <select
              value={campaign.channel}
              disabled={!!campaign.id || busy}
              onChange={(event) =>
                setCampaign({
                  ...campaign,
                  channel: event.target.value as Campaign["channel"],
                })
              }
            >
              <option value="zalo">Zalo OA</option>
              <option value="whatsapp">WhatsApp</option>
            </select>
          </label>
          {campaign.channel === "whatsapp" && (
            <label>
              Số WhatsApp quốc tế, chỉ nhập chữ số
              <input
                name="phone"
                inputMode="tel"
                pattern="[1-9][0-9]{7,14}"
                defaultValue={campaign.phone}
                required
              />
            </label>
          )}
          <label>
            Lời chào sau khi khách gửi mã
            <textarea
              name="welcome"
              defaultValue={campaign.welcome}
              maxLength={500}
              required
            />
          </label>
          <label>
            Lời nhắc sau 1 giờ
            <textarea
              name="follow1"
              defaultValue={campaign.follow_ups?.[0]}
              maxLength={500}
            />
          </label>
          <label>
            Lời nhắc sau 8 giờ
            <textarea
              name="follow2"
              defaultValue={campaign.follow_ups?.[1]}
              maxLength={500}
            />
          </label>
          <p>
            Lời nhắc chỉ gửi khi khách đồng ý. Khách có thể gửi STOP để dừng.
          </p>
          <label className="check-row">
            <input
              type="checkbox"
              name="enabled"
              defaultChecked={campaign.enabled}
            />
            Bật chiến dịch
          </label>
          <label className="check-row">
            <input
              type="checkbox"
              name="follow_up_tickets"
              defaultChecked={campaign.follow_up_tickets}
            />
            Tạo phiếu khi khách yêu cầu tư vấn, báo giá hoặc hỗ trợ
          </label>
          <button className="button primary" disabled={busy}>
            {campaign.id ? "Lưu chiến dịch" : "Tạo mã QR"}
          </button>
        </form>
        {message && <p role="status">{message}</p>}
        {(error || list.error) && <p role="alert">{error || list.error}</p>}
      </section>
      {campaign.id ? (
        <CampaignResults key={campaign.id} campaign={current ?? campaign} />
      ) : (
        <section className="workspace-results">
          <h3>Mã QR và kết quả</h3>
          <p className="muted">
            Tạo hoặc chọn chiến dịch để tải mã QR và xem khách tiềm năng.
          </p>
        </section>
      )}
    </div>
  );
}
