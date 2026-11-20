import Link from "next/link";
import { notFound } from "next/navigation";
import {
  KnowledgePublicRepository,
  type PublicArticle,
} from "@/shared/api/platform";
import { ApiError, FetchHttpClient } from "@/shared/api/http-client";

const api = new KnowledgePublicRepository(
  new FetchHttpClient(
    `${process.env.BACKEND_URL || "http://127.0.0.1:8080"}/api/v1`,
  ),
);

export default async function KnowledgeArticlePage({
  params,
}: {
  params: Promise<{ tenant: string; slug: string }>;
}) {
  const { tenant, slug } = await params;
  let article: PublicArticle | null = null;
  try {
    article = await api.article(tenant, slug);
  } catch (error) {
    if (error instanceof ApiError && error.status === 404) notFound();
    throw error;
  }
  if (!article) notFound();
  return (
    <main className="kb-page kb-article">
      <p className="kb-eyebrow">
        <Link href={`/kb/${tenant}`}>Trung tâm trợ giúp</Link> /{" "}
        {article.subject}
      </p>
      <h1>{article.subject}</h1>
      {article.video_url && (
        <p>
          <a href={article.video_url} target="_blank" rel="noreferrer">
            Mở video hướng dẫn
          </a>
        </p>
      )}
      {article.video_transcript && (
        <details className="kb-content">
          <summary>Bản chép lời video</summary>
          {article.video_transcript
            .split("\n")
            .filter(Boolean)
            .map((line, index) => (
              <p key={index}>{line}</p>
            ))}
        </details>
      )}
      <div
        className="kb-content"
        dangerouslySetInnerHTML={{
          __html: safeArticleBody(article.content),
        }}
      />
      <div className="kb-article-actions">
        <p>
          Bài viết này không hữu ích?{" "}
          <Link href={`/portal/${tenant}`}>Tạo phiếu hỗ trợ</Link> hoặc{" "}
          <Link href={`/chat/${tenant}`}>chat với chúng tôi</Link>.
        </p>
      </div>
    </main>
  );
}

// Render plain text as safe paragraphs without introducing raw HTML.
function safeArticleBody(content: string) {
  const escaped = content
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
  return escaped
    .split(/\n{2,}/)
    .map((para) => `<p>${para.trim().replace(/\n/g, "<br/>")}</p>`)
    .join("");
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ tenant: string; slug: string }>;
}) {
  const { tenant, slug } = await params;
  try {
    const article = await api.article(tenant, slug);
    return {
      title: `${article.subject} — Trung tâm trợ giúp Nexa`,
      description:
        article.content.length > 160
          ? article.content.slice(0, 160).trim() + "…"
          : article.content,
      openGraph: {
        type: "article",
        title: article.subject,
        locale: "vi_VN",
        siteName: "Nexa",
      },
      alternates: { canonical: `/kb/${tenant}/${article.slug}` },
    };
  } catch {
    return { title: "Không tìm thấy bài viết | Nexa" };
  }
}
