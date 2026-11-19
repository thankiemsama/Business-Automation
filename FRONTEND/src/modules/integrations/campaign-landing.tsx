"use client";

import { useCallback, useState } from "react";
import { FetchHttpClient } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";

type Landing = {
  name: string;
  channel: "zalo" | "whatsapp";
  destination: string;
  message: string;
  follow_ups: number;
};

const api = new FetchHttpClient();

export function CampaignLanding({
  tenant,
  id,
}: {
  tenant: string;
  id: string;
}) {
  const load = useCallback(
    (signal: AbortSignal) =>
      api.request<Landing>(
        `lead/${encodeURIComponent(tenant)}/${encodeURIComponent(id)}`,
        { signal },
      ),
    [tenant, id],
  );
  const campaign = useQuery(load);
  const [followUp, setFollowUp] = useState(false);
  const [copied, setCopied] = useState(false);
  const item = campaign.data;
  const message = item
    ? item.message + (followUp ? ` OK${item.follow_ups}` : "")
    : "";
  const destination =
    item?.channel === "whatsapp"
      ? `${item.destination}?${new URLSearchParams({ text: message })}`
      : item?.destination;

  return (
    <main className="public-shell">
      <section className="panel team-form form-stack">
        <h1>{item?.name ?? "Kết nối với doanh nghiệp"}</h1>
        {campaign.loading && <p role="status">Đang mở mã chiến dịch…</p>}
        {campaign.error && <p role="alert">{campaign.error}</p>}
        {item && (
          <>
            {item.follow_ups > 0 && (
              <label className="check-row">
                <input
                  type="checkbox"
                  checked={followUp}
                  onChange={(event) => {
                    setFollowUp(event.target.checked);
                    setCopied(false);
                  }}
                />
                Tôi đồng ý nhận tối đa {item.follow_ups} lời nhắc tư vấn trong 8
                giờ. Tôi có thể gửi STOP để dừng.
              </label>
            )}
            {item.channel === "zalo" ? (
              <>
                <p>
                  Sao chép rồi gửi mã này trong cuộc trò chuyện Zalo để doanh
                  nghiệp nhận diện yêu cầu.
                </p>
                <input
                  aria-label="Mã gửi trong Zalo"
                  value={message}
                  readOnly
                  onFocus={(event) => event.target.select()}
                />
                <button
                  type="button"
                  className="button secondary"
                  onClick={() =>
                    void navigator.clipboard?.writeText(message).then(
                      () => setCopied(true),
                      () => setCopied(false),
                    )
                  }
                >
                  {copied ? "Đã sao chép" : "Sao chép mã"}
                </button>
              </>
            ) : (
              <p>Mở WhatsApp và gửi tin nhắn đã điền sẵn để bắt đầu.</p>
            )}
            <a className="button primary" href={destination} rel="noreferrer">
              Mở {item.channel === "zalo" ? "Zalo" : "WhatsApp"}
            </a>
          </>
        )}
      </section>
    </main>
  );
}
