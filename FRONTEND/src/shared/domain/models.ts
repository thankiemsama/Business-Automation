export interface User {
  id: string;
  tenant_id: string;
  name: string;
  email: string;
  role: "owner" | "agent";
}
export interface Entity {
  id: string;
  created_at: string;
  updated_at: string;
}
export interface Customer extends Entity {
  name: string;
  email?: string;
  phone?: string;
}
export interface Conversation extends Entity {
  customer_id: string;
  subject?: string;
  channel: "web" | "email" | "phone";
  status: "open" | "closed";
}
export interface Ticket extends Entity {
  subject: string;
  content?: string;
  customer_id?: string;
  conversation_id?: string;
  status: "open" | "in_progress" | "resolved";
  priority: "low" | "normal" | "high" | "urgent";
}
export interface Knowledge extends Entity {
  subject: string;
  content: string;
  enabled: boolean;
}
export interface Automation extends Entity {
  name: string;
  keywords: string[];
  enabled: boolean;
}
export interface Message extends Entity {
  conversation_id: string;
  content: string;
  sender: "customer" | "agent" | "assistant";
  sequence?: number;
}
export interface ResourceMap {
  customers: Customer;
  conversations: Conversation;
  tickets: Ticket;
  knowledge: Knowledge;
  automations: Automation;
}
export type ResourceKind = keyof ResourceMap;
export type Input<T extends Entity> = Omit<T, keyof Entity>;
export interface Page<T> {
  items: T[];
  total: number;
  page: number;
  limit: number;
}
export interface ListQuery {
  page?: number;
  limit?: number;
  q?: string;
  status?: string;
}
export interface Credentials {
  email: string;
  password: string;
  name?: string;
}
export type Dashboard = Record<
  ResourceKind | "messages" | "open_tickets",
  number
>;
export interface Reply {
  message: Message;
  source_id?: string;
  provider: string;
  needs_human: boolean;
}
