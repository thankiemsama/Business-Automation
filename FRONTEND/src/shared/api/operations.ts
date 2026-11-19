import type { ReportRow } from "../domain/models";
import { FetchHttpClient } from "./http-client";

type MetricAverage = { name: string; samples: number; value: number | null };

export type AnalyticsReport = {
  ai: {
    agent_proposed: number;
    agent_applied: number;
    agent_auto_resolved: number;
    reviews_queued: number;
    reviews_scored: number;
    reviews_pending: number;
    reviews_failed: number;
    triage_offered: number;
    triage_accepted: number;
    triage_auto_applied: number;
    bot_cases: number;
    auto_resolved: number;
    avoided_tickets: number;
    tickets: number;
    metered_requests: number;
    unpriced_requests: number;
    cost_usd: number | null;
    cost_per_ticket_usd: number | null;
  };
  csat_by_channel: MetricAverage[];
  response_minutes_by_priority: MetricAverage[];
  daily: {
    day: string;
    tickets: number;
    ratings: number;
    csat: number | null;
  }[];
  channels: { name: string; count: number }[];
  priorities: { name: string; count: number }[];
  sla: {
    week: string;
    agent_id: string;
    agent: string;
    passed: number;
    total: number;
  }[];
  cohorts: { week: string; size: number; active: (number | null)[] }[];
  leaderboard: ReportRow[];
};

export type Conditions = {
  channel: string;
  priority: string;
  status: string;
  contains: string;
  keywords_any?: string[];
};
export type Action = { type: string; value?: string };
export type Workflow = {
  id?: string;
  name: string;
  enabled: boolean;
  trigger: string;
  idle_days?: number;
  conditions: Conditions;
  actions: Action[];
  branch?: { conditions: Conditions; then: Action[]; otherwise: Action[] };
};
export type Schedule = {
  id?: string;
  name: string;
  enabled: boolean;
  frequency: "weekly" | "monthly";
  recipient_id: string;
  hour: number;
  day: number;
  next_at?: string;
};

const api = new FetchHttpClient();

export function loadAnalytics(from: string, to: string, signal: AbortSignal) {
  return api.request<AnalyticsReport>(
    `reports/analytics?${new URLSearchParams({ from, to })}`,
    { signal },
  );
}
export function loadWorkflows(signal: AbortSignal) {
  return api.request<Workflow[]>("operations/workflows", { signal });
}
export function loadSchedules(signal: AbortSignal) {
  return api.request<{ items: Schedule[]; email_enabled: boolean }>(
    "operations/schedules",
    { signal },
  );
}
export function saveOperation(
  kind: "workflows" | "schedules",
  value: Workflow | Schedule,
) {
  return api.request(`operations/${kind}`, {
    method: "POST",
    body: JSON.stringify(value),
  });
}
export function migrateWorkflow(id: string) {
  return api.request<Workflow>("operations/workflows/migrate", {
    method: "POST",
    body: JSON.stringify({ id }),
  });
}
export function removeOperation(kind: "workflows" | "schedules", id: string) {
  return api.request(`operations/${kind}/remove`, {
    method: "POST",
    body: JSON.stringify({ id }),
  });
}
