"use client";
import { useCallback, useRef, useState } from "react";
import { useServices } from "@/shared/providers/services-provider";
import { useUser } from "@/shared/providers/workspace-provider";
import { useQuery } from "@/shared/hooks/use-query";
import { errorMessage, FetchHttpClient } from "@/shared/api/http-client";
import { Icon } from "@/shared/ui/icon";
import styles from "./conversation-composer.module.css";

const api = new FetchHttpClient();

function TemplateTransfer({
  selectedID,
  onImported,
}: {
  selectedID?: string;
  onImported: () => void;
}) {
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [feedback, setFeedback] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function transfer(mode: "export" | "import") {
    setBusy(true);
    setFeedback("");
    setError("");
    try {
      if (mode === "export" && selectedID) {
        const result = await api.request<{ code: string }>(
          `templates/${encodeURIComponent(selectedID)}/export`,
          { method: "POST", body: JSON.stringify({ password }) },
        );
        setCode(result.code);
        setFeedback("Mã chia sẻ đã sẵn sàng.");
      } else if (mode === "import") {
        await api.request("templates/import", {
          method: "POST",
          body: JSON.stringify({ code, password }),
        });
        setCode("");
        setFeedback("Đã nhập mẫu trả lời.");
        onImported();
      }
      setPassword("");
    } catch (failure) {
      setError(errorMessage(failure));
    } finally {
      setBusy(false);
    }
  }

  return (
    <details className="form-stack">
      <summary>Chia sẻ và nhập mẫu giữa doanh nghiệp</summary>
      <label>
        Mật khẩu chia sẻ
        <input
          type="password"
          autoComplete="new-password"
          maxLength={256}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
      </label>
      <label>
        Mã chia sẻ
        <textarea
          value={code}
          maxLength={49152}
          onChange={(event) => setCode(event.target.value)}
        />
      </label>
      <div className="toolbar-controls">
        <button
          type="button"
          className="button secondary compact"
          disabled={busy || !selectedID || Array.from(password).length < 12}
          onClick={() => void transfer("export")}
        >
          Xuất mẫu đã chọn
        </button>
        <button
          type="button"
          className="button secondary compact"
          disabled={busy || !code.trim() || Array.from(password).length < 12}
          onClick={() => void transfer("import")}
        >
          Nhập mẫu
        </button>
      </div>
      {feedback && <p role="status">{feedback}</p>}
      {error && <p role="alert">{error}</p>}
    </details>
  );
}

export function ReplyTemplates({
  disabled,
  onSelect,
}: {
  disabled: boolean;
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
    <details className={styles.templates}>
      <summary className={styles.tool}>
        <Icon name="chat" size={16} />
        Câu trả lời mẫu
        <Icon name="chevron" size={13} />
      </summary>
      <fieldset className={styles.templateBody} disabled={disabled}>
        <select
          aria-label="Chọn câu trả lời mẫu"
          value={choice}
          onChange={(event) => {
            const id = event.target.value;
            setChoice(id);
            const template = templates.data?.items.find(
              (item) => item.id === id,
            );
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
        {role === "owner" && (
          <TemplateTransfer
            key={selected?.id ?? "none"}
            selectedID={selected?.id}
            onImported={templates.reload}
          />
        )}
      </fieldset>
    </details>
  );
}
