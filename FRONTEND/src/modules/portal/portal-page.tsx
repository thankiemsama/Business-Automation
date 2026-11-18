"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { PortalRepository, type PortalTicket } from "@/shared/api/platform";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";
import Link from "next/link";

const api = new PortalRepository(new FetchHttpClient());

type Phase = "login" | "tickets" | "detail" | "create";

export function PortalPage({ tenant }: { tenant: string }) {
  const storageKey = `nexa-portal:${tenant}`;
  const [token, setToken] = useState<string | null>(null);
  const [phase, setPhase] = useState<Phase>("login");
  const restore = useCallback(
    (node: HTMLElement | null) => {
      if (!node) return;
      const saved = sessionStorage.getItem(storageKey);
      if (saved) {
        setToken(saved);
        setPhase("tickets");
      }
    },
    [storageKey],
  );
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notified, setNotified] = useState(false);
  const [tickets, setTickets] = useState<PortalTicket[]>([]);
  const [active, setActive] = useState<PortalTicket | null>(null);

  useEffect(() => {
    if (!token || phase !== "tickets") return;
    const controller = new AbortController();
    api
      .tickets(tenant, token, controller.signal)
      .then(setTickets)
      .catch((err) => {
        if (controller.signal.aborted) return;
        const message = errorMessage(err);
        if (message.includes("không hợp lệ") || message.includes("hết hạn")) {
          sessionStorage.removeItem(storageKey);
          setToken(null);
          setPhase("login");
        } else {
          setError(message);
        }
      });
    return () => controller.abort();
  }, [token, tenant, phase, storageKey]);

  async function requestCode(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.requestCode(tenant, email);
      setNotified(true);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  async function login(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await api.login(tenant, email, code);
      sessionStorage.setItem(storageKey, result.token);
      setToken(result.token);
      setPhase("tickets");
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  function openTicket(ticket: PortalTicket) {
    setActive(ticket);
    setPhase("detail");
  }

  return (
    <main ref={restore} className="portal-page panel team-form">
      <header className="portal-head">
        <h1>Cổng khách hàng</h1>
        <span>Quản lý phiếu hỗ trợ của bạn</span>
      </header>

      {phase === "login" && (
        <section className="form-stack">
          {notified ? (
            <>
              <p role="status">
                Mã đã được gửi tới <strong>{email}</strong>. Kiểm tra hộp thư và
                nhập mã bên dưới.
              </p>
              <form onSubmit={login} className="form-stack">
                <label>
                  Mã 8 ký tự
                  <input
                    required
                    value={code}
                    onChange={(e) => setCode(e.target.value.toUpperCase())}
                    maxLength={8}
                  />
                </label>
                <button className="button primary" disabled={busy}>
                  Đăng nhập
                </button>
              </form>
            </>
          ) : (
            <form onSubmit={requestCode} className="form-stack">
              <p>
                Nhập email khách hàng của bạn. Chúng tôi sẽ gửi mã đăng nhập một
                lần.
              </p>
              <label>
                Email
                <input
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </label>
              <button className="button primary" disabled={busy}>
                Gửi mã đăng nhập
              </button>
            </form>
          )}
          {error && <p role="alert">{error}</p>}
        </section>
      )}

      {phase === "tickets" && token && (
        <section className="portal-list">
          <div className="toolbar-controls">
            <h2>Phiếu hỗ trợ</h2>
            <button
              className="button secondary"
              onClick={() => setPhase("create")}
            >
              Tạo phiếu mới
            </button>
            <button
              className="button ghost"
              onClick={() => {
                sessionStorage.removeItem(storageKey);
                setToken(null);
                setPhase("login");
              }}
            >
              Đăng xuất
            </button>
          </div>
          <div className="workspace-results">
            {tickets.map((ticket) => (
              <button
                key={ticket.id}
                className="portal-ticket"
                onClick={() => openTicket(ticket)}
              >
                <strong>{ticket.subject}</strong>
                <small>
                  {new Date(ticket.created_at).toLocaleDateString("vi-VN")} ·{" "}
                  {ticket.status}
                </small>
              </button>
            ))}
            {tickets.length === 0 && <p>Chưa có phiếu hỗ trợ nào.</p>}
          </div>
        </section>
      )}

      {phase === "create" && token && (
        <CreateTicket
          tenant={tenant}
          token={token}
          onDone={() => {
            setPhase("tickets");
          }}
          onCancel={() => setPhase("tickets")}
        />
      )}

      {phase === "detail" && token && active && (
        <TicketDetail
          tenant={tenant}
          token={token}
          ticket={active}
          onBack={() => setPhase("tickets")}
        />
      )}

      <footer className="portal-foot">
        <Link href={`/kb/${tenant}`}>Trung tâm trợ giúp</Link> ·{" "}
        <Link href={`/chat/${tenant}`}>Chat trực tiếp</Link>
      </footer>
    </main>
  );
}

function CreateTicket({
  tenant,
  token,
  onDone,
  onCancel,
}: {
  tenant: string;
  token: string;
  onDone: () => void;
  onCancel: () => void;
}) {
  const [file, setFile] = useState<File | null>(null);
  const [subject, setSubject] = useState("");
  const [content, setContent] = useState("");
  const [priority, setPriority] = useState("normal");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault();
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
      await api.createTicket(tenant, token, {
        ...attachment,
        subject,
        content,
        priority,
      });
      onDone();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit} className="form-stack">
      <h2>Tạo phiếu hỗ trợ</h2>
      <label>
        Tiêu đề
        <input
          required
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
        />
      </label>
      <label>
        Mô tả
        <textarea
          required
          value={content}
          onChange={(e) => setContent(e.target.value)}
        />
      </label>
      <label>
        Mức ưu tiên
        <select value={priority} onChange={(e) => setPriority(e.target.value)}>
          <option value="low">Thấp</option>
          <option value="normal">Bình thường</option>
          <option value="high">Cao</option>
          <option value="urgent">Khẩn cấp</option>
        </select>
      </label>
      <label>
        Tệp đính kèm (PNG, JPEG, PDF · tối đa 32 KiB)
        <input
          type="file"
          accept="image/png,image/jpeg,application/pdf"
          onChange={(event) => setFile(event.target.files?.[0] ?? null)}
        />
      </label>
      {error && <p role="alert">{error}</p>}
      <button className="button primary" disabled={busy}>
        Gửi phiếu
      </button>
      <button type="button" className="button ghost" onClick={onCancel}>
        Hủy
      </button>
    </form>
  );
}

