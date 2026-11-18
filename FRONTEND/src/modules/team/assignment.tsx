"use client";
import { useCallback, useState } from "react";
import { useQuery } from "@/shared/hooks/use-query";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
import { errorMessage } from "@/shared/api/http-client";

export function AssignmentFilter({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <select
      aria-label="Lọc theo phân công"
      value={value}
      onChange={(event) => onChange(event.target.value)}
    >
      <option value="">Mọi phân công</option>
      <option value="mine">Của tôi</option>
      <option value="unassigned">Chưa phân công</option>
    </select>
  );
}

export function Assignment({
  assignee,
  save,
}: {
  assignee?: string;
  save: (id: string, claim: boolean) => Promise<void>;
}) {
  const { auth } = useServices();
  const user = useUser();
  const load = useCallback(
    (signal: AbortSignal) => auth.members(signal),
    [auth],
  );
  const members = useQuery(load);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function assign(id: string, claim: boolean) {
    setBusy(true);
    setError("");
    try {
      await save(id, claim);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div>
      <select
        aria-label="Người phụ trách"
        value={assignee ?? ""}
        disabled={busy || !members.data}
        onChange={(event) => void assign(event.target.value, false)}
      >
        <option value="">Chưa phân công</option>
        {assignee &&
          !members.data?.some((member) => member.id === assignee) && (
            <option value={assignee}>Đã phân công</option>
          )}
        {members.data
          ?.filter((member) => !member.disabled || member.id === assignee)
          .map((member) => (
            <option key={member.id} value={member.id}>
              {member.name}
            </option>
          ))}
      </select>
      {!assignee && (
        <button
          type="button"
          className="button secondary compact"
          disabled={busy}
          onClick={() => void assign(user.id, true)}
        >
          Nhận việc
        </button>
      )}
      {(error || members.error) && <p role="alert">{error || members.error}</p>}
    </div>
  );
}
