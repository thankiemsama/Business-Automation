"use client";
import { useCallback, useRef, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage } from "@/shared/api/http-client";

export function ReplyTemplates({
  onSelect,
}: {
  onSelect: (content: string) => void;
}) {
  const { conversations } = useServices();
  const { role } = useUser();
  const load = useCallback(
    (signal: AbortSignal) => conversations.templates(signal),
    [conversations],
  );
  const templates = useQuery(load);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const subjectInput = useRef<HTMLInputElement>(null);
  const contentInput = useRef<HTMLTextAreaElement>(null);
  const [choice, setChoice] = useState("");
  const selected = templates.data?.items.find(
    (template) => template.id === choice,
  );
  return (
    <details>
      <summary>Câu trả lời mẫu</summary>
      <select
        aria-label="Chọn câu trả lời mẫu"
        value={choice}
        onChange={(event) => {
          const id = event.target.value;
          setChoice(id);
          const template = templates.data?.items.find((item) => item.id === id);
          if (template?.enabled) onSelect(template.content);
        }}
      >
        <option value="">Chọn mẫu</option>
        {templates.data?.items
          .filter((item) => item.enabled || role === "owner")
          .map((item) => (
            <option key={item.id} value={item.id}>
              {item.subject}
              {item.enabled ? "" : " (đã tắt)"}
            </option>
          ))}
      </select>
      {role === "owner" && (
        <div
          key={selected ? `${selected.id}:${selected.version}` : "new"}
          className="form-stack"
        >
          <label>
            Tên mẫu
            <input
              ref={subjectInput}
              defaultValue={selected?.subject}
              maxLength={200}
            />
          </label>
          <label>
            Nội dung mẫu
            <textarea
              ref={contentInput}
              defaultValue={selected?.content}
              maxLength={16000}
            />
          </label>
          <button
            type="button"
            className="button secondary compact"
            disabled={busy}
            onClick={async () => {
              const subject = subjectInput.current?.value.trim() ?? "";
              const content = contentInput.current?.value.trim() ?? "";
              if (!subject || !content) {
                setError("Nhập tên và nội dung mẫu.");
                return;
              }
              setBusy(true);
              setError("");
              try {
                await conversations.saveTemplate(
                  subject,
                  content,
                  true,
                  selected?.id,
                  selected?.version,
                );
                templates.reload();
              } catch (error) {
                setError(errorMessage(error));
              } finally {
                setBusy(false);
              }
            }}
          >
            {selected ? "Lưu mẫu" : "Thêm mẫu"}
          </button>
          {selected?.enabled && (
            <button
              type="button"
              className="button secondary compact"
              disabled={busy}
              onClick={async () => {
                setBusy(true);
                setError("");
                try {
                  await conversations.saveTemplate(
                    selected.subject,
                    selected.content,
                    false,
                    selected.id,
                    selected.version,
                  );
                  templates.reload();
                } catch (error) {
                  setError(errorMessage(error));
                } finally {
                  setBusy(false);
                }
              }}
            >
              Tắt mẫu
            </button>
          )}
        </div>
      )}
      {(error || templates.error) && (
        <p role="alert">{error || templates.error}</p>
      )}
    </details>
  );
}