function TicketDetail({
  tenant,
  token,
  ticket,
  onBack,
}: {
  tenant: string;
  token: string;
  ticket: PortalTicket;
  onBack: () => void;
}) {
  const [fresh, setFresh] = useState(ticket);
  const [messages, setMessages] = useState<
    {
      id: string;
      content: string;
      sender: string;
      created_at: string;
      attachment_name?: string;
      attachment_data?: string;
    }[]
  >([]);
  const [error, setError] = useState("");
  useEffect(() => {
    api
      .ticket(tenant, token, ticket.id)
      .then((result) => {
        setMessages(result.messages);
        setFresh(result.ticket);
      })
      .catch((err) => setError(errorMessage(err)));
  }, [tenant, token, ticket.id]);
  return (
    <section className="portal-detail">
      <button className="text-link" onClick={onBack}>
        ← Danh sách phiếu
      </button>
      <h2>{ticket.subject}</h2>
      <p>
        Trạng thái: <strong>{fresh.status}</strong> · Ưu tiên:{" "}
        <strong>{ticket.priority}</strong>
      </p>
      {error && <p role="alert">{error}</p>}
      {fresh.rating && (
        <p>
          Đánh giá: {fresh.rating}/5 {fresh.rating_comment}
        </p>
      )}
      <div className="thread-messages">
        {messages.map((message) => (
          <div
            key={message.id}
            className={`message ${message.sender === "agent" ? "inbound" : "outbound"}`}
          >
            <strong>{message.sender === "agent" ? "Hỗ trợ" : "Bạn"}</strong>
            <p>{message.content}</p>
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
        {messages.length === 0 && !error && <p>Đang xử lý…</p>}
      </div>
    </section>
  );
}
