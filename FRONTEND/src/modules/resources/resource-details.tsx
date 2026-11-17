"use client";
import type { ResourceKind, ResourceMap } from "@/shared/domain/models";
import { Modal } from "@/shared/ui/primitives";
import { resourceModules } from "./registry";

export function ResourceDetails<K extends ResourceKind>({
  kind,
  entity,
  onClose,
}: {
  kind: K;
  entity: ResourceMap[K];
  onClose: () => void;
}) {
  const definition = resourceModules[kind];
  const values = new Map(Object.entries(entity));
  return (
    <Modal title={`Chi tiết ${definition.singular}`} onClose={onClose}>
      <dl className="resource-details">
        {definition.fields.map((field) => {
          const value = values.get(field.name);
          return (
            <div key={field.name}>
              <dt>{field.label}</dt>
              <dd>
                {typeof value === "boolean"
                  ? value
                    ? "Đang bật"
                    : "Đã tắt"
                  : Array.isArray(value)
                    ? value.join(", ")
                    : String(value || "Chưa cung cấp")}
              </dd>
            </div>
          );
        })}
      </dl>
      <div className="detail-actions">
        <button className="button secondary" onClick={onClose}>
          Đóng
        </button>
      </div>
    </Modal>
  );
}
