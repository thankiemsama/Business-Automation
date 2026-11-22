// Windows notes:
// - Turbopack's Node loader worker can leave its generated chunk locked, so dev
//   runs with webpack on Windows.
// - On Windows the dev cache (.next/dev) is often locked by Windows Search
//   Indexer (SearchIndexer/SearchFilterHost hold a shared read handle) and/or
//   Windows Defender real-time scanning. After a force-killed dev run the next
//   startup then fails with "EPERM: unlink <...>/.next/dev/.../page.js".
// - sweepStaleDevCache() recovers from that: it first tries to delete the stale
//   dev cache; if the files are still locked it renames the whole tree aside
//   (a rename is a metadata op that succeeds even when file handles are held)
//   so Next can start a fresh .next/dev, then deletes the moved tree in the
//   background once the handles are released.
// - Only the dev cache is touched. .next/server, .next/static and .next/cache
//   (production build + persistent cache) are deliberately left alone, so no
//   other feature is affected.
// - If cleanup cannot run at all, a printed hint points at the one-time system
//   exclusion script in scripts/windows-system-exclusion.ps1.
import { readdir, rename, rm } from "node:fs/promises";
import net from "node:net";
import path from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const isWindows = process.platform === "win32";
const bundler = isWindows ? ["--webpack"] : [];

const scriptsDir = path.dirname(fileURLToPath(import.meta.url));
const nextDir = path.join(scriptsDir, "..", ".next");
const devDir = path.join(nextDir, "dev");

function devPort() {
  const i = process.argv.indexOf("--port");
  const value = i >= 0 ? Number(process.argv[i + 1]) : NaN;
  return Number.isInteger(value) && value > 0 ? value : 3000;
}

function portInUse(port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once("error", () => resolve(true));
    server.once("listening", () => {
      server.close(() => resolve(false));
    });
    server.listen(port, "0.0.0.0");
  });
}

// Best-effort retry delete of a directory that may still be locked by the
// Search Indexer / Defender. Never throws; runs in the background of the
// long-lived dev process and gives up quietly once the handle is released.
async function retryDelete(dir, backoffsMs) {
  for (const ms of backoffsMs) {
    await delay(ms);
    try {
      await rm(dir, { recursive: true, force: true });
      return;
    } catch {
      /* still locked; keep retrying */
    }
  }
}

// The dev cache is owned by the running dev server; stale leftovers from a
// killed run are safe to move aside because Next recreates it. We rename (not
// unlink) the whole tree because on Windows the indexer may still hold file
// handles so per-file unlink fails with EPERM, while renaming the parent
// directory is a metadata operation that succeeds. The moved tree is cleaned
// up in the background.
async function sweepStaleDevCache() {
  // Another dev server owns the cache; don't clobber a running server.
  if (await portInUse(devPort())) return;

  // Indexer handles can be held for a while, so use long backoffs for the
  // background cleanup.
  const cleanBackoffs = [5000, 10000, 20000, 30000, 60000, 120000];

  // Clean up previously moved-aside caches from earlier runs first.
  let entries = [];
  try {
    entries = await readdir(nextDir, { withFileTypes: true });
  } catch {
    /* .next may not exist yet */
  }
  for (const entry of entries) {
    if (entry.isDirectory() && /^dev\.stale\b/.test(entry.name)) {
      void retryDelete(path.join(nextDir, entry.name), cleanBackoffs);
    }
  }

  // 1) Try to delete the current stale .next/dev outright.
  for (let attempt = 1; attempt <= 4; attempt++) {
    try {
      await rm(devDir, { recursive: true, force: true });
      return; // clean case: deleted
    } catch (error) {
      if ((error.code === "EPERM" || error.code === "EBUSY") && attempt < 4) {
        await delay(500 * attempt);
        continue;
      }
      break;
    }
  }

  // 2) rm failed (indexer/defender lock): rename the tree aside, retrying a
  // few times because the lock can clear within seconds.
  const renameBackoffs = [500, 1000, 2000, 4000, 8000];
  let moved = false;
  for (const ms of renameBackoffs) {
    const staleDir = path.join(nextDir, `dev.stale-${Date.now()}`);
    try {
      await rename(devDir, staleDir);
      console.log(
        `[dev] moved locked .next\\dev -> .next\\${path.basename(staleDir)}; cleaning up in background`,
      );
      void retryDelete(staleDir, cleanBackoffs);
      moved = true;
      break;
    } catch {
      await delay(ms);
    }
  }

  if (!moved) {
    console.warn(
      `[dev] Could not clean stale .next\\dev cache (still locked).\n` +
        `The dev server will still start (Next recreates .next\\dev), but to stop this\n` +
        `recurring permanently, run scripts\\windows-system-exclusion.ps1 once in an\n` +
        `elevated terminal. It adds .next + node_modules to Windows Defender exclusions\n` +
        `and (optionally) removes them from the Windows Search indexer. If the dev server\n` +
        `fails to start anyway, stop any leftover dev server and delete .next\\dev manually.`,
    );
  }
}

if (isWindows) await sweepStaleDevCache();

process.argv.splice(2, 0, "dev", ...bundler);
await import("next/dist/bin/next");
