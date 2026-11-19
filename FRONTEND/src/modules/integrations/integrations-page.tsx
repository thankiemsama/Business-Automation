"use client";
import { FeatureSections } from "@/shared/ui/feature-sections";
import Link from "next/link";
import { useUser } from "@/shared/providers/workspace-provider";
import { PageHeading } from "@/shared/ui/primitives";
import { Icon } from "@/shared/ui/icon";
import { Webhooks } from "./webhooks-panel";
import { ApiKeys } from "./api-keys-panel";
import { Channels } from "./channels-panel";
import { LeadCampaigns } from "./lead-campaigns";

// Integrations hub: outgoing webhooks, public API keys, multi-channel
// configuration, and the public OpenAPI document. Owner-only.
export function IntegrationsPage() {
  const user = useUser();
  if (user.role !== "owner")
    return (
      <PageHeading
        title="Tích hợp"
        eyebrow="VẬN HÀNH"
        description="Chỉ quản trị viên được cấu hình tích hợp."
      />
    );
  return (
    <>
      <PageHeading
        title="Tích hợp & Kết nối"
        eyebrow="VẬN HÀNH"
        description="Kết nối Nexa với hệ thống của bạn qua webhook, API công khai và đa kênh."
      />
      <FeatureSections
        label="Nhóm tích hợp"
        sections={[
          {
            id: "api",
            label: "API công khai",
            content: (
              <section className="panel team-form">
                <h2>
                  <Icon name="globe" size={18} /> API công khai
                </h2>
                <p>
                  Tạo API key cho bên thứ ba (Zapier, n8n, app riêng). Key có
                  phạm vi đọc, giới hạn tốc độ riêng và xác thực bằng bearer
                  token.
                </p>
                <Link className="button secondary" href="/api-docs">
                  Xem OpenAPI / Swagger
                </Link>
                <ApiKeys />
              </section>
            ),
          },
          {
            id: "webhooks",
            label: "Webhook",
            content: (
              <section className="panel team-form">
                <h2>
                  <Icon name="bolt" size={18} /> Webhook đi (outgoing)
                </h2>
                <p>
                  Nhận sự kiện <code>ticket.created</code>,{" "}
                  <code>message.new</code>, <code>sla.breached</code>,{" "}
                  <code>csat.submitted</code>. Payload được ký bằng HMAC-SHA256
                  (header <code>X-Nexa-Signature</code>).
                </p>
                <Webhooks />
              </section>
            ),
          },
          {
            id: "lead-campaigns",
            label: "QR thu hút khách hàng",
            content: <LeadCampaigns />,
          },
          {
            id: "channels",
            label: "Kênh kết nối",
            content: (
              <section className="panel team-form">
                <h2>
                  <Icon name="chat" size={18} /> Đa kênh (multi-channel)
                </h2>
                <p>
                  Cấu hình kết nối các kênh hỗ trợ. Mỗi kênh có trình adapter
                  riêng và được lưu theo tenant.
                </p>
                <Channels />
              </section>
            ),
          },
        ]}
      />
    </>
  );
}
