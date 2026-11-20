"use client";

import { useCallback, useEffect, useState } from "react";
import type { RealtimeQA } from "@/shared/api/repositories";
import { useQuery } from "@/shared/hooks/use-query";
import { useServices } from "@/shared/providers/services-provider";

const riskLabels: Record<string, string> = {
  cancellation: "Nguy cơ hủy",
  unfulfilled_promise: "Cam kết chưa được xử lý",
  procedure: "Quy trình",
  prohibited_language: "Ngôn từ",
  complaint: "Nguy cơ khiếu nại",
  unaddressed_emotion: "Cảm xúc chưa được xử lý",
};

export function RealtimeQACoaching({
  id,
  draft,
}: {
  id: string;
  draft: string;
}) {
  const { conversations } = useServices();
  const [consent, setConsent] = useState(false);
  const [review, setReview] = useState<{
    id: string;
    draft: string;
    result: RealtimeQA;
  }>();
  const load = useCallback(
    (signal: AbortSignal) => conversations.realtimeQAStatus(signal),
    [conversations],
  );
  const settings = useQuery(load);
  const enabled = settings.data?.enabled === true;

  useEffect(() => {
    const text = draft.trim();
    if (
      !enabled ||
      !consent ||
      text.length < 12 ||
      Array.from(text).length > 4000
    )
      return;
    const controller = new AbortController();
    const { signal } = controller;
    let unsubscribe = () => {};
    const timer = window.setTimeout(async () => {
      try {
        const request = await conversations.queueRealtimeQA(id, text, signal);
        if (signal.aborted) return;
        const refresh = async () => {
          try {
            const result = await conversations.realtimeQAResult(
              id,
              request.id,
              signal,
            );
            if (!signal.aborted) setReview({ id, draft, result });
          } catch {
            // Feedback is optional and must not interrupt the composer.
          }
        };
        const onRefresh = () => void refresh();
        window.addEventListener("workspace-refresh", onRefresh);
        unsubscribe = () =>
          window.removeEventListener("workspace-refresh", onRefresh);
        void refresh();
      } catch {
        // The agent can keep editing and send while coaching is unavailable.
      }
    }, 2200);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
      unsubscribe();
    };
  }, [consent, conversations, draft, enabled, id]);

  if (!enabled) return null;
  const result =
    consent && review?.id === id && review.draft === draft
      ? review.result
      : undefined;
  return (
    <section aria-label="QA thực thời">
      <label className="check-row">
        <input
          type="checkbox"
          checked={consent}
          onChange={(event) => setConsent(event.target.checked)}
        />
        Cho phép AI kiểm tra bản nháp và ngữ cảnh hội thoại
      </label>
      <div aria-live="polite">
        {result?.status === "quota" && <p>Đã hết quota AI hôm nay.</p>}
        {result?.status === "unavailable" && (
          <p>Chưa nhận được đánh giá chất lượng.</p>
        )}
        {result?.status === "completed" && result.labels.length > 0 && (
          <>
            <strong>Cảnh báo chất lượng</strong>
            <ul>
              {result.labels.map((label) => (
                <li key={label.kind}>
                  <strong>{riskLabels[label.kind]}</strong> · {label.message}
                </li>
              ))}
            </ul>
            {result.suggestion && <p>{result.suggestion}</p>}
          </>
        )}
      </div>
    </section>
  );
}
