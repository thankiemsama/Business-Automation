import { readFile } from "node:fs/promises";
import { join } from "node:path";

const bundle = join(
  process.cwd(),
  "node_modules/swagger-ui-dist/swagger-ui-bundle.js",
);
const stylesheet = join(
  process.cwd(),
  "node_modules/swagger-ui-dist/swagger-ui.css",
);

export async function GET(
  _request: Request,
  context: { params: Promise<{ file: string }> },
) {
  const { file } = await context.params;
  if (file === "init.js") {
    return new Response(
      `SwaggerUIBundle({
      url: "/api/platform/public/openapi.json?proxy=1",
      dom_id: "#swagger-ui",
      validatorUrl: null,
      persistAuthorization: false,
    });`,
      { headers: { "Content-Type": "text/javascript; charset=utf-8" } },
    );
  }
  if (file !== "swagger-ui.css" && file !== "swagger-ui-bundle.js") {
    return new Response(null, { status: 404 });
  }
  const isCSS = file === "swagger-ui.css";
  const body = await readFile(isCSS ? stylesheet : bundle);
  return new Response(new Uint8Array(body), {
    headers: {
      "Content-Type": isCSS
        ? "text/css; charset=utf-8"
        : "text/javascript; charset=utf-8",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
