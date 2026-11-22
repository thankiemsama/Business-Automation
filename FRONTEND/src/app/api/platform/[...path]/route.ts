import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "nexa_session";
const id = "[a-zA-Z0-9_-]+";
const token = "[a-zA-Z0-9_.-]+";
const resource =
  "(?:customers|conversations|tickets|knowledge|automations|templates)";

const allowedPath = new RegExp(
  `^(?:auth/(?:login|register|me|logout|password|forgot|redeem|oidc-exchange)|` +
    `branding|oidc/providers|account/(?:sessions(?:/revoke)?|mfa)|admin/(?:settings|brand|audit|usage|export|backup|fields|billing|crm|custom/(?:customers|tickets|conversations)/${id})|` +
    `chat/${token}(?:/(?:events|voice-status|transcribe))?|` +
    `team(?:/${id})?|` +
    `ai/(?:status|search|transcribe|voice-inbox/(?:status|transcribe)|summary/(?:conversations|tickets)/${id}|(?:suggestion|review)/${id})|` +
    `ai/ticket-agent/(?:status|traces|${id}/(?:plan|apply))|` +
    `ai/sparring(?:/preview/${id}|/scenarios(?:/${id}/archive)?|/sessions(?:/${id}/(?:turn|end))?)?|` +
    `dashboard|reports(?:/export|/analytics)?|operations/(?:workflows|schedules)(?:/remove|/migrate)?|events|push|search|` +
    `operations/field-extraction|` +
    `operations/skill-routing|` +
    `notifications(?:/read)?|views(?:/remove)?|` +
    `lead-campaigns(?:/${id}/(?:qr|leads))?|lead/${token}/${id}|` +
    `kb/${token}(?:/${token})?|` +
    `sat/${token}|` +
    `portal/${token}/(?:code|login|tickets)(?:/${id})?|` +
    `${resource}(?:/${id})?|` +
    `templates/${id}/export|` +
    `customers/${id}/(?:profile|notes|merge)|` +
    `conversations/${id}/(?:messages|reply|draft|assignment|read)|` +
    `conversations/${id}/messages/${id}/deliver|` +
    `ai/realtime-qa/status|conversations/${id}/realtime-qa(?:/${id})?|` +
    `tickets/${id}/(?:assignment|escalate|response)|` +
    `channels(?:/${id}(?:/test|/deliveries(?:/${id}/retry)?)?)?|` +
    `webhooks(?:/${id}(?:/remove)?)?|webhooks/dead-letters/${id}/replay|` +
    `api-keys(?:/${id}/revoke)?|` +
    `public/(?:openapi\\.json|v1/(?:customers|tickets|conversations)(?:/${id})?))$`,
);

// Endpoints that can be reached without the session cookie (guests, portal
// tokens, or developer API keys). Everything else requires the HttpOnly cookie.
function isPublicEndpoint(path: string) {
  return (
    path === "auth/login" ||
    [
      "auth/register",
      "auth/forgot",
      "auth/redeem",
      "auth/oidc-exchange",
      "branding",
      "oidc/providers",
    ].includes(path) ||
    path.startsWith("chat/") ||
    path.startsWith("lead/") ||
    path.startsWith("kb/") ||
    path.startsWith("sat/") ||
    path.startsWith("portal/") ||
    path.startsWith("public/")
  );
}

// These families carry their own credential in the Authorization header (a chat
// token, a portal session token, or a developer API key) rather than the cookie.
function usesHeaderAuth(path: string) {
  return (
    path.startsWith("chat/") ||
    path.startsWith("portal/") ||
    path.startsWith("public/")
  );
}

// The browser BFF forwards most paths to /api/v1/*, but the developer API-key
// surface lives under a distinct backend base path, /api/public/*.
function backendPath(path: string) {
  if (path.startsWith("public/")) return "/api/public/" + path.slice(7);
  return `/api/v1/${path}`;
}

