import { KnowledgePublicRepository } from "@/shared/api/platform";
import { FetchHttpClient } from "@/shared/api/http-client";
import type { PublicArticle } from "@/shared/api/platform";
import { KBSearchBox } from "./search-box";

const api = new KnowledgePublicRepository(
  new FetchHttpClient(
    `${process.env.BACKEND_URL || "http://127.0.0.1:8080"}/api/v1`,
  ),
);

export default async function KnowledgeIndexPage({
  params,
}: {
  params: Promise<{ tenant: string }>;
}) {
  const { tenant } = await params;
  let articles: PublicArticle[] = [];
  let error = "";
  try {
    articles = await api.list(tenant);
  } catch (err) {
    error = err instanceof Error ? err.message : "Không tải được nội dung.";
  }
  return (
    <main className="kb-page">
      <section className="kb-hero">
        <p className="kb-eyebrow">Trung tâm trợ giúp</p>
        <h1>Tìm câu trả lời trước khi mở ticket</h1>
        <p>
          Duyệt các hướng dẫn chính thức của chúng tôi. Bạn không cần đăng nhập
          để xem.
        </p>
        <KBSearchBox tenant={tenant} articles={articles} />
      </section>
      {error && <p role="alert">{error}</p>}
      <div className="kb-grid">
        {articles.map((article) => (
          <article key={article.slug} className="kb-card">
            <h2>
              <a href={`/kb/${tenant}/${article.slug}`}>{article.subject}</a>
            </h2>
            <p>{article.content.slice(0, 140).trim()}…</p>
            <a className="text-link" href={`/kb/${tenant}/${article.slug}`}>
              Xem hướng dẫn
            </a>
          </article>
        ))}
      </div>
      {articles.length === 0 && !error && (
        <p>Chưa có bài viết công khai nào.</p>
      )}
    </main>
  );
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ tenant: string }>;
}) {
  const { tenant } = await params;
  try {
    await api.list(tenant);
    return {
      title: "Trung tâm trợ giúp — Nexa",
      description: "Tra cứu các hướng dẫn chính thức trước khi liên hệ.",
      openGraph: {
        type: "website",
        section: "kb",
        locale: "vi_VN",
        siteName: "Nexa",
        url: `/kb/${tenant}`,
      },
      keywords: ["trợ giúp", "hỗ trợ", "cskh", "nexa"],
      robots: { index: true, follow: true },
      alternates: { canonical: `/kb/${tenant}` },
    };
  } catch {
    return { title: "Trung tâm trợ giúp | Nexa" };
  }
}
