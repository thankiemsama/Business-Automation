"use client";
import { useState, type FormEvent } from "react";
import type { ResourceKind, ResourceMap } from "@/shared/domain/models";
import type { ResourceRepository } from "@/shared/api/repositories";
import { errorMessage } from "@/shared/api/http-client";
import { ErrorState, Modal } from "@/shared/ui/primitives";
import { resourceModules } from "./registry";
import type { FormValues } from "./module-definition";
import { ResourceField } from "./resource-field";
export function ResourceEditor<K extends ResourceKind>({
  kind,
  repository,
  entity,
  onClose,
  onSaved,
}: {
  kind: K;
  repository: ResourceRepository<ResourceMap[K]>;
  entity?: ResourceMap[K];
  onClose: () => void;
  onSaved: (entity: ResourceMap[K]) => void;
}) {
  const definition = resourceModules[kind];
  const [values, setValues] = useState<FormValues>(() => {
    const initial = { ...definition.defaults };
    if (entity)
      for (const [key, value] of Object.entries(entity)) {
        if (key in initial)
          initial[key] = Array.isArray(value)
            ? value.join(", ")
            : typeof value === "boolean"
              ? value
              : String(value ?? "");
      }
    return initial;
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const change = (name: string, value: string | boolean) =>
    setValues((previous) => ({ ...previous, [name]: value }));
  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      onSaved(await repository.save(definition.serialize(values), entity?.id));
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title={`${entity ? "Cập nhật" : "Tạo"} ${definition.singular}`}
      description="Thông tin được lưu trong không gian doanh nghiệp của bạn."
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <form className="form-stack" onSubmit={submit}>
        <fieldset disabled={busy} className="form-fields">
          {definition.fields.map((field) => (
            <ResourceField
              key={field.name}
              field={field}
              value={values[field.name] ?? ""}
              editing={!!entity}
              onChange={(value) => change(field.name, value)}
            />
          ))}
        </fieldset>
        {error && <ErrorState message={error} />}
        <div className="modal-actions">
          <button
            type="button"
            className="button secondary"
            onClick={onClose}
            disabled={busy}
          >
            Hủy
          </button>
          <button className="button primary" disabled={busy}>
            {busy ? "Đang lưu…" : "Lưu thông tin"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
