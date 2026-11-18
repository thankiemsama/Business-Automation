import {
  text,
  ticketStatuses,
  type ModuleDefinition,
} from "../resources/module-definition";
import { parseTags } from "@/shared/api/workspace";
import type { Ticket } from "@/shared/domain/models";
export const tickets: ModuleDefinition<"tickets"> = {
  kind: "tickets",
  title: "Phiếu hỗ trợ",
  singular: "phiếu hỗ trợ",
  icon: "ticket",
  description:
    "Từ yêu cầu đầu tiên đến khi được giải quyết. Không bỏ lỡ điều gì.",
  defaults: {
    tags: "",
    subject: "",
    content: "",
    customer_id: "",
    conversation_id: "",
    status: "open",
    priority: "normal",
  },
  fields: [
    { name: "tags", label: "Nhãn (phân cách bằng dấu phẩy)" },
    {
      name: "subject",
      label: "Tiêu đề",
      required: true,
      placeholder: "Tóm tắt yêu cầu hỗ trợ",
    },
    { name: "content", label: "Nội dung", type: "textarea" },
    { name: "customer_id", label: "Khách hàng (tùy chọn)", type: "customer" },
    {
      name: "conversation_id",
      label: "Hội thoại liên quan (tùy chọn)",
      type: "conversation",
    },
    {
      name: "status",
      label: "Trạng thái",
      type: "select",
      options: ticketStatuses,
    },
    {
      name: "priority",
      label: "Độ ưu tiên",
      type: "select",
      options: [
        ["low", "Thấp"],
        ["normal", "Bình thường"],
        ["high", "Cao"],
        ["urgent", "Khẩn cấp"],
      ],
    },
  ],
  serialize: (values) => ({
    tags: parseTags(text(values, "tags")),
    subject: text(values, "subject"),
    content: text(values, "content"),
    customer_id: text(values, "customer_id"),
    conversation_id: text(values, "conversation_id"),
    status: text(values, "status") as Ticket["status"],
    priority: text(values, "priority") as Ticket["priority"],
  }),
};
