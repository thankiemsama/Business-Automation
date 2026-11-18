import { PortalPage } from "@/modules/portal/portal-page";

export const metadata = {
  title: "Cổng khách hàng | Nexa",
  description:
    "Tạo phiếu hỗ trợ, theo dõi trạng thái và gửi đánh giá trong một nơi.",
  robots: { index: false, follow: true },
};

export default async function Page({
  params,
}: {
  params: Promise<{ tenant: string }>;
}) {
  const { tenant } = await params;
  return <PortalPage tenant={tenant} />;
}
