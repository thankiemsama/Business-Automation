"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";

const api = new FetchHttpClient();

export function AIKnowledgeSearch({
  onSelect,
}: {
  onSelect: (id: string) => void | Promise<void>;
}) {
  const [input, setInput] = useState("");
  const [q, setQ] = useState("");
  const load = useCallback(
    (signal: AbortSignal) =>
      q
        ? api.request<{ id: string; subject: string; video_url?: string }[]>(
            `ai/search?${new URLSearchParams({ q })}`,
            { signal },
          )
        : Promise.resolve([]),
    [q],
  );
  const result = useQuery(load);
  return (
    <section className="panel team-form">
      <h2>Tìm theo ngữ nghĩa</h2>
      <form
        className="toolbar-controls"
        onSubmit={(event) => {
          event.preventDefault();
          const term = input.trim();
          if (term === q) result.reload();
          else setQ(term);
        }}
      >
        <input
          aria-label="Truy vấn tìm kiếm ngữ nghĩa"
          value={input}
          maxLength={1000}
          onChange={(event) => setInput(event.target.value)}
          placeholder="Nhập câu hỏi của khách…"
        />
        <button
          className="button secondary compact"
          disabled={!input.trim() || Boolean(q && result.loading)}
        >
          Tìm bài liên quan
        </button>
      </form>
      {result.error && <p role="alert">{result.error}</p>}
      {q && !result.loading && !result.error && !result.data?.length && (
        <p>Chưa tìm thấy bài phù hợp.</p>
      )}
      <ul>
        {result.data?.map((item) => (
          <li key={item.id}>
            <button
              type="button"
              className="text-link"
              onClick={() => void onSelect(item.id)}
            >
              {item.subject}
            </button>
            {item.video_url && (
              <>
                {" "}
                ·{" "}
                <a
                  className="text-link"
                  href={item.video_url}
                  target="_blank"
                  rel="noreferrer"
                >
                  Mở video
                </a>
              </>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

export function AISummary({
  kind,
  id,
  saved,
}: {
  kind: "tickets" | "conversations";
  id: string;
  saved?: string;
}) {
  const load = useCallback(
    (signal: AbortSignal) =>
      api.request<{ summary: string }>(
        `ai/summary/${kind}/${encodeURIComponent(id)}`,
        { method: "POST", signal },
      ),
    [kind, id],
  );
  const result = useQuery(load);
  const summary = saved || result.data?.summary;
  if (!summary && !result.error) return null;
  return (
    <details className="subtle-note">
      <summary>Tóm tắt AI</summary>
      {summary && (
        <ul>
          {summary.split("\n").map((sentence, index) => (
            <li key={index}>{sentence}</li>
          ))}
        </ul>
      )}
      {result.error && <p role="alert">{result.error}</p>}
      <button
        className="text-link"
        type="button"
        onClick={result.reload}
        disabled={result.loading}
      >
        Cập nhật tóm tắt
      </button>
    </details>
  );
}

export function AITicketSuggestion({
  id,
  onApplied,
}: {
  id: string;
  onApplied: () => void;
}) {
  const path = `ai/suggestion/${encodeURIComponent(id)}`;
  const load = useCallback(
    (signal: AbortSignal) =>
      api.request<{ tags: string[]; priority: string } | null>(path, {
        signal,
      }),
    [path],
  );
  const result = useQuery(load, "workspace-refresh");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  if (!result.data && !error) return null;
  return (
    <section className="subtle-note">
      {result.data && (
        <div>
          <strong>Đề xuất phân loại AI</strong>
          <p>
            Nhãn: {result.data.tags.join(", ") || "—"} · Ưu tiên:{" "}
            {{ low: "Thấp", normal: "Thường", high: "Cao", urgent: "Khẩn" }[
              result.data.priority
            ] ?? result.data.priority}
          </p>
          <button
            type="button"
            className="button secondary compact"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              setError("");
              try {
                await api.request(path, { method: "POST" });
                result.reload();
                onApplied();
              } catch (error) {
                setError(errorMessage(error));
              } finally {
                setBusy(false);
              }
            }}
          >
            Xác nhận và áp dụng
          </button>
        </div>
      )}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}

const reviewStatus: Record<string, string> = {
  pending: "Đang chờ kiểm tra",
  running: "Đang kiểm tra",
  insufficient_evidence: "Thiếu dữ liệu để chấm điểm",
  failed: "Kiểm tra thất bại",
  disabled: "AI đã tắt cho doanh nghiệp",
  superseded: "Phiếu đã thay đổi trạng thái",
};

export function AITicketReview({ id }: { id: string }) {
  const load = useCallback(
    (signal: AbortSignal) =>
      api.request<{
        status: string;
        score?: number;
        findings?: string[];
        customer_impression?: string;
      } | null>(`ai/review/${encodeURIComponent(id)}`, { signal }),
    [id],
  );
  const result = useQuery(load, "workspace-refresh");
  const review = result.data;
  if (!review && !result.error) return null;
  return (
    <details className="subtle-note">
      <summary>Kiểm tra chất lượng bằng AI</summary>
      {review && (
        <>
          <p>
            {review.score != null
              ? `Điểm chất lượng ${review.score}/100`
              : reviewStatus[review.status]}
          </p>
          {review.customer_impression && (
            <p>Ghi nhận về khách hàng · {review.customer_impression}</p>
          )}
          {!!review.findings?.length && (
            <ul>
              {review.findings.map((finding, index) => (
                <li key={index}>{finding}</li>
              ))}
            </ul>
          )}
        </>
      )}
      {result.error && <p role="alert">{result.error}</p>}
      <button
        type="button"
        className="text-link"
        onClick={result.reload}
        disabled={result.loading}
      >
        Cập nhật kết quả
      </button>
    </details>
  );
}

export function VoiceInput({
  disabled,
  onText,
  guest,
}: {
  disabled: boolean;
  guest?: { path: string; token: string };
  onText: (text: string) => void;
}) {
  const guestPath = guest?.path;
  const guestToken = guest?.token;
  const load = useCallback(
    (signal: AbortSignal) =>
      api.request<{ enabled: boolean; transcription?: boolean }>(
        guestPath ? `${guestPath}/voice-status` : "ai/status",
        {
          signal,
          headers: guestToken
            ? { Authorization: `Bearer ${guestToken}` }
            : undefined,
        },
      ),
    [guestPath, guestToken],
  );
  const status = useQuery(load);
  const [recording, setRecording] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [analysis, setAnalysis] = useState<{
    intent?: string;
    sentiment?: string;
  }>({});
  const active = useRef<MediaRecorder | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      if (timer.current) clearTimeout(timer.current);
      active.current?.stream.getTracks().forEach((track) => track.stop());
    };
  }, []);
  if (!status.data?.enabled || (!guestPath && !status.data.transcription))
    return null;
  async function transcribe(file: Blob) {
    setError("");
    setAnalysis({});
    if (!file.size || file.size > 4 * 1024 * 1024) {
      setError("Âm thanh phải có dữ liệu và tối đa 4 MiB.");
      return;
    }
    setBusy(true);
    try {
      const data = await api.request<{
        text: string;
        intent?: string;
        sentiment?: string;
      }>(guestPath ? `${guestPath}/transcribe` : "ai/transcribe", {
        headers: guestToken
          ? { Authorization: `Bearer ${guestToken}` }
          : undefined,
        method: "POST",
        body: file,
      });
      if (mounted.current) {
        onText(data.text);
        setAnalysis(data);
      }
    } catch (error) {
      if (mounted.current) setError(errorMessage(error));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  async function record() {
    if (active.current?.state === "recording") {
      active.current.stop();
      return;
    }
    setError("");
    setBusy(true);
    let stream: MediaStream | undefined;
    try {
      if (
        !navigator.mediaDevices?.getUserMedia ||
        typeof MediaRecorder === "undefined"
      )
        throw new Error(
          "Trình duyệt không hỗ trợ ghi âm. Bạn có thể chọn tệp.",
        );
      stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (!mounted.current) {
        stream.getTracks().forEach((track) => track.stop());
        return;
      }
      const recorder = new MediaRecorder(stream);
      active.current = recorder;
      const chunks: Blob[] = [];
      recorder.ondataavailable = (event) => {
        if (event.data.size) chunks.push(event.data);
      };
      recorder.onstop = () => {
        recorder.stream.getTracks().forEach((track) => track.stop());
        if (timer.current) clearTimeout(timer.current);
        active.current = null;
        if (!mounted.current) return;
        setRecording(false);
        void transcribe(new Blob(chunks, { type: recorder.mimeType }));
      };
      recorder.start();
      timer.current = setTimeout(() => {
        if (recorder.state === "recording") recorder.stop();
      }, 45000);
      setRecording(true);
    } catch (error) {
      stream?.getTracks().forEach((track) => track.stop());
      setError(errorMessage(error));
    } finally {
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <div className="toolbar-controls">
      <button
        type="button"
        className="button secondary compact"
        disabled={!recording && (disabled || busy)}
        onClick={() => void record()}
      >
        {recording ? "Dừng ghi âm" : "Ghi âm (tối đa 45 giây)"}
      </button>
      <label>
        Hoặc chọn tệp âm thanh
        <input
          type="file"
          accept="audio/webm,audio/ogg,audio/mp4,audio/wav,audio/mpeg,.m4a"
          disabled={recording || busy || disabled}
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (file) void transcribe(file);
          }}
        />
      </label>
      {busy && !recording && <span role="status">Đang xử lý âm thanh…</span>}
      {!guestPath && analysis.intent && <span>Ý định: {analysis.intent}</span>}
      {!guestPath && analysis.sentiment && (
        <span>
          Cảm xúc:{" "}
          {analysis.sentiment === "angry"
            ? "Tức giận"
            : analysis.sentiment === "negative"
              ? "Tiêu cực"
              : "Trung tính"}
        </span>
      )}
      {error && <span role="alert">{error}</span>}
    </div>
  );
}
