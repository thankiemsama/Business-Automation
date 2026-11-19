"use client";
import { useState, type FormEvent } from "react";
import type { ResourceKind, ResourceMap } from "@/shared/domain/models";
import type { ResourceRepository } from "@/shared/api/repositories";
import { ApiError, errorMessage } from "@/shared/api/http-client";
import { ErrorState, Modal } from "@/shared/ui/primitives";
import { resourceModules } from "./registry";
import type { FormValues } from "./module-definition";
import { ResourceField } from "./resource-field";

function formValues(defaults: FormValues, entity?: object): FormValues {
  const values = { ...defaults };
  if (entity)
    for (const [key, value] of Object.entries(entity)) {
      if (key in values)
        values[key] = Array.isArray(value)
          ? value.join(", ")
          : typeof value === "boolean"
            ? value
            : String(value ?? "");
    }
  return values;
}

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
  const [original, setOriginal] = useState(
    () => entity && { id: entity.id, version: entity.version },
  );
  const [values, setValues] = useState(() =>
    formValues(definition.defaults, entity),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [canReload, setCanReload] = useState(false);
  const change = (name: string, value: string | boolean) =>
    setValues((previous) => ({ ...previous, [name]: value }));
  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      onSaved(
        await repository.save(
          definition.serialize(values),
          original?.id,
          original?.version,
        ),
      );
    } catch (error) {
      setError(errorMessage(error));
      setCanReload(
        error instanceof ApiError && [409, 428].includes(error.status),
      );
    } finally {
      setBusy(false);
    }
  }
  async function reloadLatest() {
    if (!original) return;
    setBusy(true);
    try {
      const latest = await repository.get(original.id);
      setOriginal({ id: latest.id, version: latest.version });
      setValues(formValues(definition.defaults, latest));
      setError("");
      setCanReload(false);
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
        {canReload && original && (
          <button
            type="button"
            className="text-link"
            disabled={busy}
            onClick={() => void reloadLatest()}
          >
            Tải bản mới và bỏ chỉnh sửa đang nhập
          </button>
        )}
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
