import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "Nexa — Không gian chăm sóc khách hàng",
    template: "%s | Nexa",
  },
  description:
    "Quản lý khách hàng, hội thoại và tự động hóa trong một không gian làm việc.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="vi">
      <body>{children}</body>
    </html>
  );
}
