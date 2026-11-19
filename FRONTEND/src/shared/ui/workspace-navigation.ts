import type { IconName } from "./icon";

export const navigation: {
  href: string;
  title: string;
  icon: IconName;
  group: string;
}[] = [
  { href: "/", title: "Tổng quan", icon: "grid", group: "Hỗ trợ" },
  {
    href: "/conversations",
    title: "Hộp hội thoại",
    icon: "chat",
    group: "Hỗ trợ",
  },
  {
    href: "/customers",
    title: "Khách hàng",
    icon: "users",
    group: "Hỗ trợ",
  },
  {
    href: "/tickets",
    title: "Phiếu hỗ trợ",
    icon: "ticket",
    group: "Hỗ trợ",
  },
  {
    href: "/knowledge",
    title: "Kho kiến thức",
    icon: "book",
    group: "Công cụ",
  },
  {
    href: "/automations",
    title: "Quy tắc từ khóa",
    icon: "bolt",
    group: "Công cụ",
  },
  {
    href: "/integrations",
    title: "Tích hợp",
    icon: "globe",
    group: "Công cụ",
  },
  {
    href: "/workflows",
    title: "Workflow và lịch gửi",
    icon: "bolt",
    group: "Công cụ",
  },
  { href: "/admin", title: "Quản trị", icon: "shield", group: "Cài đặt" },
  { href: "/security", title: "Bảo mật", icon: "shield", group: "Cài đặt" },
  { href: "/team", title: "Đội ngũ", icon: "shield", group: "Cài đặt" },
];
