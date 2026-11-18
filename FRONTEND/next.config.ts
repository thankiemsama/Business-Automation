import type { NextConfig } from "next";

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
            test: /\.css$/i,
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
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
};

export default nextConfig;
