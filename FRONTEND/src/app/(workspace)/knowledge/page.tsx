import { ResourcePage } from "@/modules/resources/resource-page";
import { WorkspaceFeature } from "@/shared/ui/workspace-feature";

export default function Page() {
  return (
    <WorkspaceFeature tone="knowledge">
      <ResourcePage kind="knowledge" />
    </WorkspaceFeature>
  );
}
