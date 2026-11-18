import { ConversationDetail } from "@/modules/resources/workspace-detail";
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  return <ConversationDetail id={(await params).id} />;
}
