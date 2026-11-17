import Link from "next/link";
export default function NotFound() {
  return (
    <main className="boot-screen">
      <div className="empty-state">
        <span className="eyebrow">404</span>
        <h1>Trang này không tồn tại</h1>
        <p>Quay lại không gian làm việc để tiếp tục.</p>
        <Link href="/" className="button primary">
          Về tổng quan
        </Link>
      </div>
    </main>
  );
}
