import {
  text,
  conversationStatuses,
  type ModuleDefinition,
} from "../resources/module-definition";
import { parseTags } from "@/shared/api/workspace";
import type { Conversation } from "@/shared/domain/models";
export const conversations: ModuleDefinition<"conversations"> = {
  kind: "conversations",
  title: "Hộp hội thoại",
  singular: "hội thoại",
  icon: "chat",
  description: "Lắng nghe, phản hồi và giữ mọi kết nối luôn liền mạch.",
  defaults: {
    tags: "",
    customer_id: "",
    subject: "",
    channel: "web",
    status: "open",
  },
  fields: [
    { name: "tags", label: "Nhãn (phân cách bằng dấu phẩy)" },
    {
      name: "customer_id",
      label: "Khách hàng",
      type: "customer",
      required: true,
      immutable: true,
    },
    {
      name: "subject",
      label: "Chủ đề",
      placeholder: "Khách hàng cần hỗ trợ điều gì?",
    },
    {
      name: "channel",
      label: "Kênh liên hệ",
      type: "select",
      options: [
        ["web", "Website"],
        ["email", "Email"],
        ["phone", "Điện thoại"],
      ],
    },
    {
      name: "status",
      label: "Trạng thái",
      type: "select",
      options: conversationStatuses,
    },
  ],
  serialize: (values) => ({
    tags: parseTags(text(values, "tags")),
    customer_id: text(values, "customer_id"),
    subject: text(values, "subject"),
    channel: text(values, "channel") as Conversation["channel"],
    status: text(values, "status") as Conversation["status"],
  }),
};
