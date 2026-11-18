import { text, type ModuleDefinition } from "../resources/module-definition";
export const knowledge: ModuleDefinition<"knowledge"> = {
  kind: "knowledge",
  title: "Kho kiến thức",
  singular: "bài kiến thức",
  icon: "book",
  ownerOnly: true,
  description: "Nguồn thông tin tin cậy cho những câu trả lời nhất quán.",
  defaults: { subject: "", content: "", enabled: true, public: false },
  fields: [
    {
      name: "public",
      label: "Xuất bản trong trung tâm trợ giúp công khai",
      type: "checkbox",
    },
    {
      name: "subject",
      label: "Tiêu đề bài viết",
      required: true,
      placeholder: "Ví dụ: Chính sách đổi trả",
    },
    {
      name: "content",
      label: "Nội dung phản hồi",
      type: "textarea",
      required: true,
      placeholder: "Thông tin sẽ được dùng khi phản hồi khách hàng…",
    },
    {
      name: "enabled",
      label: "Cho phép sử dụng trong phản hồi tự động",
      type: "checkbox",
    },
  ],
  serialize: (values) => ({
    subject: text(values, "subject"),
    content: text(values, "content"),
    enabled: !!values.enabled,
    public: !!values.public,
  }),
};
