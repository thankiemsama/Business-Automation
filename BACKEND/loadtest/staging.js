import http from "k6/http";
import { check } from "k6";
import { scenario } from "k6/execution";
import { Rate } from "k6/metrics";

function integer(name, fallback, min, max) {
  const value = Number(__ENV[name] ?? fallback);
  if (!Number.isInteger(value) || value < min || value > max) {
    throw new Error(name + " must be an integer between " + min + " and " + max);
  }
  return value;
}

const baseUrl = (__ENV.STAGING_API_URL || "").replace(/\/$/, "");
const tokens = (__ENV.STAGING_API_TOKENS || "").split(",").map((token) => token.trim()).filter(Boolean);
const local = /^http:\/\/(127\.0\.0\.1|localhost)(:\d+)?$/.test(baseUrl);
if (__ENV.CONFIRM_ISOLATED_STAGING !== "yes" ||
    (!local && !/^https:\/\/[a-z0-9.-]+(:\d+)?$/i.test(baseUrl)) || tokens.length === 0) {
  throw new Error("Set CONFIRM_ISOLATED_STAGING=yes, STAGING_API_URL and STAGING_API_TOKENS for an isolated test environment");
}

const rate = integer("STAGING_RPS", 5, 1, 2000);
const writes = integer("STAGING_WRITE_PERCENT", 10, 0, 100);
const vus = integer("STAGING_VUS", Math.max(10, Math.ceil(rate / 2)), 1, 3000);
const p95Target = integer("STAGING_P95_MS", 300, 1, 10000);
const failed = new Rate("staging_unexpected_status");

export const options = {
  discardResponseBodies: true,
  maxRedirects: 0,
  scenarios: {
    staging_api: {
      executor: "constant-arrival-rate",
      rate,
      timeUnit: "1s",
      duration: __ENV.STAGING_DURATION || "30s",
      preAllocatedVUs: vus,
    },
  },
  thresholds: {
    staging_unexpected_status: ["rate<0.001"],
    http_req_duration: ["p(95)<" + p95Target, "p(99)<1200"],
    dropped_iterations: ["count==0"],
  },
};

export default function () {
  const iteration = scenario.iterationInTest;
  const token = tokens[iteration % tokens.length];
  const write = (Math.floor(iteration / tokens.length) * 37) % 100 < writes;
  const params = {
    headers: { Authorization: "Bearer " + token, "Content-Type": "application/json" },
    tags: { operation: write ? "create_ticket" : "list_tickets" },
    timeout: "5s",
  };
  const result = write
    ? http.post(baseUrl + "/api/v1/tickets", JSON.stringify({ subject: "k6 staging ticket " + iteration, priority: "normal" }), params)
    : http.get(baseUrl + "/api/v1/tickets?limit=20", params);
  failed.add(!check(result, { "expected API status": (res) => res.status === (write ? 201 : 200) }));
}
