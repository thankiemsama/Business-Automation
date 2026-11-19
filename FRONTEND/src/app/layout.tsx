import { cookies } from "next/headers";
import { LocaleProvider } from "@/shared/i18n/provider";
import { messages } from "@/shared/i18n/messages";
import { PWAProvider } from "@/shared/ui/platform-tools";
import type { Metadata } from "next";
import "./globals.css";
import "./workspace-layout.css";

export const metadata: Metadata = {
  title: {
    default: "Nexa — Không gian chăm sóc khách hàng",
    template: "%s | Nexa",
  },
  description:
    "Quản lý khách hàng, hội thoại và tự động hóa trong một không gian làm việc.",
};

export default async function RootLayout({ children }: LayoutProps<"/">) {
  const locale =
    (await cookies()).get("nexa_locale")?.value === "en" ? "en" : "vi";
  return (
    <html lang={locale}>
      <body>
        <LocaleProvider locale={locale} messages={messages[locale]}>
          <PWAProvider />
          {children}
        </LocaleProvider>
      </body>
    </html>
  );
}
