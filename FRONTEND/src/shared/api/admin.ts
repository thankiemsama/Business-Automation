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
  /** G8: exchange a password for a 5-minute single-use step-up token. */
  stepUp(password: string) {
    return this.http.request<{ token: string; expires_in: number }>(
      "account/step-up",
      { method: "POST", body: JSON.stringify({ password }) },
    );
  }
  /** Download an admin payload that needs step-up confirmation. */
  async adminDownload(
    password: string,
    path: "admin/export" | "admin/backup",
    query = "",
  ): Promise<void> {
    const { token } = await this.stepUp(password);
    const suffix = query ? query + "&step_up=" : "?step_up=";
    const url = new URL(
      "/api/platform/" + path + suffix + encodeURIComponent(token),
      window.location.origin,
    );
    const response = await fetch(url, {
      credentials: "same-origin",
      cache: "no-store",
    });
    if (!response.ok) {
      throw new Error("Yêu cầu bị từ chối, vui lòng thử lại.");
    }
    const disposition = response.headers.get("Content-Disposition") || "";
    const match = /filename="([^"]+)"/.exec(disposition);
    const blob = await response.blob();
    const objectUrl = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = objectUrl;
    anchor.download = match ? match[1] : "nexa-download";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(objectUrl);
  }
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
