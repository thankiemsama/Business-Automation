import type { Input, ResourceKind, ResourceMap } from "@/shared/domain/models";
import type { IconName } from "@/shared/ui/icon";
export type FormValues = Record<string, string | boolean>;
export interface Field {
  name: string;
  label: string;
  type?:
    | "text"
    | "email"
    | "textarea"
    | "select"
    | "checkbox"
    | "customer"
    | "conversation";
  required?: boolean;
  options?: [string, string][];
  placeholder?: string;
  immutable?: boolean;
}
export interface ModuleDefinition<K extends ResourceKind> {
  kind: K;
  title: string;
  singular: string;
  description: string;
  icon: IconName;
  ownerOnly?: boolean;
  fields: Field[];
  defaults: FormValues;
  serialize(values: FormValues): Input<ResourceMap[K]>;
}
export const text = (values: FormValues, name: string) =>
  String(values[name] ?? "").trim();
export const ticketStatuses: [string, string][] = [
  ["open", "Đang mở"],
  ["in_progress", "Đang xử lý"],
  ["resolved", "Đã giải quyết"],
];
export const conversationStatuses: [string, string][] = [
  ["open", "Đang mở"],
  ["closed", "Đã đóng"],
];
