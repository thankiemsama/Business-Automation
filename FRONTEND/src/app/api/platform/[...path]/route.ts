import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "nexa_session";
const resource = "(?:customers|conversations|tickets|knowledge|automations)";
const allowedPath = new RegExp(
  `^(?:auth/(?:login|register|me|logout)|team|dashboard|${resource}(?:/[a-zA-Z0-9_-]+)?|conversations/[a-zA-Z0-9_-]+/(?:messages|reply))$`,
);

async function forward(
  request: NextRequest,
  context: { params: Promise<{ path: string[] }> },
) {
  const path = (await context.params).path.join("/");
  const fail = (message: string, status: number) =>
    NextResponse.json({ data: null, message, code: "ERROR" }, { status });
  if (!allowedPath.test(path)) return fail("Endpoint không tồn tại", 404);
  // Next normalizes loopback names in nextUrl; the browser's Host retains the actual origin.
  const expectedOrigin =
    process.env.APP_ORIGIN ||
    `${request.nextUrl.protocol}//${request.headers.get("host")}`;
  if (
    request.method !== "GET" &&
    request.headers.get("origin") !== expectedOrigin
  )
    return fail("Nguồn yêu cầu không hợp lệ", 403);
  const publicEndpoint = path === "auth/login" || path === "auth/register";
  const token = request.cookies.get(SESSION_COOKIE)?.value;
  if (!publicEndpoint && !token)
    return fail("Vui lòng đăng nhập để tiếp tục", 401);
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token && !publicEndpoint) headers.Authorization = `Bearer ${token}`;
  try {
    const body = request.method === "GET" ? undefined : await request.text();
    if (body && new TextEncoder().encode(body).length > 65536)
      return fail("Dữ liệu vượt quá 64 KiB", 413);
    const upstream = await fetch(
      `${process.env.BACKEND_URL || "http://127.0.0.1:8080"}/api/v1/${path}${request.nextUrl.search}`,
      {
        method: request.method,
        headers,
        body: body || undefined,
        cache: "no-store",
        signal: AbortSignal.timeout(15000),
        redirect: "error",
      },
    );
    const envelope = await upstream.json();
    const login = path === "auth/login" && upstream.ok;
    const response = NextResponse.json(
      login ? { ...envelope, data: { authenticated: true } } : envelope,
      { status: upstream.status, headers: { "Cache-Control": "no-store" } },
    );
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
export { forward as GET, forward as POST, forward as PUT };