async function forward(
  request: NextRequest,
  context: { params: Promise<{ path: string[] }> },
) {
  const path = (await context.params).path.join("/");
  const fail = (message: string, status: number) =>
    NextResponse.json({ data: null, message, code: "ERROR" }, { status });
  if (!allowedPath.test(path)) return fail("Endpoint không tồn tại", 404);
  // Next normalizes loopback names in nextUrl; the browser's Host retains the actual origin.
  let expectedOrigin =
    process.env.APP_ORIGIN ||
    `${request.nextUrl.protocol}//${request.headers.get("host")}`;
  if (
    request.method !== "GET" &&
    request.headers.get("origin") !== expectedOrigin
  ) {
    const host = request.headers.get("host") ?? "";
    const candidate = `https://${host}`;
    if (
      !/^[a-z0-9.-]+$/i.test(host) ||
      request.headers.get("origin") !== candidate
    )
      return fail("Nguồn yêu cầu không hợp lệ", 403);
    try {
      const backend = process.env.BACKEND_URL || "http://127.0.0.1:8080";
      const lookup = await fetch(
        `${backend}/api/v1/branding?${new URLSearchParams({ host })}`,
        { cache: "no-store", signal: AbortSignal.timeout(2000) },
      );
      const result = await lookup.json();
      if (!lookup.ok || !result.data || !Object.hasOwn(result.data, "name"))
        return fail("Tên miền chưa cấu hình", 403);
      expectedOrigin = candidate;
    } catch {
      return fail("Không kiểm tra được tên miền", 502);
    }
  }
  const publicEndpoint = isPublicEndpoint(path);
  const token = request.cookies.get(SESSION_COOKIE)?.value;
  if (!publicEndpoint && !token)
    return fail("Vui lòng đăng nhập để tiếp tục", 401);
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  const proxySecret = process.env.NEXA_PROXY_SECRET;
  // A custom ingress may sign X-Nexa-Client-IP itself; behind Render or any
  // trusted edge the real client IP arrives in the first X-Forwarded-For hop.
  // Behind Render the real client IP is the right-most X-Forwarded-For hop
  // (Render appends it last); the left-most hop can be client-supplied/spoofed.
  const xff = request.headers.get("x-forwarded-for");
  const forwarded = xff ? xff.split(",").pop()?.trim() : undefined;
  const ingressIP = request.headers.get("x-nexa-ingress-ip") || forwarded;
  if (
    process.env.NEXA_TRUST_INGRESS_IP === "true" &&
    proxySecret &&
    ingressIP
  ) {
    headers["X-Nexa-Proxy-Secret"] = proxySecret;
    headers["X-Nexa-Client-IP"] = ingressIP;
  }
  const stream =
    request.method === "GET" && (path === "events" || path.endsWith("/events"));
  const eventID = request.headers.get("last-event-id");
  if (stream && eventID) headers["Last-Event-ID"] = eventID;
  if (token && !publicEndpoint) headers.Authorization = `Bearer ${token}`;
  if (usesHeaderAuth(path))
    headers.Authorization = request.headers.get("authorization") ?? "";
  try {
    const audio = path.endsWith("/transcribe") && request.method === "POST";
    const body = audio
      ? await readAudio(request)
      : request.method === "GET"
        ? undefined
        : await request.text();
    if (audio && !body) return fail("Tệp vượt 4 MiB hoặc trống", 413);
    if (audio) headers["Content-Type"] = "application/octet-stream";
    if (
      typeof body === "string" &&
      new TextEncoder().encode(body).length > 65536
    )
      return fail("Dữ liệu vượt quá 64 KiB", 413);
    const upstream = await fetch(
      `${process.env.BACKEND_URL || "http://127.0.0.1:8080"}${backendPath(path)}${request.nextUrl.search}`,
      {
        method: request.method,
        headers,
        body: body || undefined,
        cache: "no-store",
        signal: stream
          ? request.signal
          : AbortSignal.timeout(
              audio
                ? 45000
                : path.endsWith("/draft") || path.startsWith("ai/")
                  ? 30000
                  : 15000,
            ),
        redirect: "error",
      },
    );
    if (
      upstream.ok &&
      (stream ||
        ["reports/export", "admin/export", "admin/backup"].includes(path))
    ) {
      const headers = new Headers({ "Cache-Control": "no-store" });
      for (const key of [
        "Content-Type",
        "Content-Disposition",
        "X-Accel-Buffering",
      ]) {
        const value = upstream.headers.get(key);
        if (value) headers.set(key, value);
      }
      return new Response(upstream.body, { status: upstream.status, headers });
    }
    const envelope = await upstream.json();
    const login =
      (path === "auth/login" || path === "auth/oidc-exchange") && upstream.ok;
    const response = NextResponse.json(
      login ? { ...envelope, data: { authenticated: true } } : envelope,
      { status: upstream.status, headers: { "Cache-Control": "no-store" } },
    );
    const retry = upstream.headers.get("retry-after");
    if (retry) response.headers.set("Retry-After", retry);
    if (login)
      response.cookies.set(SESSION_COOKIE, envelope.data.access_token, {
        httpOnly: true,
        sameSite: "lax",
        secure: new URL(expectedOrigin).protocol === "https:",
        path: "/",
        maxAge: envelope.data.expires_in,
      });
    if (path === "auth/logout" || (upstream.status === 401 && !publicEndpoint))
      response.cookies.delete(SESSION_COOKIE);
    return response;
  } catch {
    return fail(
      "Không kết nối được máy chủ. Hãy kiểm tra backend đang chạy và thử lại.",
      502,
    );
  }
}
async function readAudio(request: Request) {
  const reader = request.body?.getReader();
  if (!reader) return null;
  const parts: Uint8Array<ArrayBuffer>[] = [];
  let size = 0;
  while (true) {
    const next = await reader.read();
    if (next.done) break;
    size += next.value.byteLength;
    if (size > 4 * 1024 * 1024) {
      await reader.cancel();
      return null;
    }
    parts.push(new Uint8Array(next.value));
  }
  return size ? new Blob(parts) : null;
}
export { forward as GET, forward as POST, forward as PUT };
