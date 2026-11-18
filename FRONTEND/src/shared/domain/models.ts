export interface User {
  id: string;
  tenant_id: string;
  name: string;
  email: string;
  disabled: boolean;
  role: "owner" | "agent";
}
export interface Entity {
  id: string;
  created_at: string;
  updated_at: string;
}
export interface Customer extends Entity {
  tags?: string[];
  name: string;
  email?: string;
  phone?: string;
}
export interface Conversation extends Entity {
  ai_summary?: string;
  tags?: string[];
  assignee_id?: string;
  unread?: boolean;
  customer_id: string;
  subject?: string;
  channel: "web" | "email" | "phone";
  status: "open" | "closed";
}
export interface Ticket extends Entity {
  ai_summary?: string;
  sla_risk_reason?: string;
  tags?: string[];
  assignee_id?: string;
  overdue?: boolean;
  due_soon?: boolean;
  first_response_at?: string;
  response_due_at?: string;
  resolution_due_at?: string;
  escalated_at?: string;
  subject: string;
  content?: string;
  customer_id?: string;
  conversation_id?: string;
  status: "open" | "in_progress" | "resolved";
  priority: "low" | "normal" | "high" | "urgent";
}
export interface Knowledge extends Entity {
  public?: boolean;
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
  sentiment?: "neutral" | "negative" | "angry";
  status?: "pending" | "sent";
  attachment_name?: string;
  attachment_data?: string;
  conversation_id: string;
  content: string;
  sender: "customer" | "agent" | "assistant" | "note";
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
  assignment?: string;
  priority?: string;
  tag?: string[];
  assignee?: string;
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
export interface ReportRow {
  id: string;
  name: string;
  opened: number;
  backlog: number;
  responses: number;
  resolved: number;
  ratings: number;
  average_response_minutes: number;
  average_resolution_minutes: number;
  average_rating: number;
}
export interface OperationsReport {
  total: ReportRow;
  agents: ReportRow[];
}
export interface Reply {
  message: Message;
  source_id?: string;
  provider: string;
  needs_human: boolean;
}
