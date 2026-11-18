"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { CSATRepository } from "@/shared/api/platform";
import { FetchHttpClient, errorMessage } from "@/shared/api/http-client";

const api = new CSATRepository(new FetchHttpClient());

const labels: [number, string][] = [
  [1, "Rất kém"],
  [2, "Kém"],
  [3, "Bình thường"],
  [4, "Tốt"],
  [5, "Xuất sắc"],
];

export function CSATPage({ token }: { token: string }) {
  const [status, setStatus] = useState<"loading" | "form" | "done" | "error">(
    "loading",
  );
  const [subject, setSubject] = useState("");
  const [rating, setRating] = useState(5);
  const [comment, setComment] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    api
      .resolve(token)
      .then((result) => {
        if (cancelled) return;
        setSubject(result.subject || "vấn đề của bạn");
        setStatus(result.rated ? "done" : "form");
      })
      .catch((err) => {
        if (cancelled) return;
        setError(errorMessage(err));
        setStatus("error");
      });
    return () => {
      cancelled = true;
    };
  }, [token]);

  const submit = useCallback(
    async (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      setBusy(true);
      setError("");
      try {
        await api.submit(token, rating, comment);
        setStatus("done");
      } catch (err) {
        setError(errorMessage(err));
      } finally {
        setBusy(false);
      }
    },
    [token, rating, comment],
  );

  if (status === "loading")
    return (
      <main className="sat-page">
        <p>Đang tải…</p>
      </main>
    );

  if (status === "error")
    return (
      <main className="sat-page">
        <h1>Không thể tải khảo sát</h1>
        <p role="alert">{error}</p>
        <p>Liên kết khảo sát có thể đã hết hạn.</p>
      </main>
    );

  if (status === "done")
    return (
      <main className="sat-page">
        <h1>Cảm ơn bạn!</h1>
        <p>
          Đánh giá của bạn đã được ghi nhận. Cảm ơn đã góp phần giúp chúng tôi
          hỗ trợ tốt hơn.
        </p>
      </main>
    );

  return (
    <main className="sat-page">
      <h1>Bạn hài lòng với cách hỗ trợ “{subject}” như thế nào?</h1>
      <p className="sat-lead">
        Vui lòng đánh giá trải nghiệm của bạn (1–5 sao).
      </p>
      <div className="sat-stars" role="radiogroup" aria-label="Đánh giá sao">
        {labels.map(([value, label]) => (
          <label key={value} className={rating >= value ? "is-active" : ""}>
            <input
              type="radio"
              name="rating"
              value={value}
              checked={rating === value}
              onChange={() => setRating(value)}
            />
            <span>★</span>
            <small>{label}</small>
          </label>
        ))}
      </div>
      <form className="sat-form" onSubmit={submit}>
        <label>
          Nhận xét thêm (không bắt buộc)
          <textarea
            value={comment}
            maxLength={1000}
            placeholder="Chia sẻ thêm điều gì bạn muốn chúng tôi biết…"
            onChange={(e) => setComment(e.target.value)}
          />
        </label>
        <button className="button primary" disabled={busy}>
          Gửi đánh giá
        </button>
      </form>
      {error && <p role="alert">{error}</p>}
    </main>
  );
}
