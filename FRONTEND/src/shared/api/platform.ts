import type { HttpClient } from "./http-client";

// Platform differentiation API surface: Customer 360°, public knowledge base,
// customer portal, CSAT surveys, multi-channel config, webhooks, and API keys.

export interface CustomerProfile {
  customer: {
    id: string;
    name: string;
    email?: string;
    phone?: string;
    tags?: string[];
    created_at: string;
  };
  total_tickets: number;
  open_tickets: number;
  average_rating: number;
  conversations: number;
  channels: string[];
  tags: string[];
  notes: { id: string; content: string; author: string; created_at: string }[];
  ltv: {
    tickets: number;
    resolved: number;
    avg_rating: number;
    last_active: string;
  };
  timeline: {
    kind: string;
    channel?: string;
    subject: string;
    content?: string;
    sender?: string;
    created_at: string;
  }[];
}

export interface APIKey {
  id: string;
  name: string;
  prefix: string;
  secret?: string;
  scopes: string[];
  rate_per_min: number;
  enabled: boolean;
}

export interface Webhook {
  id: string;
  name: string;
  url: string;
  events: string[];
  secret?: string;
}

export interface ChannelStatus {
  [channel: string]: {
    channel: string;
    name?: string;
    content?: string;
    enabled: boolean;
    status?: string;
  };
}

export interface ChannelDelivery {
  id: string;
  status: "pending" | "sending" | "sent" | "failed";
  email_last_error?: string;
  email_attempts?: number;
  updated_at: string;
}

export interface PublicArticle {
  slug: string;
  subject: string;
  content: string;
}

export interface PortalTicket {
  rating?: number;
  rating_comment?: string;
  id: string;
  subject: string;
  content?: string;
  status: string;
  priority: string;
  created_at: string;
}

export const webhookEvents = [
  "ticket.created",
  "message.new",
  "sla.breached",
  "csat.submitted",
] as const;

export const publicChannels = [
  "zalo",
  "messenger",
  "whatsapp",
  "sms",
  "instagram",
] as const;

export class PlatformRepository {
  constructor(private readonly http: HttpClient) {}

  // Customer 360°
  profile(id: string, signal?: AbortSignal) {
    return this.http.request<CustomerProfile>(
      `customers/${encodeURIComponent(id)}/profile`,
      { signal },
    );
  }
  addNote(id: string, content: string) {
    return this.post(`customers/${encodeURIComponent(id)}/notes`, { content });
  }
  mergeCustomers(sourceId: string, targetId: string) {
    return this.post(`customers/${encodeURIComponent(sourceId)}/merge`, {
      target_id: targetId,
    });
  }

  // Multi-channel config
  channels(signal?: AbortSignal) {
    return this.http.request<Record<string, ChannelStatus[number]>>(
      "channels",
      {
        signal,
      },
    );
  }
  saveChannel(
    channel: string,
    input: { name: string; content: string; enabled: boolean },
  ) {
    return this.post(`channels/${encodeURIComponent(channel)}`, input);
  }

  channelDeliveries(channel: string, signal?: AbortSignal) {
    return this.http.request<ChannelDelivery[]>(
      `channels/${encodeURIComponent(channel)}/deliveries`,
      { signal },
    );
  }
  testChannel(channel: string, to: string, text: string) {
    return this.post<ChannelDelivery>(
      `channels/${encodeURIComponent(channel)}/test`,
      { to, text },
    );
  }
  retryChannel(channel: string, id: string) {
    return this.post(
      `channels/${encodeURIComponent(channel)}/deliveries/${encodeURIComponent(id)}/retry`,
      {},
    );
  }

  // Webhooks
  webhooks(signal?: AbortSignal) {
    return this.http.request<Webhook[]>("webhooks", { signal });
  }
  createWebhook(input: { name: string; url: string; events: string[] }) {
    return this.post<Webhook>("webhooks", input);
  }
  updateWebhook(
    id: string,
    input: { name: string; url: string; events: string[] },
  ) {
    return this.post<Webhook>(`webhooks/${encodeURIComponent(id)}`, input);
  }
  removeWebhook(id: string) {
    return this.post(`webhooks/${encodeURIComponent(id)}/remove`, {});
  }

