"use client";

import { useState } from "react";
import Link from "next/link";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";

type Plan = {
  id: string;
  action: "handoff" | "resolve_ticket" | "send_csat";
  reason: string;
  status: "proposed" | "handed_off" | "applied";
};

const api = new FetchHttpClient();
const loadStatus = (signal: AbortSignal) =>
  api.request<{ enabled: boolean; can_resolve: boolean }>(
    "ai/ticket-agent/status",
    { signal },
  );

const actions = {
  handoff: "Chuyển cho nhân viên xử lý",
  resolve_ticket: "Đánh dấu phiếu đã giải quyết",
  send_csat: "Xếp hàng gửi khảo sát CSAT",
};

export function TicketAgent({
  id,
  onApplied,
}: {
  id: string;
  onApplied: () => void;
}) {
  const status = useQuery(loadStatus);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const path = `ai/ticket-agent/${encodeURIComponent(id)}`;

  if (!status.data?.enabled) return null;

  async function run(apply: boolean) {
    setBusy(true);
    setError("");
    try {
      const next = await api.request<Plan>(
        `${path}/${apply ? "apply" : "plan"}`,
        {
          method: "POST",
          ...(apply && plan
            ? { body: JSON.stringify({ plan_id: plan.id }) }
            : {}),
        },
      );
      setPlan(next);
      if (apply) onApplied();
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setBusy(false);
    }
  }

  return (
    <details className="subtle-note">
      <summary>Agent xử lý phiếu</summary>
      {!status.data.can_resolve && (
        <p>
          Để agent giải quyết phiếu, cần{" "}
          <Link href="/sparring">chứng chỉ AI Sparring</Link> còn hiệu lực.
        </p>
      )}
      {plan && (
        <>
          <p>
            <strong>{actions[plan.action]}</strong>
          </p>
          <p>{plan.reason}</p>
          {plan.status === "applied" && <p role="status">Đã thực hiện.</p>}
        </>
      )}
      <div className="toolbar-controls">
        <button
          type="button"
          className="button secondary compact"
          disabled={busy}
          onClick={() => void run(false)}
        >
          {busy ? "Đang xử lý…" : "Phân tích phiếu"}
        </button>
        {plan?.status === "proposed" && (
          <button
            type="button"
            className="button primary compact"
            disabled={busy}
            onClick={() => void run(true)}
          >
            Duyệt và thực hiện
          </button>
        )}
      </div>
      {error && <p role="alert">{error}</p>}
    </details>
  );
}
