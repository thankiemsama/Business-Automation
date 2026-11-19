"use client";
import Link from "next/link";
import { FeatureSections } from "@/shared/ui/feature-sections";
import { useUser } from "@/shared/providers/workspace-provider";
import { ErrorState, PageHeading } from "@/shared/ui/primitives";
import { WorkflowPanel } from "./workflow-panel";
import { SchedulePanel } from "./schedule-panel";

export function WorkflowPage() {
  const user = useUser();
  if (user.role !== "owner")
    return (
      <ErrorState message="Chỉ chủ doanh nghiệp được quản lý workflow và lịch gửi." />
    );
  return (
    <>
      <PageHeading
        title="Tự động hóa"
        description="Quản lý workflow và lập lịch báo cáo."
      >
        <Link className="button secondary" href="/automations">
          Quy tắc từ khóa
        </Link>
      </PageHeading>
      <FeatureSections
        label="Tự động hóa"
        sections={[
          { id: "rules", label: "Workflow", content: <WorkflowPanel /> },
          {
            id: "schedules",
            label: "Lịch gửi báo cáo",
            content: <SchedulePanel />,
          },
        ]}
      />
    </>
  );
}
