export function GET() {
  return new Response(
    `<!doctype html>
<html lang="vi"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Nexa Public API</title><link rel="stylesheet" href="/api-docs/assets/swagger-ui.css"></head>
<body><div id="swagger-ui"></div><script src="/api-docs/assets/swagger-ui-bundle.js"></script>
<script src="/api-docs/assets/init.js"></script></body></html>`,
    {
      headers: {
        "Content-Type": "text/html; charset=utf-8",
        "Content-Security-Policy":
          "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'",
        "Referrer-Policy": "no-referrer",
      },
    },
  );
}
