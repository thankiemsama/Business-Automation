"use client";

import { useState } from "react";
import type { Conversation } from "@/shared/domain/models";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import { useQuery } from "@/shared/hooks/use-query";
import { RelationSelect } from "../resources/relation-select";

const api = new FetchHttpClient();
const maxBytes = 4 * 1024 * 1024;

function loadStatus(signal: AbortSignal) {
  return api.request<{ enabled: boolean }>("ai/voice-inbox/status", { signal });
}

export function VoiceImport({
  onImported,
}: {
  onImported: (conversation: Conversation) => void;
}) {
  const status = useQuery(loadStatus);
  const [customerID, setCustomerID] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [authorized, setAuthorized] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  if (!status.data?.enabled) return null;

  return (
    <details className="panel team-form">
      <summary>Nhập bản ghi âm cuộc gọi</summary>
      <form
        className="form-stack"
        onSubmit={async (event) => {
          event.preventDefault();
          if (!file || !customerID || !authorized || busy) return;
          setError("");
          setNotice("");
          if (file.size > maxBytes) {
            setError("Tệp tối đa 4 MiB.");
            return;
          }
          setBusy(true);
          const form = event.currentTarget;
          try {
            const params = new URLSearchParams({
              customer_id: customerID,
              authorized: "true",
            });
            const conversation = await api.request<Conversation>(
              `ai/voice-inbox/transcribe?${params}`,
              {
                method: "POST",
                headers: { "Content-Type": "application/octet-stream" },
                body: file,
              },
            );
            onImported(conversation);
            setNotice("Đã mở hội thoại có bản chép lời.");
            setFile(null);
            setAuthorized(false);
            form.reset();
          } catch (failure) {
            setError(errorMessage(failure));
          } finally {
            setBusy(false);
          }
        }}
      >
        <label htmlFor="voice-customer">Khách hàng</label>
        <RelationSelect
          id="voice-customer"
          kind="customers"
          value={customerID}
          onChange={setCustomerID}
          required
          disabled={busy}
        />
        <label>
          Tệp ghi âm WEBM, OGG, M4A, WAV hoặc MP3, tối đa 4 MiB
          <input
            type="file"
            accept=".webm,.ogg,.m4a,.wav,.mp3"
            required
            disabled={busy}
            onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          />
        </label>
        <label className="check-row">
          <input
            type="checkbox"
            checked={authorized}
            disabled={busy}
            onChange={(event) => setAuthorized(event.target.checked)}
          />
          Tôi xác nhận doanh nghiệp có quyền xử lý bản ghi này và gửi tệp tới
          OpenAI để chép lời.
        </label>
        <button
          className="button primary compact"
          disabled={!file || !customerID || !authorized || busy}
        >
          {busy ? "Đang chép lời…" : "Tạo hội thoại từ bản ghi âm"}
        </button>
        {notice && <p role="status">{notice}</p>}
        {error && <p role="alert">{error}</p>}
      </form>
    </details>
  );
}
