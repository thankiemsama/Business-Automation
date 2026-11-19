import { AdminPage } from "@/modules/admin/admin-page";
import { WorkspaceFeature } from "@/shared/ui/workspace-feature";

export default function Page() {
  return (
    <WorkspaceFeature tone="admin">
      <AdminPage />
    </WorkspaceFeature>
  );
}
