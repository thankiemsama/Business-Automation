"use client";
import { useEffect, useId, useRef, type ReactNode } from "react";
import { useTranslations } from "next-intl";
import { Icon, type IconName } from "./icon";
export function Brand() {
  return (
    <span className="brand">
      <span className="brand-mark">n</span>nexa
      <span className="brand-dot">.</span>
    </span>
  );
}
export function Avatar({
  name,
  small = false,
}: {
  name: string;
  small?: boolean;
}) {
  return (
    <span className={`avatar ${small ? "small" : ""}`}>
      {name
        .split(" ")
        .filter(Boolean)
        .slice(-2)
        .map((word) => word[0])
        .join("")
        .toUpperCase() || "?"}
    </span>
  );
}
const labels: Record<string, string> = {
  open: "Đang mở",
  closed: "Đã đóng",
  in_progress: "Đang xử lý",
  resolved: "Đã giải quyết",
  low: "Thấp",
  normal: "Bình thường",
  high: "Cao",
  urgent: "Khẩn cấp",
  web: "Website",
  email: "Email",
  phone: "Điện thoại",
};
export function Badge({ value }: { value: string }) {
  const t=useTranslations("Common");
  return (
    <span className={`badge badge-${value}`}>
      <span />
      {t.has(value) ? t(value) : labels[value] || value}
    </span>
  );
}
export function EmptyState({
  title,
  description,
  icon = "grid",
  children,
}: {
  title?: string;
  description?: string;
  icon?: IconName;
  children?: ReactNode;
}) {
  const t=useTranslations("Common");
  return (
    <div className="empty-state">
      <span className="empty-icon">
        <Icon name={icon} size={27} />
      </span>
      <h3>{title??t("empty")}</h3>
      <p>{description??t("start")}</p>
      {children}
    </div>
  );
}
export function ErrorState({
  message,
  retry,
}: {
  message: string;
  retry?: () => void;
}) {
  const t=useTranslations("Common");
  return (
    <div className="error-state" role="alert">
      <Icon name="info" />
      <span>{message}</span>
      {retry && (
        <button className="button secondary compact" onClick={retry}>
          {t("retry")}
        </button>
      )}
    </div>
  );
}
export function Loading() {
 const t=useTranslations("Common");
  return (
    <div className="loading" role="status">
      <span className="spinner" />
      {t("loading")}
    </div>
  );
}
export function PageHeading({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow: string;
  title: string;
  description: string;
  children?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        <span className="eyebrow">{eyebrow}</span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      <div className="heading-actions">{children}</div>
    </div>
  );
}
export function Pagination({
  page,
  total,
  limit,
  onChange,
}: {
  page: number;
  total: number;
  limit: number;
  onChange: (page: number) => void;
}) {
  const t=useTranslations("Common");
  const last = Math.max(1, Math.ceil(total / limit));
  return (
    <div className="pagination">
      <span>
        {total
          ? `${(page - 1) * limit + 1}–${Math.min(page * limit, total)}`
          : "0"}{" "}
        {t("results",{total})}
      </span>
      <div>
        <button
          aria-label={t("previous")}
          disabled={page <= 1}
          onClick={() => onChange(page - 1)}
        >
          ←
        </button>
        <span>
          {t("page",{page,last})}
        </span>
        <button
          aria-label={t("next")}
          disabled={page >= last}
          onClick={() => onChange(page + 1)}
        >
          →
        </button>
      </div>
    </div>
  );
}
export function Modal({
  title,
  description,
  onClose,
  children,
}: {
  title: string;
  description?: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const t=useTranslations("Common");
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    const dialog = ref.current;
    dialog?.showModal();
    return () => dialog?.close();
  }, []);
  return (
    <dialog
      aria-labelledby={titleId}
      ref={ref}
      className="modal"
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="modal-header">
        <div>
          <h2 id={titleId}>{title}</h2>
          {description && <p>{description}</p>}
        </div>
        <button
          type="button"
          className="icon-button"
          onClick={onClose}
          aria-label={t("close")}
        >
          <Icon name="close" />
        </button>
      </div>
      {children}
    </dialog>
  );
}
export function dateLabel(value: string) {
  return new Intl.DateTimeFormat("vi-VN", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(new Date(value));
}
