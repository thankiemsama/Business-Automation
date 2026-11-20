"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import { VoiceInput } from "../ai/assist";
import { useQuery } from "@/shared/hooks/use-query";
import { Icon } from "@/shared/ui/icon";
import type { BrandCard } from "@/shared/domain/models";
import { ConversationBrandCard } from "./brand-card";
import styles from "./chat-widget.module.css";

const api = new FetchHttpClient();
type ChatMessage = {
  brand_card?: BrandCard;
  id: string;
  content: string;
  sender: string;
  attachment_name?: string;
  attachment_data?: string;
};
type ChatState = {
  messages: ChatMessage[];
  can_rate: boolean;
  rated: boolean;
};

export function ChatWidget({ tenant }: { tenant: string }) {
  const [token, setToken] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [fileVersion, setFileVersion] = useState(0);
  const [content, setContent] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const path = `chat/${encodeURIComponent(tenant)}`;
  const storageKey = `nexa-chat:${tenant}`;
  const [savedToken, setSavedToken] = useState<string | null>(null);
  const read = useCallback(
    (signal: AbortSignal) => {
      const secret = token || savedToken;
      return secret
        ? api.request<ChatState>(`${path}?include_state=true`, {
            signal,
            headers: { Authorization: `Bearer ${secret}` },
          })
        : Promise.resolve(null);
    },
    [path, token, savedToken],
  );
  const active = token || savedToken;
  const refreshEvent = `nexa-chat-refresh:${tenant}`;
  const chat = useQuery(read, refreshEvent);
  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    let retry: ReturnType<typeof setTimeout> | undefined;
    async function subscribe() {
      try {
        const response = await fetch(`/api/platform/${path}/events`, {
          signal: controller.signal,
          headers: { Authorization: `Bearer ${active}` },
        });
        if (response.status === 401 || response.status === 404) return;
        if (!response.ok || !response.body)
          throw new Error("Không kết nối được chat.");
        const reader = response.body
          .pipeThrough(new TextDecoderStream())
          .getReader();
        let buffer = "";
        while (!controller.signal.aborted) {
          const chunk = await reader.read();
          if (chunk.done) break;
          buffer += chunk.value;
          let separator = buffer.indexOf("\n\n");
          while (separator >= 0) {
            if (buffer.slice(0, separator).includes("event: refresh"))
              window.dispatchEvent(new Event(refreshEvent));
            buffer = buffer.slice(separator + 2);
            separator = buffer.indexOf("\n\n");
          }
        }
      } catch {
        // The same credentials are used after temporary connection interruptions.
      }
      if (!controller.signal.aborted)
        retry = setTimeout(() => void subscribe(), 2000);
    }
    void subscribe();
    return () => {
      controller.abort();
      if (retry) clearTimeout(retry);
    };
  }, [active, path, refreshEvent]);
  const restore = useCallback(
    (node: HTMLElement | null) => {
      if (node) setSavedToken(localStorage.getItem(storageKey) || "");
    },
    [storageKey],
  );
  async function send(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const fields = new FormData(event.currentTarget);
    setBusy(true);
    setError("");
    try {
      if (file && file.size > 32 * 1024) throw new Error("Tệp tối đa 32 KiB.");
      const attachment = file
        ? {
            attachment_name: file.name,
            attachment_data: btoa(
              String.fromCharCode(...new Uint8Array(await file.arrayBuffer())),
            ),
          }
        : undefined;
      if (!active) {
        const reply = await api.request<{ token: string }>(path, {
          method: "POST",
          body: JSON.stringify({
            name: fields.get("name"),
            content,
            ...attachment,
          }),
        });
        localStorage.setItem(storageKey, reply.token);
        setToken(reply.token);
      } else {
        await api.request(path, {
          method: "POST",
          headers: { Authorization: `Bearer ${active}` },
          body: JSON.stringify({ content, ...attachment }),
        });
        chat.reload();
      }
      setContent("");
      setFile(null);
      setFileVersion((version) => version + 1);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  async function rate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!active || busy || chat.loading || !chat.data?.can_rate) return;
    const rating = Number(new FormData(event.currentTarget).get("rating"));
    setError("");
    setBusy(true);
    try {
      await api.request(path, {
        method: "POST",
        headers: { Authorization: `Bearer ${active}` },
        body: JSON.stringify({ rating }),
      });
      chat.reload();
    } catch (error) {
      setError(errorMessage(error));
      chat.reload();
    } finally {
      setBusy(false);
    }
  }
  function newChat() {
    localStorage.removeItem(storageKey);
    setToken("");
    setSavedToken("");
    setError("");
  }
  return (
    <main className={styles.page} ref={restore}>
      <div className={`panel public-chat ${styles.card}`}>
        <header className={styles.header}>
          <span className={styles.mark}>
            <Icon name="chat" size={24} />
          </span>
          <h1>Hỗ trợ khách hàng</h1>
        </header>
        <section
          className={`thread-messages ${styles.messages}`}
          aria-label="Tin nhắn hỗ trợ"
          aria-live="polite"
        >
          {!chat.data?.messages.length && !chat.error && (
            <p className={styles.prompt}>
              {active && chat.loading
                ? "Đang tải tin nhắn…"
                : "Gửi tin nhắn để bắt đầu trao đổi với đội ngũ hỗ trợ."}
            </p>
          )}
          {chat.data?.messages.map((message) => (
            <div
              className={`message ${message.sender === "customer" ? "outbound" : "inbound"}`}
              key={message.id}
            >
              <strong>
                {message.sender === "customer"
                  ? "Bạn"
                  : message.sender === "assistant"
                    ? "Trợ lý AI"
                    : "Hỗ trợ"}
              </strong>
              <p>{message.content}</p>
              {message.brand_card && (
                <ConversationBrandCard card={message.brand_card} />
              )}
              {message.attachment_data && (
                <a
                  download={message.attachment_name}
                  href={`data:application/octet-stream;base64,${message.attachment_data}`}
                >
                  {message.attachment_name}
                </a>
              )}
            </div>
          ))}
        </section>
        {(error || chat.error) && (
          <p className={styles.error} role="alert">
            {error || chat.error}
          </p>
        )}
        <form className={`form-stack ${styles.composer}`} onSubmit={send}>
          {active && (
            <VoiceInput
              guest={{ path, token: active }}
              disabled={busy}
              onText={(text) =>
                setContent((current) =>
                  current ? `${current}\n${text}` : text,
                )
              }
            />
          )}
          {!active && (
            <label>
              Tên của bạn
              <input name="name" required maxLength={200} />
            </label>
          )}
          <label>
            Tin nhắn
            <textarea
              required={!file}
              maxLength={16000}
              value={content}
              onChange={(event) => setContent(event.target.value)}
            />
          </label>
          <label>
            Tệp đính kèm (tối đa 32 KiB)
            <input
              key={fileVersion}
              type="file"
              accept="image/png,image/jpeg,application/pdf"
              onChange={(event) => setFile(event.target.files?.[0] ?? null)}
            />
          </label>
          <button
            className={`button primary ${styles.send}`}
            disabled={busy || savedToken === null || (!content.trim() && !file)}
          >
            <Icon name="send" size={16} />
            Gửi tin nhắn
          </button>
        </form>
        {active && (
          <div className={`form-stack ${styles.actions}`}>
            {chat.data?.rated ? (
              <p role="status">Cảm ơn bạn đã đánh giá.</p>
            ) : chat.data?.can_rate ? (
              <form onSubmit={rate}>
                <label>
                  Mức hài lòng sau khi hỗ trợ kết thúc{" "}
                  <select name="rating" defaultValue="5">
                    {[1, 2, 3, 4, 5].map((rating) => (
                      <option key={rating}>{rating}</option>
                    ))}
                  </select>
                </label>
                <button
                  className="button secondary"
                  disabled={busy || chat.loading}
                >
                  Gửi đánh giá
                </button>
              </form>
            ) : null}
            <button
              className="button secondary"
              disabled={busy}
              onClick={newChat}
            >
              Bắt đầu hội thoại mới
            </button>
          </div>
        )}
      </div>
    </main>
  );
}
