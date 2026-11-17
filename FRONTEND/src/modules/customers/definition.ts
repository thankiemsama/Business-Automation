import { text, type ModuleDefinition } from "../resources/module-definition";
export const customers: ModuleDefinition<"customers"> = {
  kind: "customers",
  title: "Khách hàng",
  singular: "khách hàng",
  icon: "users",
  description:
    "Hiểu khách hàng hơn, kết nối tốt hơn. Mọi thông tin ở cùng một nơi.",
  defaults: { name: "", email: "", phone: "" },
  fields: [
    {
      name: "name",
      label: "Họ và tên",
      required: true,
      placeholder: "Nguyễn Minh Anh",
    },
    {
      name: "email",
      label: "Email",
      type: "email",
      placeholder: "khachhang@example.com",
    },
    { name: "phone", label: "Số điện thoại", placeholder: "090 123 4567" },
  ],
  serialize: (values) => ({
    name: text(values, "name"),
    email: text(values, "email"),
    phone: text(values, "phone"),
  }),
};
