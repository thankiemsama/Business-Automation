import { IntegrationsPage } from "@/modules/integrations/integrations-page";
import { WorkspaceFeature } from "@/shared/ui/workspace-feature";

export default function Page() {
  return (
    <WorkspaceFeature tone="integration">
      <IntegrationsPage />
    </WorkspaceFeature>
  );
}
