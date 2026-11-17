import { text, type ModuleDefinition } from "../resources/module-definition";
export const automations: ModuleDefinition<"automations"> = {
  kind: "automations",
  title: "Tự động hóa",
  singular: "quy tắc",
  icon: "bolt",
  ownerOnly: true,
  description: "Biến những thao tác lặp lại thành quy trình hỗ trợ chủ động.",
  defaults: { name: "", keywords: "", enabled: true },
  fields: [
    {
      name: "name",
      label: "Tên quy tắc",
      required: true,
      placeholder: "Ví dụ: Tiếp nhận yêu cầu hoàn tiền",
    },
    {
      name: "keywords",
      label: "Từ khóa kích hoạt (phân cách bằng dấu phẩy)",
      required: true,
      placeholder: "hoàn tiền, khiếu nại, đổi trả",
    },
    { name: "enabled", label: "Kích hoạt quy tắc", type: "checkbox" },
  ],
  serialize: (values) => ({
    name: text(values, "name"),
    keywords: text(values, "keywords")
      .split(",")
      .map((word) => word.trim())
      .filter(Boolean),
    enabled: !!values.enabled,
  }),
};
