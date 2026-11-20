import type { HttpClient } from "./http-client";

export type Settings = {
  persona_sop?: Partial<Record<"sales" | "cs" | "ops", string>>;
  allowlist?: string[];
  flags?: Record<string, boolean>;
  daily_ai_requests?: number;
  realtime_qa_policy?: string;
  ai_handoff?: { enabled: boolean; minimum_confidence: number };
  brand_card?: {
    enabled: boolean;
    intro: string;
    product_url: string;
    support_url: string;
  };
  display_name: string;
  logo_url: string;
  accent: string;
  host: string;
};
type AuditEvent = {
  id: string;
  created_at: string;
  author_id: string;
  name: string;
  conversation_id: string;
  status: string;
  content: string;
};
type Usage = {
  quota?: Record<
    "tickets" | "ai",
    {
      used: number;
      included: number;
      credits: number;
      overage: number;
      limited: boolean;
      low: boolean;
    }
  >;
  month: string;
  messages: number;
  tickets: number;
  requests: number;
  plan: string;
};

export class AdminRepository {
  constructor(private readonly http: HttpClient) {}

  settings = (signal: AbortSignal) =>
    this.http.request<Settings>("admin/settings", { signal });
  audit = (signal: AbortSignal) =>
    this.http.request<{ items: AuditEvent[]; total: number }>("admin/audit", {
      signal,
    });
  usage = (signal: AbortSignal) =>
    this.http.request<Usage>("admin/usage", { signal });
  billing = (signal: AbortSignal) =>
    this.http.request<Record<string, boolean>>("admin/billing", { signal });
  crm = (signal: AbortSignal) =>
    this.http.request<Record<string, boolean>>("admin/crm", { signal });

  saveSettings(settings: Settings) {
    return this.http.request("admin/settings", {
      method: "PUT",
      body: JSON.stringify(settings),
    });
  }
  billingLink(plan: string) {
    return this.http.request<{ url: string }>("admin/billing", {
      method: "POST",
      body: JSON.stringify({ plan }),
    });
  }
  syncContact(customerId: string, provider: string) {
    return this.http.request("admin/crm", {
      method: "POST",
      body: JSON.stringify({ customer_id: customerId, provider }),
    });
  }
}
