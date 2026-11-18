import { NextRequest } from "next/server";

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ provider: string; step: string }> },
) {
  const { provider, step } = await params;
  if (
    !["google", "okta", "microsoft"].includes(provider) ||
    !["start", "callback"].includes(step)
  )
    return new Response(null, { status: 404 });
  const backend = process.env.BACKEND_URL || "http://127.0.0.1:8080";
  const response = await fetch(
    `${backend}/api/v1/oidc/${provider}/${step}${request.nextUrl.search}`,
    {
      headers: {
        Cookie: request.cookies.get("nexa_oidc")
          ? `nexa_oidc=${request.cookies.get("nexa_oidc")?.value}`
          : "",
      },
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.timeout(12000),
    },
  );
  const headers = new Headers({
    "Cache-Control": "no-store",
    "Referrer-Policy": "no-referrer",
  });
  for (const key of ["location", "set-cookie", "content-type"]) {
    const value = response.headers.get(key);
    if (value) headers.set(key, value);
  }
  return new Response(response.body, { status: response.status, headers });
}
