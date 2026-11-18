import type { HttpClient } from "./http-client";
export type WorkKind = "tickets" | "conversations";
export type Filters = { priority: string; tags: string[]; assignee: string };
export type SavedView = Filters & {
  id?: string;
  kind: WorkKind;
  name: string;
  status: string;
  assignment: string;
  search: string;
};
export type SearchResult = {
  id: string;
  kind: WorkKind | "customers";
  title: string;
};
export type Notice = {
  id: string;
  subject: string;
  content?: string;
  name: WorkKind;
  conversation_id: string;
  created_at: string;
  read_at?: string;
};

export function parseTags(value: string) {
  return value
    .split(",")
    .map((tag) => tag.trim())
    .filter(Boolean);
}

export class WorkspaceRepository {
  constructor(private readonly http: HttpClient) {}
  notifications(signal?: AbortSignal) {
    return this.http.request<{ items: Notice[]; unread: number }>(
      "notifications",
      { signal },
    );
  }
  read(id = "") {
    return this.post("notifications/read", { id });
  }
  views(signal?: AbortSignal) {
    return this.http.request<SavedView[]>("views", { signal });
  }
  saveView(view: SavedView) {
    return this.post<SavedView>("views", view);
  }
  removeView(id: string) {
    return this.post("views/remove", { id });
  }
  search(q: string, signal?: AbortSignal) {
    return this.http.request<SearchResult[]>(
      `search?${new URLSearchParams({ q })}`,
      { signal },
    );
  }
  bulk(kind: WorkKind, ids: string[], action: string, assignee_id: string) {
    return this.post<{ updated: number }>(`${kind}/bulk`, {
      ids,
      action,
      assignee_id,
    });
  }
  pushKey() {
    return this.http.request<{ key: string }>("push");
  }
  push(subscription: PushSubscriptionJSON, disable: boolean) {
    return this.post("push", {
      endpoint: subscription.endpoint,
      keys: subscription.keys,
      disable,
    });
  }
  private post<T>(path: string, body: unknown) {
    return this.http.request<T>(path, {
      method: "POST",
      body: JSON.stringify(body),
    });
  }
}
