"use client";
import { FeatureSections } from "@/shared/ui/feature-sections";
import Link from "next/link";
import type { ReactNode } from "react";
import { useUser } from "@/shared/providers/workspace-provider";
import { PageHeading } from "@/shared/ui/primitives";
import { Icon, type IconName } from "@/shared/ui/icon";
import { Webhooks } from "./webhooks-panel";
import { ApiKeys } from "./api-keys-panel";
import { Channels } from "./channels-panel";
import { LeadCampaigns } from "./lead-campaigns";
import styles from "./integrations.module.css";

function IntegrationSection({
  title,
  description,
  icon,
  action,
  children,
}: {
  title: string;
  description: ReactNode;
  icon: IconName;
  action?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className={styles.section}>
      <header className={`panel ${styles.header}`}>
        <span className={styles.icon}>
          <Icon name={icon} size={22} />
        </span>
        <div>
          <h2>{title}</h2>
          <p>{description}</p>
        </div>
        {action && <div className={styles.headerAction}>{action}</div>}
      </header>
      {children}
    </section>
  );
}

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
    <div className={styles.page}>
      <PageHeading
        title="Tích hợp & Kết nối"
        description="Quản lý khóa API, webhook, chiến dịch QR và các kênh hỗ trợ."
      />
      <FeatureSections
        label="Nhóm tích hợp"
        sections={[
          {
            id: "api",
            label: "API công khai",
            content: (
              <IntegrationSection
                title="API công khai"
                icon="globe"
                description="Cấp quyền đọc cho ứng dụng bên ngoài qua Bearer token."
                action={
                  <Link className="button secondary compact" href="/api-docs">
                    <Icon name="book" size={16} /> Tài liệu API
                  </Link>
                }
              >
                <ApiKeys />
              </IntegrationSection>
            ),
          },
          {
            id: "webhooks",
            label: "Webhook",
            content: (
              <IntegrationSection
                title="Webhook gửi đi"
                icon="bolt"
                description={
                  <>
                    Gửi sự kiện đến hệ thống của bạn. Xác thực chữ ký
                    HMAC-SHA256 qua <code>X-Nexa-Signature</code>.
                  </>
                }
              >
                <Webhooks />
              </IntegrationSection>
            ),
          },
          {
            id: "lead-campaigns",
            label: "QR thu hút khách hàng",
            content: (
              <IntegrationSection
                title="Chiến dịch QR"
                icon="users"
                description="Tạo mã QR cho Zalo OA hoặc WhatsApp và theo dõi khách tiềm năng."
              >
                <LeadCampaigns />
              </IntegrationSection>
            ),
          },
          {
            id: "channels",
            label: "Kênh kết nối",
            content: (
              <IntegrationSection
                title="Kênh kết nối"
                icon="chat"
                description="Cấu hình từng kênh và theo dõi trạng thái gửi của Zalo OA hoặc WhatsApp."
              >
                <Channels />
              </IntegrationSection>
            ),
          },
        ]}
      />
    </div>
  );
}
