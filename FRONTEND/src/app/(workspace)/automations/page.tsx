import { ResourcePage } from "@/modules/resources/resource-page";
import { WorkspaceFeature } from "@/shared/ui/workspace-feature";

export default function Page() {
  return (
    <WorkspaceFeature tone="automation">
      <ResourcePage kind="automations" />
    </WorkspaceFeature>
  );
}
