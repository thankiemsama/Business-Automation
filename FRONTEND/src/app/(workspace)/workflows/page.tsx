import { WorkflowPage } from "@/modules/operations/workflow-page";
import { WorkspaceFeature } from "@/shared/ui/workspace-feature";

export default function Page() {
  return (
    <WorkspaceFeature tone="automation">
      <WorkflowPage />
    </WorkspaceFeature>
  );
}
