import type { NextConfig } from "next";
import { PHASE_DEVELOPMENT_SERVER } from "next/constants";

const nextConfig: NextConfig = {
  devIndicators: false,
  outputFileTracingIncludes: {
    "/api/reports/pdf": [
      "./node_modules/@expo-google-fonts/noto-sans/{400Regular,700Bold}/*.ttf",
    ],
  },
  webpack(config, { dev }) {
    if (dev && process.platform === "win32") {
      // Run Tailwind before Next.js's built-in CSS pipeline.
      config.module.rules.push({
        rules: [
          {
            test: /[\\/]src[\\/]app[\\/]globals\.css$/,
            enforce: "pre",
            loader: "@tailwindcss/turbopack",
          },
        ],
      });
    }

    return config;
  },
  turbopack: {
    rules: {
      "src/app/globals.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default function config(phase: string): NextConfig {
  if (phase === PHASE_DEVELOPMENT_SERVER && process.platform === "win32") {
    return {
      ...nextConfig,
      // Webpack can fail to unlink the MCP file logger's output during startup.
      experimental: { ...nextConfig.experimental, mcpServer: false },
    };
  }

  return nextConfig;
}
