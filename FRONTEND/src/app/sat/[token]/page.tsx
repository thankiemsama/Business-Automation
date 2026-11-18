import { CSATPage } from "@/modules/sat/sat-page";

export const metadata = {
  title: "Đánh giá dịch vụ | Nexa",
  description: "Xin dành ít giây đánh giá trải nghiệm hỗ trợ của bạn.",
  robots: { index: false, follow: true },
};

export default async function Page({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;
  return <CSATPage token={token} />;
}
