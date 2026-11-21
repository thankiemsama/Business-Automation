// Nexa SSE fanout latency harness (dependency-free, Node 18+).
//
// Measures how quickly /api/v1/events SSE connections are told to
// refetch after a committed write. This is the observable symptom of
// the single-process SSE wound: in a multi-replica deployment where
// fanout is not wired to NATS, a replica that did not perform the
// write never refreshes its SSE streams, and this harness will show
// timeouts / large p95.
//
// Usage:
//   NEXA_SSE_API_URL=http://127.0.0.1:8080 \
//   NEXA_SSE_TOKENS=<owner-bearer-token>[,<owner-token-2>] \
//   NEXA_SSE_STREAMS=5 NEXA_SSE_WRITES=40 \
//   node loadtest/sse-latency.mjs
//
// Exit code 0 = all writes produced a refresh within the deadline.
// Exit code 1 = one or more writes missed (SSE fanout gap observed).

const apiUrl = (process.env.NEXA_SSE_API_URL || "http://127.0.0.1:8080").replace(/\/$/, "");
const tokens = (process.env.NEXA_SSE_TOKENS || "")
  .split(",")
  .map((t) => t.trim())
  .filter(Boolean);
const streamCount = clampInt(process.env.NEXA_SSE_STREAMS, 5, 1, 200, "NEXA_SSE_STREAMS");
const writeCount = clampInt(process.env.NEXA_SSE_WRITES, 40, 1, 10000, "NEXA_SSE_WRITES");
const refreshDeadlineMs = clampInt(process.env.NEXA_SSE_REFRESH_DEADLINE_MS, 5000, 100, 600000, "NEXA_SSE_REFRESH_DEADLINE_MS");

function clampInt(raw, fallback, min, max, name) {
  const value = Number(raw ?? fallback);
  if (!Number.isInteger(value) || value < min || value > max) {
    throw new Error(`${name} must be an integer between ${min} and ${max}`);
  }
  return value;
}

if (tokens.length === 0) {
  console.error("Set NEXA_SSE_TOKENS to one or more Bearer tokens for authenticated SSE.");
  process.exit(1);
}

// ── SSE client ──────────────────────────────────────────────────

// Opens an SSE stream and resolves per-`id:` lines through onEvent.
// Returns an async generator-free object: { close() } plus a promise
// that resolves when the stream ends.
function openSseStream(token, onId) {
  const ctrl = new AbortController();
  const fetchUrl = `${apiUrl}/api/v1/events`;
  const read = async () => {
    let buffer = "";
    try {
      const res = await fetch(fetchUrl, {
        headers: { Authorization: `Bearer ${token}`, Accept: "text/event-stream" },
        signal: ctrl.signal,
      });
      if (!res.ok || !res.body) {
        throw new Error(`SSE connect failed: ${res.status}`);
      }
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        let sep;
        while ((sep = buffer.indexOf("\n\n")) !== -1) {
          const frame = buffer.slice(0, sep);
          buffer = buffer.slice(sep + 2);
          const idLine = frame
            .split("\n")
            .find((l) => l.startsWith("id:"));
          if (idLine) onId(idLine.slice(3).trim());
        }
      }
    } catch (err) {
      if (err.name !== "AbortError") console.error("SSE stream error:", err.message);
    }
  };
  return {
    ready: read(),
    close: () => ctrl.abort(),
  };
}

// ── Main harness ────────────────────────────────────────────────

async function main() {
  // Each stream watches one tenant scope (its token's tenant) and
  // reports the latest id it has seen.
  const writers = tokens.map((t) => t); // which token performs the write
  const streams = [];
  const streamIds = new Array(streamCount).fill(null); // last id per stream

  for (let i = 0; i < streamCount; i++) {
    const token = tokens[i % tokens.length];
    streams.push(
      openSseStream(token, (id) => {
        streamIds[i] = id;
      }),
    );
  }

  // Wait for every stream to reach a steady cursor (first frame).
  const t0 = performance.now();
  while (streamIds.some((v) => v === null) && performance.now() - t0 < refreshDeadlineMs) {
    await new Promise((r) => setTimeout(r, 20));
  }
  if (streamIds.some((v) => v === null)) {
    console.error("Timed out waiting for SSE streams to connect.");
    process.exit(1);
  }

  const latencies = [];
  let missed = 0;
  const perStreamRefreshes = new Array(streamCount).fill(0);

  for (let w = 0; w < writeCount; w++) {
    const token = writers[w % writers.length];
    const start = performance.now();
    const target = streamIds; // snapshot array by reference
    const snapshot = [...streamIds];

    const createRes = await fetch(`${apiUrl}/api/v1/tickets`, {
      method: "POST",
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      body: JSON.stringify({ subject: `sse-latency ${w}`, priority: "normal" }),
    });
    if (createRes.status !== 201 && createRes.status !== 200) {
      console.error(`Write #${w} returned ${createRes.status}; stopping.`);
      process.exit(1);
    }

    // Wait until any stream observes an id different from its snapshot.
    const deadline = start + refreshDeadlineMs;
    let got = false;
    while (performance.now() < deadline) {
      got = streamIds.some((v, i) => v !== null && snapshot[i] !== null && v !== snapshot[i]);
      if (got) break;
      await new Promise((r) => setTimeout(r, 5));
    }
    if (got) {
      latencies.push(performance.now() - start);
      for (let i = 0; i < streamCount; i++) {
        if (streamIds[i] !== snapshot[i]) perStreamRefreshes[i]++;
      }
    } else {
      missed++;
    }
    // Advance the shared snapshot reference is implicit: next loop
    // re-snapshots the (possibly advanced) ids.
    void target;
  }

  for (const s of streams) s.close();

  // ── Report ──
  const pct = (p) => {
    if (latencies.length === 0) return "n/a";
    const sorted = [...latencies].sort((a, b) => a - b);
    const idx = Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length));
    return `${Math.round(sorted[idx])}ms`;
  };
  const avg =
    latencies.length === 0
      ? 0
      : latencies.reduce((a, b) => a + b, 0) / latencies.length;

  console.log("\n=== Nexa SSE fanout latency report ===");
  console.log(`API:              ${apiUrl}`);
  console.log(`Streams:          ${streamCount}`);
  console.log(`Writes:           ${writeCount}`);
  console.log(`Refresh deadline: ${refreshDeadlineMs}ms`);
  console.log(`Delivered:        ${latencies.length}/${writeCount}`);
  console.log(`Missed (no SSE refresh in time): ${missed}`);
  console.log(`Latency p50:      ${pct(50)}`);
  console.log(`Latency p95:      ${pct(95)}`);
  console.log(`Latency p99:      ${pct(99)}`);
  console.log(`Latency avg:      ${avg.toFixed(1)}ms`);
  console.log(`Per-stream refresh counts: ${JSON.stringify(perStreamRefreshes)}`);

  // In a healthy single-process or NATS-wired deployment, a committed
  // write should refresh *every* stream on that tenant. A stream that
  // never observed a refresh strongly indicates the fanout is local-only
  // and this replica did not perform the write.
  const starved = perStreamRefreshes.filter((n) => n === 0).length;
  console.log(`Starved streams (0 refreshes): ${starved}`);

  if (missed > 0 || starved > 0) {
    console.error("\nRESULT: FAIL — SSE fanout gap detected.");
    process.exit(1);
  }
  console.log("\nRESULT: PASS — all writes refreshed all SSE streams within deadline.");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
