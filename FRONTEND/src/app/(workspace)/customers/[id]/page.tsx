import { WorkspaceDetail } from "@/modules/resources/workspace-detail";
import { CustomerProfilePanel } from "@/modules/customers/customer-profile";

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <>
      <WorkspaceDetail kind="customers" id={id} />
      <CustomerProfilePanel id={id} />
    </>
  );
}
