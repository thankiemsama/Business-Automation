import type { HttpClient } from "./http-client";
import type {
  Credentials,
  Dashboard,
  Entity,
  Input,
  ListQuery,
  Message,
  Page,
  Reply,
  ResourceKind,
  ResourceMap,
  User,
} from "../domain/models";

export interface ResourceRepository<T extends Entity> {
  assign(id: string, assignee: string, claim?: boolean): Promise<T>;
  escalate(id: string): Promise<T>;
  recordResponse(id: string): Promise<T>;
  list(query?: ListQuery, signal?: AbortSignal): Promise<Page<T>>;
  get(id: string, signal?: AbortSignal): Promise<T>;
  save(input: Input<T>, id?: string, version?: number): Promise<T>;
}
export class ApiResourceRepository<
  K extends ResourceKind,
> implements ResourceRepository<ResourceMap[K]> {
  constructor(
    private readonly http: HttpClient,
    private readonly kind: K,
  ) {}
  list(query: ListQuery = {}, signal?: AbortSignal) {
    const params = new URLSearchParams();
    Object.entries(query).forEach(([key, value]) => {
      if (Array.isArray(value)) {
        value.forEach((item) => params.append(key, item));
        return;
      }
      if (value !== undefined && value !== "") params.set(key, String(value));
    });
    return this.http.request<Page<ResourceMap[K]>>(`${this.kind}?${params}`, {
      signal,
    });
  }
  recordResponse(id: string) {
    return this.http.request<ResourceMap[K]>(
      `tickets/${encodeURIComponent(id)}/response`,
      { method: "POST" },
    );
  }
  escalate(id: string) {
    return this.http.request<ResourceMap[K]>(
      `tickets/${encodeURIComponent(id)}/escalate`,
      { method: "POST" },
    );
  }
  assign(id: string, assignee_id: string, claim = false) {
    return this.http.request<ResourceMap[K]>(
      `${this.kind}/${encodeURIComponent(id)}/assignment`,
      {
        method: "POST",
        body: JSON.stringify({ assignee_id, claim }),
      },
    );
  }
  get(id: string, signal?: AbortSignal) {
    return this.http.request<ResourceMap[K]>(
      `${this.kind}/${encodeURIComponent(id)}`,
      { signal },
    );
  }
  save(input: Input<ResourceMap[K]>, id?: string, version?: number) {
    return this.http.request<ResourceMap[K]>(
      `${this.kind}${id ? `/${encodeURIComponent(id)}` : ""}`,
      {
        method: id ? "PUT" : "POST",
        body: JSON.stringify({ ...input, version }),
      },
    );
  }
}
export class AuthRepository {
  constructor(private readonly http: HttpClient) {}
  me() {
    return this.http.request<User>("auth/me");
  }
  async login(credentials: Credentials) {
    await this.http.request("auth/login", {
      method: "POST",
      body: JSON.stringify(credentials),
    });
    return this.me();
  }
  async register(credentials: Credentials) {
    await this.http.request<User>("auth/register", {
      method: "POST",
      body: JSON.stringify(credentials),
    });
    return this.login({
      email: credentials.email,
      password: credentials.password,
    });
  }
  logout() {
    return this.http.request("auth/logout", { method: "POST" });
  }
  account(
    action: "auth/password" | "team/invite" | "auth/forgot" | "auth/redeem",
    input: Record<string, string>,
  ) {
    return this.http.request(action, {
      method: "POST",
      body: JSON.stringify(input),
    });
  }
  members(signal?: AbortSignal) {
    return this.http.request<User[]>("team", { signal });
  }
  updateMember(id: string, input: Pick<User, "name" | "email" | "disabled">) {
    return this.http.request<User>(`team/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
  }
  addAgent(credentials: Credentials) {
    return this.http.request<User>("team", {
      method: "POST",
      body: JSON.stringify(credentials),
    });
  }
}
export type RealtimeQA = {
  id: string;
  status: "pending" | "completed" | "quota" | "unavailable" | "stale";
  labels: { kind: string; message: string }[];
  suggestion?: string;
};

export class ConversationRepository extends ApiResourceRepository<"conversations"> {
  realtimeQAStatus(signal: AbortSignal) {
    return this.client.request<{ enabled: boolean }>("ai/realtime-qa/status", {
      signal,
    });
  }
  queueRealtimeQA(id: string, draft: string, signal: AbortSignal) {
    return this.client.request<RealtimeQA>(
      `conversations/${encodeURIComponent(id)}/realtime-qa`,
      {
        method: "POST",
        body: JSON.stringify({ draft }),
        signal,
      },
    );
  }
  realtimeQAResult(id: string, request: string, signal: AbortSignal) {
    return this.client.request<RealtimeQA>(
      `conversations/${encodeURIComponent(id)}/realtime-qa/${encodeURIComponent(request)}`,
      { signal },
    );
  }
  constructor(private readonly client: HttpClient) {
    super(client, "conversations");
  }
  templates(signal?: AbortSignal) {
    return this.client.request<
      Page<{
        id: string;
        version: number;
        subject: string;
        content: string;
        enabled: boolean;
      }>
    >("templates?limit=100", { signal });
  }
  saveTemplate(
    subject: string,
    content: string,
    enabled: boolean,
    id?: string,
    version?: number,
  ) {
    return this.client.request(
      `templates${id ? "/" + encodeURIComponent(id) : ""}`,
      {
        method: id ? "PUT" : "POST",
        body: JSON.stringify({ subject, content, enabled, version }),
      },
    );
  }
  messages(id: string, query: ListQuery, signal?: AbortSignal) {
    return this.client.request<Page<Message>>(
      `conversations/${encodeURIComponent(id)}/messages?page=${query.page || 1}&limit=${query.limit || 50}`,
      { signal },
    );
  }
  read(id: string, sequence: number) {
    return this.client.request(`conversations/${encodeURIComponent(id)}/read`, {
      method: "POST",
      body: JSON.stringify({ sequence }),
    });
  }
  send(
    id: string,
    content: string,
    sender: "agent" | "customer" | "note",
    attachment?: { attachment_name: string; attachment_data: string },
    sourceMessageId?: string,
  ) {
    return this.client.request<Message>(
      `conversations/${encodeURIComponent(id)}/messages`,
      {
        method: "POST",
        body: JSON.stringify({
          content,
          sender,
          ...attachment,
          source_message_id: sourceMessageId,
        }),
      },
    );
  }
  deliver(id: string, message: string) {
    return this.client.request<Message>(
      `conversations/${encodeURIComponent(id)}/messages/${encodeURIComponent(message)}/deliver`,
      { method: "POST" },
    );
  }
  draft(id: string) {
    return this.client.request<Reply>(
      `conversations/${encodeURIComponent(id)}/draft`,
      { method: "POST" },
    );
  }
  reply(id: string) {
    return this.client.request<Reply>(
      `conversations/${encodeURIComponent(id)}/reply`,
      { method: "POST" },
    );
  }
}
export class DashboardRepository {
  constructor(private readonly http: HttpClient) {}
  report(from: string, to: string, signal?: AbortSignal) {
    return this.http.request<import("../domain/models").OperationsReport>(
      `reports?${new URLSearchParams({ from, to })}`,
      { signal },
    );
  }
  get(signal?: AbortSignal) {
    return this.http.request<Dashboard>("dashboard", { signal });
  }
}
