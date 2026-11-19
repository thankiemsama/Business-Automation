"use client";
import { useUser } from "@/shared/providers/workspace-provider";
import { FeatureSections } from "@/shared/ui/feature-sections";
import { ErrorState, PageHeading } from "@/shared/ui/primitives";
import {
  AccessPanel,
  BackupPanel,
  BillingPanel,
  CRMPanel,
  AuditPanel,
} from "./admin-panels";
import { FieldsAdmin } from "./custom-values";

export function AdminPage() {
  const user = useUser();
  if (user.role !== "owner")
    return <ErrorState message="Chỉ chủ doanh nghiệp có quyền quản trị." />;
  return (
    <>
      <PageHeading
        title="Cài đặt doanh nghiệp"
        description="Quản lý truy cập, dữ liệu và dịch vụ của doanh nghiệp."
      />
      <FeatureSections
        label="Cài đặt doanh nghiệp"
        sections={[
          {
            id: "access",
            label: "Truy cập và thương hiệu",
            content: <AccessPanel />,
          },
          {
            id: "backups",
            label: "Dữ liệu và sao lưu",
            content: <BackupPanel />,
          },
          { id: "billing", label: "Gói dịch vụ", content: <BillingPanel /> },
          { id: "crm", label: "CRM", content: <CRMPanel /> },
          { id: "fields", label: "Trường tùy chỉnh", content: <FieldsAdmin /> },
          { id: "audit", label: "Nhật ký thay đổi", content: <AuditPanel /> },
        ]}
      />
    </>
  );
}
