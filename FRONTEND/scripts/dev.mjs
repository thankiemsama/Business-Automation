// Turbopack's Node loader worker can leave its generated chunk locked on Windows.
const bundler = process.platform === "win32" ? ["--webpack"] : [];
process.argv.splice(2, 0, "dev", ...bundler);
await import("next/dist/bin/next");
