"use client";
import Link from "next/link";
import { useLocale, useTranslations } from "next-intl";
export default function Help() {
  const locale = useLocale(),
    t = useTranslations("Shell"),
    english = locale === "en";
  return (
    <div className="form-stack">
      <h1>{t("help")}</h1>
      <section className="panel team-form form-stack">
        <h2>{t("tour")}</h2>
        <Link href="/conversations">
          {t("conversations")} ·{" "}
          {english
            ? "Reply and assign conversations"
            : "Trả lời và phân công hội thoại"}
        </Link>
        <Link href="/tickets">
          {t("tickets")} ·{" "}
          {english
            ? "Manage requests and deadlines"
            : "Quản lý yêu cầu và thời hạn"}
        </Link>
        <Link href="/security">
          {t("security")} ·{" "}
          {english
            ? "Protect your account and revoke sessions"
            : "Bảo vệ tài khoản và thu hồi phiên"}
        </Link>
      </section>
      <section className="panel team-form form-stack">
        <h2>{t("release")}</h2>
        <time dateTime="2026-11-18">18/11/2026</time>
        <ul>
          <li>
            {english
              ? "Two-factor authentication and active session management"
              : "Xác thực hai lớp và quản lý phiên đang hoạt động"}
          </li>
          <li>
            {english
              ? "Custom fields and workspace administration"
              : "Trường tùy chỉnh và cài đặt doanh nghiệp"}
          </li>
          <li>
            {english
              ? "Installable app with a privacy-preserving offline screen"
              : "Cài ứng dụng và trang ngoại tuyến bảo vệ dữ liệu"}
          </li>
        </ul>
      </section>
    </div>
  );
}
