import { CampaignLanding } from "@/modules/integrations/campaign-landing";

export default async function Page({
  params,
}: {
  params: Promise<{ tenant: string; id: string }>;
}) {
  const { tenant, id } = await params;
  return <CampaignLanding tenant={tenant} id={id} />;
}
