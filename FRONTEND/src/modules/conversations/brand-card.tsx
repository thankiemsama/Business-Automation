import type { BrandCard } from "@/shared/domain/models";

export function ConversationBrandCard({ card }: { card: BrandCard }) {
  return (
    <aside className="panel form-stack" aria-label="Danh thiếp thương hiệu">
      <strong>{card.name}</strong>
      {card.intro && <p>{card.intro}</p>}
      {(card.product_url || card.support_url) && (
        <div className="toolbar-controls">
          {card.product_url?.startsWith("https://") && (
            <a
              href={card.product_url}
              target="_blank"
              rel="noopener noreferrer"
            >
              Sản phẩm
            </a>
          )}
          {card.support_url?.startsWith("https://") && (
            <a
              href={card.support_url}
              target="_blank"
              rel="noopener noreferrer"
            >
              Hỗ trợ
            </a>
          )}
        </div>
      )}
    </aside>
  );
}
