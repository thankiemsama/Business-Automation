import { WorkspaceDetail } from "@/modules/resources/workspace-detail";
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  return <WorkspaceDetail kind="tickets" id={(await params).id} />;
}
