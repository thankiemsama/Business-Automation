import { SecurityPage } from "@/modules/admin/security-page";
import { WorkspaceFeature } from "@/shared/ui/workspace-feature";

export default function Page() {
  return (
    <WorkspaceFeature tone="security">
      <SecurityPage />
    </WorkspaceFeature>
  );
}