  // API keys
  apiKeys(signal?: AbortSignal) {
    return this.http.request<APIKey[]>("api-keys", { signal });
  }
  createApiKey(input: { name: string; scope: string; rate_per_min?: number }) {
    return this.post<APIKey>("api-keys", input);
  }
  revokeApiKey(id: string) {
    return this.post(`api-keys/${encodeURIComponent(id)}/revoke`, {});
  }

  private post<T = unknown>(path: string, body: unknown) {
    return this.http.request<T>(path, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }
}

// ---- Guest-facing repositories (no session cookie; use header tokens) ----

export class KnowledgePublicRepository {
  constructor(private readonly http: HttpClient) {}
  list(tenant: string, signal?: AbortSignal) {
    return this.http.request<PublicArticle[]>(
      `kb/${encodeURIComponent(tenant)}`,
      { signal },
    );
  }
  article(tenant: string, slug: string, signal?: AbortSignal) {
    return this.http.request<PublicArticle>(
      `kb/${encodeURIComponent(tenant)}/${encodeURIComponent(slug)}`,
      { signal },
    );
  }
}

export class PortalRepository {
  constructor(private readonly http: HttpClient) {}
  requestCode(tenant: string, email: string) {
    return this.http.request(`portal/${encodeURIComponent(tenant)}/code`, {
      method: "POST",
      body: JSON.stringify({ email }),
    });
  }
  login(tenant: string, email: string, code: string) {
    return this.http.request<{
      token: string;
      customer: string;
      email: string;
    }>(`portal/${encodeURIComponent(tenant)}/login`, {
      method: "POST",
      body: JSON.stringify({ email, code }),
    });
  }
  tickets(tenant: string, token: string, signal?: AbortSignal) {
    return this.http.request<PortalTicket[]>(
      `portal/${encodeURIComponent(tenant)}/tickets`,
      { signal, headers: { Authorization: `Bearer ${token}` } },
    );
  }
  ticket(tenant: string, token: string, id: string) {
    return this.http.request<{
      ticket: PortalTicket & { conversation_id?: string };
      messages: {
        id: string;
        content: string;
        sender: string;
        created_at: string;
        attachment_name?: string;
        attachment_data?: string;
      }[];
    }>(
      `portal/${encodeURIComponent(tenant)}/tickets/${encodeURIComponent(id)}`,
      { headers: { Authorization: `Bearer ${token}` } },
    );
  }
  createTicket(
    tenant: string,
    token: string,
    input: {
      subject: string;
      content: string;
      priority?: string;
      attachment_name?: string;
      attachment_data?: string;
    },
  ) {
    return this.http.request<PortalTicket>(
      `portal/${encodeURIComponent(tenant)}/tickets`,
      {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
        body: JSON.stringify(input),
      },
    );
  }
}

export class CSATRepository {
  constructor(private readonly http: HttpClient) {}
  private split(raw: string) {
    const i = raw.indexOf(".");
    if (i < 0) return [raw, ""];
    return [raw.slice(0, i), raw.slice(i + 1)];
  }
  resolve(raw: string, signal?: AbortSignal) {
    const [tenant, secret] = this.split(raw);
    return this.http.request<{
      conversation: string;
      subject: string;
      rated: boolean;
    }>(`sat/${encodeURIComponent(tenant)}.${encodeURIComponent(secret)}`, {
      signal,
    });
  }
  submit(raw: string, rating: number, comment: string) {
    const [tenant, secret] = this.split(raw);
    return this.http.request(
      `sat/${encodeURIComponent(tenant)}.${encodeURIComponent(secret)}`,
      {
        method: "POST",
        body: JSON.stringify({ rating, comment }),
      },
    );
  }
}

export class PublicApiRepository {
  constructor(
    private readonly http: HttpClient,
    private readonly key: string,
  ) {}
  tickets(signal?: AbortSignal) {
    return this.http.request("public/v1/tickets", {
      signal,
      headers: { Authorization: `Bearer nexa_${this.key}` },
    });
  }
}
