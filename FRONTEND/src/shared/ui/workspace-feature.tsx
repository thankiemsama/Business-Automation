import type { ReactNode } from "react";
import styles from "./workspace-feature.module.css";

export function WorkspaceFeature({
  children,
  tone,
}: {
  children: ReactNode;
  tone:
    "knowledge" | "automation" | "integration" | "admin" | "security" | "team";
}) {
  return (
    <div className={styles.feature} data-tone={tone}>
      {children}
    </div>
  );
}
