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
  list(query?: ListQuery, signal?: AbortSignal): Promise<Page<T>>;
  get(id: string, signal?: AbortSignal): Promise<T>;
  save(input: Input<T>, id?: string): Promise<T>;
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
      if (value !== undefined && value !== "") params.set(key, String(value));
    });
    return this.http.request<Page<ResourceMap[K]>>(`${this.kind}?${params}`, {
      signal,
    });
  }
  get(id: string, signal?: AbortSignal) {
    return this.http.request<ResourceMap[K]>(
      `${this.kind}/${encodeURIComponent(id)}`,
      { signal },
    );
  }
  save(input: Input<ResourceMap[K]>, id?: string) {
    return this.http.request<ResourceMap[K]>(
      `${this.kind}${id ? `/${encodeURIComponent(id)}` : ""}`,
      { method: id ? "PUT" : "POST", body: JSON.stringify(input) },
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
  addAgent(credentials: Credentials) {
    return this.http.request<User>("team", {
      method: "POST",
      body: JSON.stringify(credentials),
    });
  }
}
export class ConversationRepository extends ApiResourceRepository<"conversations"> {
  constructor(private readonly client: HttpClient) {
    super(client, "conversations");
  }
  messages(id: string, query: ListQuery, signal?: AbortSignal) {
    return this.client.request<Page<Message>>(
      `conversations/${encodeURIComponent(id)}/messages?page=${query.page || 1}&limit=${query.limit || 50}`,
      { signal },
    );
  }
  send(id: string, content: string, sender: "agent" | "customer") {
    return this.client.request<Message>(
      `conversations/${encodeURIComponent(id)}/messages`,
      { method: "POST", body: JSON.stringify({ content, sender }) },
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
  get(signal?: AbortSignal) {
    return this.http.request<Dashboard>("dashboard", { signal });
  }
}
