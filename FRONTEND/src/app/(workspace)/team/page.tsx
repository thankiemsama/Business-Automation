import { TeamPage } from "@/modules/team/team-page";
import { WorkspaceFeature } from "@/shared/ui/workspace-feature";

export default function Page() {
  return (
    <WorkspaceFeature tone="team">
      <TeamPage />
    </WorkspaceFeature>
  );
}
