"use client";
import { useMemo, useState } from "react";
import type { PublicArticle } from "@/shared/api/platform";

// Client-side fuzzy filter for the public knowledge base.
export function KBSearchBox({
  tenant,
  articles,
}: {
  tenant: string;
  articles: PublicArticle[];
}) {
  const [q, setQ] = useState("");
  const filtered = useMemo(() => {
    const query = q.trim().toLowerCase();
    if (!query) return articles;
    return articles.filter((a) =>
      (a.subject + " " + a.content).toLowerCase().includes(query),
    );
  }, [q, articles]);
  return (
    <div className="kb-search">
      <input
        aria-label="Tìm kiếm trong trung tâm trợ giúp"
        placeholder="Tìm kiếm: đổi trả, thanh toán, tài khoản…"
        value={q}
        onChange={(e) => setQ(e.target.value)}
      />
      {q && (
        <ul className="kb-suggest">
          {filtered.slice(0, 6).map((a) => (
            <li key={a.slug}>
              <a href={`/kb/${tenant}/${a.slug}`}>{a.subject}</a>
            </li>
          ))}
          {filtered.length === 0 && <li>Không tìm thấy bài viết nào.</li>}
        </ul>
      )}
    </div>
  );
}
