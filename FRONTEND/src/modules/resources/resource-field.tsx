"use client";
import { useId } from "react";
import type { Field } from "./module-definition";
import { RelationSelect } from "./relation-select";

export function ResourceField({
  field,
  value,
  editing,
  onChange,
}: {
  field: Field;
  value: string | boolean;
  editing: boolean;
  onChange: (value: string | boolean) => void;
}) {
  const id = useId();
  if (field.type === "checkbox")
    return (
      <label className="checkbox-field">
        <input
          type="checkbox"
          checked={!!value}
          onChange={(event) => onChange(event.target.checked)}
        />
        <span>{field.label}</span>
      </label>
    );
  let control;
  if (field.type === "customer" || field.type === "conversation") {
    control = (
      <RelationSelect
        id={id}
        kind={field.type === "customer" ? "customers" : "conversations"}
        required={field.required}
        disabled={editing && field.immutable}
        value={String(value || "")}
        onChange={onChange}
      />
    );
  } else if (field.type === "select") {
    control = (
      <select
        id={id}
        value={String(value)}
        onChange={(event) => onChange(event.target.value)}
      >
        {field.options?.map(([value, label]) => (
          <option key={value} value={value}>
            {label}
          </option>
        ))}
      </select>
    );
  } else if (field.type === "textarea") {
    control = (
      <textarea
        id={id}
        rows={5}
        required={field.required}
        value={String(value || "")}
        onChange={(event) => onChange(event.target.value)}
        placeholder={field.placeholder}
        maxLength={16000}
      />
    );
  } else {
    control = (
      <input
        id={id}
        type={field.type || "text"}
        required={field.required}
        value={String(value || "")}
        onChange={(event) => onChange(event.target.value)}
        placeholder={field.placeholder}
        maxLength={1000}
      />
    );
  }
  return (
    <div className="form-field">
      <label htmlFor={id}>
        {field.label}
        {field.required && <span className="required"> *</span>}
      </label>
      {control}
    </div>
  );
}
