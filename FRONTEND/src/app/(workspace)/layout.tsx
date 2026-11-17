import { WorkspaceProvider } from "@/shared/providers/workspace-provider";
export default function Layout({ children }: { children: React.ReactNode }) {
  return <WorkspaceProvider>{children}</WorkspaceProvider>;
}
