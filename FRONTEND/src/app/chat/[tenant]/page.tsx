import { ChatWidget } from "@/modules/conversations/chat-widget";

export default async function ChatPage({
  params,
}: {
  params: Promise<{ tenant: string }>;
}) {
  const { tenant } = await params;
  return <ChatWidget tenant={tenant} />;
}
