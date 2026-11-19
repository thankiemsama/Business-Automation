"use client";
import { useCallback, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";
import { ErrorState } from "@/shared/ui/primitives";

export function useActiveMembers() {
  const { auth } = useServices();
  const memberLoad = useCallback(
    (signal: AbortSignal) => auth.members(signal),
    [auth],
  );
  const members = useQuery(memberLoad);
  const people = members.data?.filter((member) => !member.disabled) ?? [];

  return { members, people };
}

export function useOperationMutation() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  async function mutate(
    action: () => Promise<unknown>,
    reload: () => void,
    message: string,
  ) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
      reload();
      setNotice(message);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  return { busy, error, notice, mutate };
}

export function OperationFeedback({
  error,
  notice,
}: {
  error?: string;
  notice: string;
}) {
  return (
    <>
      {error && <ErrorState message={error} />}
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
    </>
  );
}
