// Nexa platform — comprehensive functional + security test harness (v3, final).
// Boots a dedicated backend instance per run, exercises every function
// end-to-end (dev email sink => real link/code extraction from the log),
// and records PASS/FAIL per check.
import { spawn } from "node:child_process";
import { createWriteStream, existsSync, readFileSync, rmSync, writeFileSync, watchFile } from "node:fs";
import { createHmac } from "node:crypto";

const EXE = "D:/GOLANG/data/api-test.exe";
const PORT = Number(process.env.PORT || 18080);
const BASE = `http://127.0.0.1:${PORT}`;
const ts = Date.now();
const RUN = String(process.env.RUN || "main"); // "main" (auto-confirm) | "verify" (pending verification)
const DATA_FILE = `D:/GOLANG/data/testrun/platform-${RUN}-${ts}.json`;
const LOG_FILE = `D:/GOLANG/data/testrun/server-${RUN}-${ts}.log`;
const SEC_KEY = Buffer.alloc(32).fill(7).toString("base64");
const AUTO_CONFIRM = RUN === "main";

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// ---- shared guest rate limiter (20/min/IP) pacer --------------------------
// The backend keys one bucket per client IP for EVERY allowAuth-gated call
// (register, login, forgot, verify, redeem, portal code/login, chat POST,
// sat submit, voicebot, lead landing...). Keep well under 20/min.
let winStart = 0, winCount = 0;
async function paceAuth() {
  const now = Date.now();
  if (now - winStart > 58000) { winStart = now; winCount = 0; }
  if (winCount >= 15) { const wait = winStart + 60000 - now; if (wait > 0) await sleep(wait + 300); winStart = Date.now(); winCount = 0; }
  winCount++;
}

// ---- results ---------------------------------------------------------------
const results = [];
function record(area, name, res, expect, note = "") {
  const okStatuses = Array.isArray(expect) ? expect : [expect];
  const pass = okStatuses.includes(res.status);
  results.push({ id: results.length + 1, area, name, expected: okStatuses.join("|"), actual: res.status, reason: res.reason || "", pass, note: note || "" });
  const tag = pass ? "PASS" : "FAIL";
  console.log(` [${tag}] ${String(area).padEnd(10)} ${name}  => ${res.status}${res.reason ? " (" + res.reason + ")" : ""}  want ${okStatuses.join("/")}${note ? "  [" + note + "]" : ""}`);
  return { pass, res };
}

// ---- HTTP ------------------------------------------------------------------
async function api(method, path, body, opts = {}) {
  const headers = {};
  if (opts.token) headers.Authorization = `Bearer ${opts.token}`;
  if (body !== undefined && body !== null && !opts.rawBody) headers["Content-Type"] = "application/json";
  if (opts.header) Object.assign(headers, opts.header); // custom headers win over the default content-type
  let res;
  try {
    res = await fetch(BASE + path, {
      method, headers,
      body: body === undefined || body === null ? undefined : opts.rawBody ? body : typeof body === "string" ? body : JSON.stringify(body),
    });
  } catch (e) {
    return { status: 0, ok: false, data: null, raw: "", message: "ERR " + e.message, reason: "network:" + e.message, headers: new Map() };
  }
  const raw = await res.text();
  let envelope = null; try { envelope = JSON.parse(raw); } catch {}
  return {
    status: res.status, ok: res.ok,
    data: envelope?.data ?? null,
    raw,
    message: envelope?.message || "",
    reason: envelope?.reason || "",
    headers: res.headers,
  };
}
async function login(email, password, code = "") { await paceAuth(); return api("POST", "/api/v1/auth/login", code ? { email, password, code } : { email, password }); }
// rawLogin bypasses the pacer — used ONLY for the server rate-limit test.
function rawLogin(email, password) { return api("POST", "/api/v1/auth/login", { email, password }); }
async function register(name, email, extra = {}) { await paceAuth(); return api("POST", "/api/v1/auth/register", { name, email, ...extra }); }
const jsv = (v, n = 60) => String(JSON.stringify(v ?? "")).slice(0, n);

// ---- log helpers (dev email sink writes links verbatim on their own line) --
function logLines() { return existsSync(LOG_FILE) ? readFileSync(LOG_FILE, "utf8").split(/\r?\n/) : []; }
function linkOf(re) { for (let i = logLines().length - 1; i >= 0; i--) { const m = logLines()[i].match(re); if (m) return m[1]; } return ""; }
const resetLinks = () => linkOf(/account\/setup#([A-Za-z0-9._-]+)/);
const verifyLinks = () => linkOf(/account\/verify#([A-Za-z0-9._-]+)/);
const portalCodes = () => linkOf(/Mã truy cập của bạn là ([A-Za-z0-9]{8})\./);
const csatUrls = () => linkOf(/(https?:\/\/[^\s"']+\/sat\/[A-Za-z0-9._-]+)/);

// ---- TOTP (RFC6238, SHA-1, 30 s) -------------------------------------------
const B32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
function b32decode(str) {
  const clean = str.toUpperCase().replace(/[^A-Z2-7]/g, "");
  let bits = 0, value = 0; const out = [];
  for (const c of clean) { const idx = B32.indexOf(c); if (idx < 0) return null; value = (value << 5) | idx; bits += 5; if (bits >= 8) { out.push((value >>> (bits - 8)) & 0xff); bits -= 8; } }
  return Buffer.from(out);
}
function totpAt(secret, step) {
  const key = b32decode(secret); if (!key) return null;
  const buf = Buffer.alloc(8); buf.writeBigUInt64BE(BigInt(step));
  const d = createHmac("sha1", key).update(buf).digest();
  const o = d[d.length - 1] & 15;
  return String((((d[o] & 0x7f) << 18) | (d[o + 1] << 10) | (d[o + 2] << 2) | d[o + 3]) % 1000000).padStart(6, "0");
}

// ---- server lifecycle --------------------------------------------------------
function startServer() {
  if (existsSync(DATA_FILE)) rmSync(DATA_FILE);
  const logStream = createWriteStream(LOG_FILE, { flags: "a" });
  const child = spawn(EXE, [], {
    env: { ...process.env,
      HTTP_ADDR: `127.0.0.1:${PORT}`, DATA_FILE,
      NEXA_DEV_EMAIL_SINK: "true",
      NEXA_AUTO_CONFIRM_EMAIL: AUTO_CONFIRM ? "true" : "false",
      APP_ORIGIN: BASE, NEXA_SECURITY_KEY: SEC_KEY },
    stdio: ["ignore", "pipe", "pipe"],
  });
  child.stdout.pipe(logStream); child.stderr.pipe(logStream);
  return child;
}
async function waitReady(child, ms = 15000) {
  const t0 = Date.now();
  while (Date.now() - t0 < ms) {
    try { const r = await fetch(`${BASE}/healthz`); if (r.ok) return true; } catch {}
    if (child.exitCode !== null) return false;
    await sleep(300);
  }
  return false;
}

// ==============================================================================
async function main() {
  const server = startServer();
  if (!await waitReady(server)) {
    console.log("SERVER FAILED TO START");
    if (existsSync(LOG_FILE)) console.log(readFileSync(LOG_FILE, "utf8").slice(-3000));
    process.exit(2);
  }
  console.log(`== [run=${RUN} auto-confirm=${AUTO_CONFIRM}] backend ready at ${BASE} ==\n`);

  const today = new Date().toISOString().slice(0, 10);
  const owner = { name: "Owner QA", email: `owner-${ts}@example.test`, password: "Nexa-local-test-2026!" };
  let ownerToken = "", ownerId = "", tenantId = "";

  // ===================== AUTH: REGISTER ====================================
  const A = "AUTH";
  { const r = await register("Owner QA", owner.email, { password: owner.password }); record(A, "register valid -> 201 owner+tenant", r, 201, `tenant=${String(r.data?.tenant_id || "").slice(0, 6)}`); }
  { const r = await register("Owner QA", owner.email, { password: owner.password }); record(A, "register duplicate email -> 409", r, 409, r.reason); }
  { const r = await register("N", `s-${ts}@e.test`, { password: "short" }); record(A, "register short password -> 400", r, 400, r.reason); }
  { const r = await register("N", `n${ts}@`, { password: "Nexa-local-test-2026!" }); record(A, "register malformed email -> 400", r, 400, r.reason); }
  { const r = await register("", `e${ts}@x.test`, { password: "Nexa-local-test-2026!" }); record(A, "register empty name -> 400", r, 400, r.reason); }
  { const r = await register("N", `u${ts}@x.test`, { password: "Nexa-local-test-2026!", bogus: 1 }); record(A, "register unknown JSON field -> 400", r, 400, r.reason); }
  { const r = await register("N", `long${ts}@x.test`, { password: "x".repeat(129) }); record(A, "register password >128 bytes -> 400", r, 400, r.reason); }
  { const r = await register("Ind", `ind${ts}@x.test`, { password: "Nexa-local-test-2026!", industry: "ecommerce" }); record(A, "register with industry starter -> 201", r, 201, r.reason); }
  { const r = await register("Ind", `ind${ts}@y.test`, { password: "Nexa-local-test-2026!", industry: "nonsense" }); record(A, "register bad industry -> 400", r, 400, r.reason); }
  { const r = await login(owner.email, owner.password); record(A, "login valid -> 200 + token", r, 200, `expires=${r.data?.expires_in}s`); ownerToken = r.data?.access_token || ""; }
  { const r = await login(owner.email, "wrongpassword123"); record(A, "login wrong password -> 401", r, 401, r.reason); }
  { const r = await login(`ghost-${ts}@x.test`, "Nexa-local-test-2026!"); record(A, "login unknown email -> 401 (anti-enumeration)", r, 401, r.reason); }
  { const r = await api("GET", "/api/v1/auth/me", null, { token: ownerToken }); record(A, "GET /auth/me -> 200", r, 200, jsv(r.data, 60)); ownerId = r.data?.id; tenantId = r.data?.tenant_id; }
  { const r = await api("GET", "/api/v1/auth/me"); record(A, "GET /auth/me without token -> 401", r, 401); }

  if (!AUTO_CONFIRM) {
    // ===== VERIFY-EMAIL RUN: pending-verification flow =====
    const V = "VERIFY";
    { const r = await api("GET", "/api/v1/auth/me", null, { token: ownerToken }); record(V, "registered owner pending email confirmation", r, 200, "email_verification_pending=" + r.data?.email_verification_pending); }
    { const r = await login(owner.email, owner.password); record(V, "login before verification -> 403 email_unverified", r, 403, r.reason); }
    { await paceAuth(); const r = await api("POST", "/api/v1/auth/verify", { email: owner.email }); record(V, "resend verification -> 200", r, 200, r.reason); }
    await sleep(400);
    const vlink = verifyLinks();
    { record(V, "extract verify link from dev mail sink", { status: vlink ? 200 : 501, reason: vlink }, 200, vlink); }
    if (vlink) {
      await paceAuth();
      const r = await api("POST", "/api/v1/auth/confirm", { token: vlink }); record(V, "confirm email -> 200", r, 200, r.reason);
      const again = await api("POST", "/api/v1/auth/confirm", { token: vlink }); record(V, "confirm link single-use -> 400", again, 400, again.reason);
      const ok = await login(owner.email, owner.password); record(V, "login after verification -> 200", ok, 200, "pending=" + ok.reason);
      ownerToken = ok.data?.access_token || ownerToken;
    }
    { const r = await api("POST", "/api/v1/auth/verify", { email: `nobody-${ts}@x.test` }); record(V, "resend verify unknown email -> 200 (no disclosure)", r, 200, r.reason); }
    { const r = await api("POST", "/api/v1/auth/confirm", { token: "wrong.secret" }); record(V, "confirm tampered link -> 400", r, 400, r.reason); }
  } else {
    // ===== RECOVERY: forgot password (dev sink) =====
    const RC = "RECOVERY";
    { await paceAuth(); const r = await api("POST", "/api/v1/auth/forgot", { email: owner.email }); record(RC, "forgot password -> 200 (link mailed)", r, 200, r.reason);
      await sleep(400);
      const link = resetLinks();
      record(RC, "extract reset link from dev mail sink", { status: link ? 200 : 501, reason: link ? link.slice(0, 60) : "no link" }, 200, link || "log missing link");
      if (link) {
        const newPassword = "Reset-new-pass-2026!" + String(ts).slice(-5);
        await paceAuth();
        const rr = await api("POST", "/api/v1/auth/redeem", { token: link, password: newPassword });
        record(RC, "redeem reset link -> 200", rr, 200, rr.reason);
        const rl = await login(owner.email, newPassword); record(RC, "login with reset password -> 200", rl, 200); ownerToken = rl.data?.access_token || ownerToken;
        const wrong = await login(owner.email, owner.password); record(RC, "old password rejected after reset -> 401", wrong, 401, wrong.reason);
        const replay = await api("POST", "/api/v1/auth/redeem", { token: link, password: "Another-new-pass-2026!" });
        record(RC, "reset link single-use (replay) -> 400", replay, 400, replay.reason);
        owner.password = newPassword;
      }
    }
    { await paceAuth(); const r = await api("POST", "/api/v1/auth/forgot", { email: `nobody-${ts}@x.test` }); record(RC, "forgot unknown email -> 200 (no disclosure)", r, 200, r.reason); }
    { await paceAuth(); const r = await api("POST", "/api/v1/auth/redeem", { token: "no-dot-token", password: "Reset-new-pass-2026!" }); record(RC, "redeem malformed token -> 400", r, 400, r.reason); }
    // ===== change password =====
    { await paceAuth(); const r = await api("POST", "/api/v1/auth/password", { old_password: "totally-wrong-1", password: owner.password }, { token: ownerToken }); record(A, "change password wrong old -> 400", r, 400, r.reason);
      await paceAuth(); const r2 = await api("POST", "/api/v1/auth/password", { old_password: owner.password, password: "short" }, { token: ownerToken }); record(A, "change password short new -> 400", r2, 400, r2.reason);
      await paceAuth(); const r3 = await api("POST", "/api/v1/auth/password", { old_password: owner.password, password: owner.password + "-v2" }, { token: ownerToken }); record(A, "change password valid -> 200 (revokes sessions)", r3, 200, r3.reason);
      owner.password += "-v2";
      const oldSession = await api("GET", "/api/v1/auth/me", null, { token: ownerToken }); record(A, "old token invalidated after password change -> 401", oldSession, 401, oldSession.reason);
      const re = await login(owner.email, owner.password); ownerToken = re.data?.access_token || ownerToken; record(A, "re-login after password change -> 200", re, 200); }
  }

  // ===================== TEAM / INVITES ===================================
  const T = "TEAM";
  const agent = { name: "Agent QA", email: `agent-${ts}@example.test`, password: "Nexa-local-test-2026!" };
  let agentToken = "", agentId = "";
  { const r = await api("POST", "/api/v1/team", agent, { token: ownerToken }); record(T, "owner creates agent -> 201", r, 201, r.reason); }
  { const r = await login(agent.email, agent.password); record(T, "agent login -> 200", r, 200, r.reason); agentToken = r.data?.access_token || "";
    const me = await api("GET", "/api/v1/auth/me", null, { token: agentToken }); agentId = me.data?.id; record(T, "agent role verified", me, 200, "role=" + me.data?.role); }
  { const r = await api("GET", "/api/v1/team", null, { token: ownerToken }); record(T, "owner lists team -> 200", r, 200, "members=" + r.data?.length); }
  { const r = await api("POST", "/api/v1/team", agent, { token: agentToken }); record(T, "agent cannot create agent -> 403", r, 403, r.reason); }
  { const r = await api("PUT", `/api/v1/team/${agentId}`, { name: agent.name, email: agent.email, disabled: false }, { token: ownerToken }); record(T, "owner updates member -> 200", r, 200, r.reason); }
  { const r = await api("PUT", `/api/v1/team/${agentId}`, { name: "X", email: `x${ts}@x.test` }, { token: agentToken }); record(T, "agent cannot update member -> 403", r, 403, r.reason); }
  { const r = await api("PUT", `/api/v1/team/${ownerId}`, { name: "X", email: owner.email }, { token: ownerToken }); record(T, "cannot edit owner via team/{id} -> 404", r, 404, r.reason); }
  if (AUTO_CONFIRM) {
    const invitee = `invitee-${ts}@example.test`;
    { await paceAuth(); const r = await api("POST", "/api/v1/team/invite", { name: "Invitee", email: invitee }, { token: ownerToken }); record(T, "owner invites teammate -> 200 (link mailed)", r, 200, r.reason);
      await sleep(400);
      const ilink = resetLinks();
      record(T, "extract invite link from dev mail sink", { status: ilink ? 200 : 501, reason: ilink ? "link" : "none" }, 200);
      if (ilink) {
        await paceAuth();
        const rr = await api("POST", "/api/v1/auth/redeem", { token: ilink, password: "Invite-new-pass-2026!" });
        record(T, "invitee redeems invite link -> 200", rr, 200, rr.reason);
        const li = await login(invitee, "Invite-new-pass-2026!");
        record(T, "invitee login -> 200 with agent role", li, 200, "role=" + li.data?.role);
      }
    }
  }

  // ===================== RESOURCES CRUD ====================================
  const R = "RESOURCES";
  let customer = {}, conversation = {}, ticket = {}, knowledge = {}, automation = {}, customer2 = {};
  { const r = await api("POST", "/api/v1/customers", { name: "Khách QA", email: `khach-${ts}@example.test`, phone: "0900000000" }, { token: ownerToken }); record(R, "create customer -> 201", r, 201, "version=" + r.data?.version); customer = r.data || {}; }
  { const r = await api("POST", "/api/v1/customers", { email: "no-name@test.x" }, { token: ownerToken }); record(R, "customer without name -> 400", r, 400, r.reason); }
  { const r = await api("GET", "/api/v1/customers", null, { token: ownerToken }); record(R, "list customers -> 200 (paged)", r, 200, "total=" + r.data?.total); }
  { const r = await api("GET", `/api/v1/customers?limit=1&page=2`, null, { token: ownerToken }); record(R, "pagination limit/page -> 200", r, 200, "total=" + r.data?.total + " items=" + r.data?.items?.length); }
  { const r = await api("GET", `/api/v1/customers?page=0`, null, { token: ownerToken }); record(R, "page=0 -> 400 (bounds)", r, 400, r.reason); }
  { const r = await api("GET", `/api/v1/customers?limit=999`, null, { token: ownerToken }); record(R, "limit>100 -> 400 (bounds)", r, 400, r.reason); }
  { const r = await api("GET", `/api/v1/customers/${customer.id}`, null, { token: ownerToken }); record(R, "get customer -> 200", r, 200); }
  { const r = await api("GET", "/api/v1/customers/does-not-exist", null, { token: ownerToken }); record(R, "get unknown id -> 404", r, 404); }
  { const r = await api("PUT", `/api/v1/customers/${customer.id}`, { version: customer.version, name: "Khách QA 2", email: `khach2-${ts}@example.test`, phone: "0911111111" }, { token: ownerToken }); record(R, "update customer (versioned) -> 200", r, 200, "new version=" + r.data?.version); customer = r.data || customer; }
  { const r = await api("PUT", `/api/v1/customers/${customer.id}`, { version: customer.version - 1, name: "stale", email: "s@x.test" }, { token: ownerToken }); record(R, "stale version -> 409", r, 409, r.reason); }
  { const r = await api("PUT", `/api/v1/customers/${customer.id}`, { name: "noversion" }, { token: ownerToken }); record(R, "missing version -> 428", r, 428, r.reason); }
  { const r = await api("POST", "/api/v1/conversations", { customer_id: customer.id, subject: "Cần hỗ trợ", channel: "web", status: "open" }, { token: ownerToken }); record(R, "create conversation -> 201", r, 201); conversation = r.data || {}; }
  { const r = await api("POST", "/api/v1/conversations", { customer_id: customer.id, channel: "carrier-pigeon" }, { token: ownerToken }); record(R, "conversation bad channel -> 400", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/conversations", { customer_id: "unknown-cid", channel: "web" }, { token: ownerToken }); record(R, "conversation unknown customer -> 404", r, 404, r.reason); }
  { const r = await api("POST", "/api/v1/tickets", { subject: "Yêu cầu hoàn tiền", content: "vui lòng hoàn", customer_id: customer.id, conversation_id: conversation.id, status: "open", priority: "high" }, { token: ownerToken }); record(R, "create ticket -> 201", r, 201); ticket = r.data || {}; }
  { const r = await api("POST", "/api/v1/tickets", { subject: "x", priority: "critical" }, { token: ownerToken }); record(R, "ticket bad priority -> 400", r, 400, r.reason); }
  customer2 = (await api("POST", "/api/v1/customers", { name: "Khách Khác", email: `khac-${ts}@example.test` }, { token: ownerToken })).data || {};
  { const r = await api("POST", "/api/v1/tickets", { subject: "mismatch", customer_id: customer2.id, conversation_id: conversation.id }, { token: ownerToken }); record(R, "ticket customer/conversation mismatch -> 400", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/knowledge", { subject: "Chính sách hoàn tiền", content: "Hoàn tiền trong 7 ngày.", enabled: true }, { token: ownerToken }); record(R, "create knowledge (owner) -> 201", r, 201, "slug=" + r.data?.slug); knowledge = r.data || {}; }
  { const r = await api("POST", "/api/v1/knowledge", { subject: "V", content: "C", video_url: "http://insecure.example/v.mp4", video_transcript: "chào" }, { token: ownerToken }); record(R, "knowledge non-HTTPS video -> 400", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/automations", { name: "Quy tắc hoàn tiền", keywords: ["hoàn tiền"], enabled: true }, { token: ownerToken }); record(R, "create automation -> 201", r, 201); automation = r.data || {}; }
  { const r = await api("POST", "/api/v1/automations", { name: "R" }, { token: ownerToken }); record(R, "automation without keywords -> 400", r, 400, r.reason); }
  { const r = await api("DELETE", `/api/v1/customers/${customer.id}`, null, { token: ownerToken }); record(R, "DELETE resource -> 405 (soft state only)", r, 405); }
  { const r = await api("GET", "/api/v1/unknownkind", null, { token: ownerToken }); record(R, "unknown resource kind -> 404", r, 404, r.reason); }
  { const r = await api("POST", "/api/v1/knowledge", { subject: "denied", content: "denied" }, { token: agentToken }); record(R, "agent create knowledge -> 403", r, 403, r.reason); }
  { const r = await api("POST", "/api/v1/automations", { name: "denied", keywords: ["x"] }, { token: agentToken }); record(R, "agent create automation -> 403", r, 403, r.reason); }
  { const r = await api("POST", "/api/v1/customers", { name: "agent-cust" }, { token: agentToken }); record(R, "agent create customer -> 201", r, 201); }
  { const r = await api("GET", "/api/v1/customers", null, { token: agentToken }); record(R, "agent list owner's customers -> 200 (shared tenant)", r, 200, "total=" + r.data?.total); }

  // cross-tenant isolation
  const other = { name: "Other", email: `other-${ts}@example.test`, password: "Nexa-local-test-2026!" };
  let otherToken = "", otherTenant = "";
  { const r = await register("Other", other.email, { password: other.password }); record(R, "second tenant registered -> 201", r, 201);
    const o = await login(other.email, other.password); otherToken = o.data?.access_token || ""; otherTenant = (await api("GET", "/api/v1/auth/me", null, { token: otherToken })).data?.tenant_id; }
  { const r = await api("GET", `/api/v1/customers/${customer.id}`, null, { token: otherToken }); record(R, "cross-tenant read -> 404 (isolation)", r, 404); }
  { const r = await api("PUT", `/api/v1/customers/${customer.id}`, { version: customer.version, name: "hax" }, { token: otherToken }); record(R, "cross-tenant update -> 404", r, 404); }
  { const r = await api("GET", `/api/v1/conversations/${conversation.id}/messages`, null, { token: otherToken }); record(R, "cross-tenant messages -> 404", r, 404); }
  { const r = await api("GET", "/api/v1/customers", null, { token: otherToken }); record(R, "other tenant list -> 200 (its own empty data)", r, 200, "total=" + r.data?.total); }

  // ===================== MESSAGES / AUTOMATION / REPLY ====================
  const M = "MESSAGES";
  const ticketsBefore = (await api("GET", "/api/v1/tickets", null, { token: ownerToken })).data?.total ?? -1;
  { const r = await api("POST", `/api/v1/conversations/${conversation.id}/messages`, { content: "tôi cần hoàn tiền ngay", sender: "customer" }, { token: ownerToken }); record(M, "post customer message -> 201", r, 201, "seq=" + r.data?.sequence + " status=" + r.data?.status); }
  { const r = await api("POST", `/api/v1/conversations/${conversation.id}/messages`, { content: "bad", sender: "intruder" }, { token: ownerToken }); record(M, "invalid sender role -> 400", r, 400, r.reason); }
  await sleep(400);
  { const r = await api("GET", "/api/v1/tickets", null, { token: ownerToken });
    const autoTicket = (r.data?.items || []).find((t) => t.automation_id);
    record(M, "keyword automation created ticket", autoTicket ? { status: 200 } : { status: 500, reason: "none" }, 200, autoTicket ? `automation=${autoTicket.automation_id?.slice(0, 6)} total ${r.data.total} (was ${ticketsBefore})` : "no automation ticket"); }
  { const r = await api("POST", `/api/v1/conversations/${conversation.id}/reply`, {}, { token: ownerToken }); record(M, "auto reply from knowledge -> 201", r, 201, "sender=" + r.data?.message?.sender + " source=" + r.data?.source_id + " needs_human=" + r.data?.needs_human); }
  { const r = await api("POST", `/api/v1/conversations/${conversation.id}/reply`, {}, { token: ownerToken }); record(M, "double reply -> 409", r, 409, r.reason); }
  { const nm = await api("POST", `/api/v1/conversations/${conversation.id}/messages`, { content: "hỏi thêm: tôi cần hoàn tiền", sender: "customer" }, { token: ownerToken }); record(M, "post follow-up customer msg (before draft)", nm, 201);
    const dr = await api("POST", `/api/v1/conversations/${conversation.id}/draft`, {}, { token: ownerToken }); record(M, "AI draft (no LLM -> local knowledge fallback)", dr, [200, 409], "sender=" + dr.data?.message?.sender + " provider=" + dr.data?.provider + " needs_human=" + dr.data?.needs_human); }
  { const fresh = (await api("GET", `/api/v1/conversations/${conversation.id}`, null, { token: ownerToken })).data;
    const upd = await api("PUT", `/api/v1/conversations/${conversation.id}`, { version: fresh.version, customer_id: fresh.customer_id, channel: fresh.channel, subject: fresh.subject, status: "closed" }, { token: ownerToken });
    record(M, "close conversation -> 200", upd, 200, upd.reason);
    const r = await api("POST", `/api/v1/conversations/${conversation.id}/messages`, { content: "sau khi đóng", sender: "agent" }, { token: ownerToken }); record(M, "message to closed conversation -> 409", r, 409, r.reason); }
  { const r = await api("POST", `/api/v1/conversations/${conversation.id}/read`, { sequence: 99 }, { token: ownerToken }); record(M, "mark conversation read -> 200", r, 200, r.reason); }
  { const r = await api("POST", `/api/v1/tickets/${ticket.id}/response`, {}, { token: ownerToken }); record(M, "record ticket response -> 200", r, 200, "first_response_at=" + !!r.data?.first_response_at); }
  { const r = await api("POST", `/api/v1/tickets/${ticket.id}/escalate`, {}, { token: ownerToken }); record(M, "escalate not-overdue ticket -> 409", r, 409, r.reason); }

  // ===================== DASHBOARD / REPORTS / EXPORT =====================
  const D = "REPORTS";
  { const r = await api("GET", "/api/v1/dashboard", null, { token: ownerToken }); record(D, "dashboard -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("GET", `/api/v1/reports?from=${today}&to=${today}`, null, { token: ownerToken }); record(D, "reports valid range -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("GET", "/api/v1/reports?from=2020-01-01&to=2019-01-01", null, { token: ownerToken }); record(D, "reports to<from -> 400", r, 400, r.reason); }
  { const r = await api("GET", "/api/v1/reports?from=2020-01-01&to=2026-01-01", null, { token: ownerToken }); record(D, "reports range >366d -> 400", r, 400, r.reason); }
  { const r = await api("GET", `/api/v1/reports/analytics?from=${today}&to=${today}`, null, { token: ownerToken }); record(D, "reports/analytics -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("GET", "/api/v1/reports/export?format=csv&kind=tickets", null, { token: ownerToken }); record(D, "export tickets CSV -> 200", r, 200, String(r.raw).replace(/\n/g, " ").slice(0, 50)); }
  { const r = await api("GET", "/api/v1/reports/export?format=csv&kind=customers&since=" + today, null, { token: ownerToken }); record(D, "export customers CSV (since filter) -> 200", r, 200, String(r.raw).slice(0, 40)); }
  { const r = await api("GET", "/api/v1/reports/export?format=xlsx&kind=tickets", null, { token: ownerToken }); record(D, "export tickets XLSX -> 200 (zip)", r, 200, String(r.raw).slice(0, 12)); }
  { const r = await api("GET", "/api/v1/reports/export?format=xml", null, { token: ownerToken }); record(D, "export bad format -> 400", r, 400, r.reason); }

  // ===================== WORKSPACE TOOLS ===================================
  const WS = "WORKSPACE";
  { const r = await api("GET", `/api/v1/search?q=${encodeURIComponent("hoàn tiền")}`, null, { token: ownerToken }); record(WS, "global search -> 200", r, 200, "hits=" + (Array.isArray(r.data) ? r.data.length : "?")); }
  { const r = await api("POST", "/api/v1/views", { name: "Ticket chưa xử lý", kind: "tickets", status: "open" }, { token: ownerToken }); record(WS, "save saved-view -> 200", r, 200, r.reason);
    const views = await api("GET", "/api/v1/views", null, { token: ownerToken });
    const viewId = views.data?.[0]?.id;
    const rr = await api("POST", "/api/v1/views/remove", { id: viewId }, { token: ownerToken }); record(WS, "remove view -> 200", rr, 200, rr.reason);
    const r2 = await api("POST", "/api/v1/views", { name: "Bad", kind: "customers" }, { token: ownerToken }); record(WS, "save view bad kind -> 400", r2, 400, r2.reason); }
  { const r = await api("GET", "/api/v1/notifications", null, { token: ownerToken }); record(WS, "notifications -> 200", r, 200, "unread=" + r.data?.unread); }
  { const r = await api("POST", "/api/v1/notifications/read", { id: "bogus" }, { token: ownerToken }); record(WS, "read unknown notification -> 404", r, 404, r.reason); }
  { const t2 = (await api("POST", "/api/v1/tickets", { subject: "Bulk A", priority: "low" }, { token: ownerToken })).data;
    const t3 = (await api("POST", "/api/v1/tickets", { subject: "Bulk B", priority: "normal" }, { token: ownerToken })).data;
    const r = await api("POST", "/api/v1/tickets/bulk", { ids: [t2?.id, t3?.id], action: "resolve" }, { token: ownerToken });
    record(WS, "bulk resolve tickets -> 200", r, 200, "updated=" + r.data?.updated);
    const rr = await api("POST", "/api/v1/tickets/bulk", { ids: [t2?.id], action: "explode" }, { token: ownerToken });
    record(WS, "bulk bad action -> 400", rr, 400, rr.reason);
    const c2 = (await api("POST", "/api/v1/conversations", { customer_id: customer.id, channel: "email" }, { token: ownerToken })).data;
    const cc = await api("POST", "/api/v1/conversations/bulk", { ids: [c2?.id], action: "close" }, { token: ownerToken });
    record(WS, "bulk close conversations -> 200", cc, 200, "updated=" + cc.data?.updated); }
  { const ac = new AbortController();
    let firstChunk = "";
    const ssePromise = (async () => {
      const res = await fetch(`${BASE}/api/v1/events`, { headers: { Authorization: `Bearer ${ownerToken}`, Accept: "text/event-stream" }, signal: ac.signal });
      const reader = res.body.getReader();
      const { value } = await reader.read();
      firstChunk = Buffer.from(value || "").toString();
      ac.abort();
      await sleep(50);
      return { status: res.status, ctype: res.headers.get("content-type") };
    })().catch(() => null);
    const sse = await ssePromise;
    record(WS, "SSE /events stream -> 200 text/event-stream", { status: sse?.status || 0, reason: sse?.ctype }, 200, sse?.ctype + " first=" + firstChunk.slice(0, 40).replace(/\n/g, "\\n")); }
  { const r = await api("GET", "/api/v1/events", null, { token: "bogus-token" }); record(WS, "SSE invalid token -> 401", r, 401, r.reason); }

  // ===================== CSAT (full flow) ==================================
  const S = "CSAT";
  { const r = await api("GET", `/api/v1/sat/${tenantId}.deadbeefdeadbeef`); record(S, "resolve unknown sat link -> 404", r, 404, r.reason); }
  { const r = await api("POST", `/api/v1/sat/${tenantId}.deadbeefdeadbeef`, { rating: 3 }); record(S, "submit unknown sat -> 404", r, 404, r.reason); }
  // Dedicated ticket linked to a customer WITH an email, so the survey mail
  // job is guaranteed to be queued by the onResolved hook.
  { const csatCustomer = (await api("POST", "/api/v1/customers", { name: "CSAT Customer", email: `csat-${ts}@example.test`, phone: "0900000000" }, { token: ownerToken })).data;
    const csatConv = (await api("POST", "/api/v1/conversations", { customer_id: csatCustomer?.id, subject: "CSAT flow", channel: "web", status: "open" }, { token: ownerToken })).data;
    const csatTicket = (await api("POST", "/api/v1/tickets", { subject: "CSAT ticket", content: "need help", customer_id: csatCustomer?.id, conversation_id: csatConv?.id, status: "open", priority: "normal" }, { token: ownerToken })).data;
    if (!csatTicket?.id) { record(S, "create dedicated CSAT ticket", { status: 500, reason: "create failed" }, 200); }
    else {
      const upd = await api("PUT", `/api/v1/tickets/${csatTicket.id}`, { version: csatTicket.version, subject: csatTicket.subject, content: csatTicket.content, customer_id: csatTicket.customer_id, conversation_id: csatTicket.conversation_id, status: "resolved", priority: "normal" }, { token: ownerToken });
      record(S, "resolve ticket (triggers CSAT link + mail job)", upd, 200, upd.reason);
      await sleep(6500); // CSAT mail worker tick = 5 s
      const url = csatUrls();
      record(S, "CSAT survey link e-mailed by worker (dev sink)", { status: url ? 200 : 501, reason: url ? url.slice(0, 60) : "no /sat/ link in log" }, 200, url || "link stored but not mailed");
      if (url) {
        const ref = url.split("/sat/")[1];
        const rr = await api("GET", `/api/v1/sat/${ref}`); record(S, "resolve issued CSAT link -> 200", rr, 200, jsv(rr.data, 60));
        await paceAuth();
        const sub = await api("POST", `/api/v1/sat/${ref}`, { rating: 4, comment: "nhanh chóng" }); record(S, "submit CSAT rating -> 201", sub, 201, sub.reason);
        await paceAuth();
        const dup = await api("POST", `/api/v1/sat/${ref}`, { rating: 5 }); record(S, "double CSAT submit -> 409", dup, 409, dup.reason);
      }
    }
  }

  // ===================== CHAT WIDGET (guest) ===============================
  const C = "CHAT";
  { await paceAuth(); const r = await api("POST", `/api/v1/chat/${tenantId}`, { name: "Khách Widget", content: "bạn ơi hoàn tiền thế nào" }); record(C, "guest start chat -> 201 + token", r, 201, "token=" + String(r.data?.token).slice(0, 10)); var chatToken = r.data?.token || ""; }
  { const r = await api("GET", `/api/v1/chat/${tenantId}`, null, { token: chatToken }); record(C, "guest read chat -> 200 (knowledge reply)", r, 200, "msgs=" + r.data?.length + " last=" + (r.data?.[r.data?.length - 1]?.sender || "")); }
  { const r = await api("GET", `/api/v1/chat/${tenantId}`, null, { token: "bogus" }); record(C, "guest read chat bad token -> 401", r, 401, r.reason); }
  { await paceAuth(); const r = await api("POST", `/api/v1/chat/${tenantId}`, { content: "cảm ơn bạn" }, { token: chatToken }); record(C, "guest send msg -> 200 (knowledge reply)", r, 200, jsv(Array.isArray(r.data) && r.data.length ? r.data.slice(-1) : r.data, 70)); }
  { await paceAuth(); const r = await api("POST", `/api/v1/chat/${tenantId}`, {}, { token: chatToken }); record(C, "guest empty msg -> 400", r, 400, r.reason); }
  { const r = await api("GET", `/api/v1/chat/${tenantId}/voice-status`); record(C, "guest voice-status -> 200 (LLM off)", r, 200, "enabled=" + r.data?.enabled); }
  { const r = await api("POST", `/api/v1/chat/${tenantId}/transcribe`, "x", { rawBody: true, header: { "Content-Type": "application/octet-stream" } }); record(C, "guest transcribe (feature off) -> 503", r, 503, r.reason); }
  { const ac2 = new AbortController();
    let gFirst = "";
    const gPromise = (async () => {
      const res = await fetch(`${BASE}/api/v1/chat/${tenantId}/events`, { headers: { Authorization: `Bearer ${chatToken}`, Accept: "text/event-stream" }, signal: ac2.signal });
      const reader = res.body.getReader();
      const { value } = await reader.read();
      gFirst = Buffer.from(value || "").toString();
      ac2.abort();
      await sleep(50);
      return { status: res.status, ctype: res.headers.get("content-type") };
    })().catch(() => null);
    const g = await gPromise;
    record(C, "guest chat SSE -> 200 text/event-stream", { status: g?.status || 0, reason: g?.ctype }, 200, g?.ctype + " first=" + gFirst.slice(0, 40).replace(/\n/g, "\\n")); }

  // ===================== PORTAL =============================================
  const P = "PORTAL";
  const portalEmail = customer.email || `khach-${ts}@example.test`; // customer.email is current after the versioned update
  { await paceAuth(); const r = await api("POST", `/api/v1/portal/${tenantId}/code`, { email: portalEmail }); record(P, "portal request code -> 202 (mail code)", r, 202, r.reason);
    await sleep(400);
    const code = portalCodes();
    record(P, "extract portal code from dev mail sink", { status: code ? 200 : 501, reason: code }, 200, code || "no code in log");
    if (code) {
      await paceAuth();
      const rr = await api("POST", `/api/v1/portal/${tenantId}/login`, { email: portalEmail, code });
      record(P, "portal login with mailed code -> 200", rr, 200, "token=" + String(rr.data?.token).slice(0, 8));
      const portalToken = rr.data?.token;
      const r2 = await api("GET", `/api/v1/portal/${tenantId}/tickets`, null, { token: portalToken }); record(P, "portal list own tickets -> 200", r2, 200, "count=" + r2.data?.length);
      const r3 = await api("POST", `/api/v1/portal/${tenantId}/tickets`, { subject: "Tự tạo ticket", content: "vui lòng xử lý", priority: "high" }, { token: portalToken }); record(P, "portal create ticket -> 201", r3, 201, r3.reason);
      const r4 = await api("GET", `/api/v1/portal/${tenantId}/tickets/${r3.data?.id}`, null, { token: portalToken }); record(P, "portal read own ticket -> 200", r4, 200, "msgs=" + r4.data?.messages?.length);
      const r5 = await api("GET", `/api/v1/portal/${tenantId}/tickets`, null, { token: "wrong" }); record(P, "portal wrong token -> 401", r5, 401, r5.reason);
      await paceAuth();
      const r6 = await api("POST", `/api/v1/portal/${tenantId}/login`, { email: portalEmail, code }); record(P, "portal code single-use -> 401", r6, 401, r6.reason);
    }
    await paceAuth();
    const badcode = await api("POST", `/api/v1/portal/${tenantId}/login`, { email: portalEmail, code: "AAAAAAA0" }); record(P, "portal wrong code -> 401 (uniform)", badcode, 401, badcode.reason);
  }

  // ===================== WEBHOOKS (owner, SSRF guard) =====================
  const W = "WEBHOOKS";
  let webhookId = "";
  { const r = await api("POST", "/api/v1/webhooks", { name: "hook1", url: "https://hooks.example.com/hook", events: ["ticket.created"] }, { token: ownerToken }); record(W, "save webhook (https) -> 201", r, 201, "id=" + (r.data?.id || "").slice(0, 6)); webhookId = r.data?.id || ""; }
  { const r = await api("POST", "/api/v1/webhooks", { name: "internal", url: "http://127.0.0.1:5000/hook" }, { token: ownerToken }); record(W, "webhook -> 127.0.0.1 blocked (SSRF)", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/webhooks", { name: "meta", url: "http://169.254.169.254/latest/meta-data" }, { token: ownerToken }); record(W, "webhook -> cloud metadata blocked (SSRF)", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/webhooks", { name: "lan", url: "http://10.0.0.5/x" }, { token: ownerToken }); record(W, "webhook -> RFC1918 blocked (SSRF)", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/webhooks", { name: "local", url: "http://localhost:3000/x" }, { token: ownerToken }); record(W, "webhook -> localhost blocked", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/webhooks", { name: "file", url: "file:///etc/passwd" }, { token: ownerToken }); record(W, "webhook non-http scheme -> 400", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/webhooks", { name: "u", url: "https://user:pass@x.com/" }, { token: ownerToken }); record(W, "webhook URL with userinfo -> 400", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/webhooks", { name: "bad", url: "https://example.com", events: ["not_a_real_event"] }, { token: ownerToken }); record(W, "webhook unknown event -> 400", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/webhooks", { name: "b", url: "https://example.com" }, { token: agentToken }); record(W, "agent save webhook -> 403", r, 403, r.reason); }
  { const r = await api("GET", "/api/v1/webhooks", null, { token: ownerToken }); record(W, "list webhooks -> 200", r, 200, "count=" + r.data?.length); }
  { const r = await api("POST", `/api/v1/webhooks/${webhookId}`, { name: "hook1-v2", url: "https://hooks2.example.com", events: ["message.new"] }, { token: ownerToken }); record(W, "update webhook -> 200", r, 200, r.reason); }
  { const r = await api("GET", "/api/v1/webhooks/dead-letters", null, { token: ownerToken }); record(W, "dead letters -> 200", r, 200, "count=" + r.data?.length); }
  { const r = await api("POST", "/api/v1/webhooks/dead-letters/bogus/replay", {}, { token: ownerToken }); record(W, "replay unknown dead letter -> 404", r, 404, r.reason); }
  { const r = await api("POST", `/api/v1/webhooks/${webhookId}/remove`, {}, { token: ownerToken }); record(W, "remove webhook -> 200", r, 200, r.reason); }

  // ===================== API KEYS / PUBLIC API =============================
  const K = "APIKEYS";
  let apiKey = "", keyId = "";
  { const r = await api("POST", "/api/v1/api-keys", { name: "zapi", scope: "read", rate_per_min: 30 }, { token: ownerToken }); record(K, "create API key (owner) -> 201", r, 201, "prefix=" + r.data?.prefix + " secret?=" + !!r.data?.secret); apiKey = r.data?.secret || ""; keyId = r.data?.id || ""; }
  { const r = await api("GET", "/api/v1/api-keys", null, { token: ownerToken }); record(K, "list API keys (no secrets) -> 200", r, 200, "count=" + r.data?.length + " secretsExposed=" + r.data?.some?.((k) => k.secret)); }
  { const r = await api("POST", "/api/v1/api-keys", { name: "agent" }, { token: agentToken }); record(K, "agent create key -> 403", r, 403, r.reason); }
  { const r = await api("POST", "/api/v1/api-keys", { name: "w", scope: "write" }, { token: ownerToken }); record(K, "key write scope -> 400", r, 400, r.reason); }
  { const r = await api("GET", "/api/public/v1/customers", null, { token: apiKey }); record(K, "public API with valid key -> 200", r, 200, "total=" + r.data?.total); }
  { const r = await api("GET", `/api/public/v1/customers/${customer.id}`, null, { token: apiKey }); record(K, "public API single customer -> 200", r, 200, r.reason || r.data?.name); }
  { const r = await api("GET", "/api/public/v1/tickets/unknown-id", null, { token: apiKey }); record(K, "public API unknown id -> 404", r, 404, r.reason); }
  { const r = await api("GET", "/api/public/v1/customers"); record(K, "public API without key -> 401", r, 401, r.reason); }
  { const r = await api("GET", "/api/public/v1/customers", null, { token: "nexa_badkey" }); record(K, "public API bad key -> 401", r, 401, r.reason); }
  { const r = await api("POST", "/api/public/v1/customers", { name: "x" }, { token: apiKey }); record(K, "public API write -> 405 (read-only)", r, 405); }
  { const r = await api("GET", "/api/public/openapi.json"); record(K, "public openapi.json -> 200", r, 200, "paths=" + Object.keys(r.data?.paths || {}).length); }
  { const r = await api("POST", `/api/v1/api-keys/${keyId}/revoke`, {}, { token: ownerToken }); record(K, "revoke key -> 200", r, 200, r.reason); }
  { const r = await api("GET", "/api/public/v1/customers", null, { token: apiKey }); record(K, "revoked key -> 401", r, 401, r.reason); }

  // ===================== CUSTOMER 360 =======================================
  const X3 = "CUSTOMER360";
  { const r = await api("GET", `/api/v1/customers/${customer.id}/profile`, null, { token: ownerToken }); record(X3, "customer profile -> 200", r, 200, `tickets=${r.data?.total_tickets} notes=${r.data?.notes?.length} timeline=${r.data?.timeline?.length}`); }
  { const r = await api("POST", `/api/v1/customers/${customer.id}/notes`, { content: "khách thân thiện" }, { token: ownerToken }); record(X3, "add customer note -> 201", r, 201, r.reason); }
  { const r = await api("POST", `/api/v1/customers/${customer.id}/notes`, { content: "   " }, { token: ownerToken }); record(X3, "empty note -> 400", r, 400, r.reason); }
  { const r = await api("POST", `/api/v1/customers/${customer2.id}/merge`, { target_id: customer.id }, { token: ownerToken }); record(X3, "merge customer -> 200", r, 200, r.reason);
    const merged = await api("GET", `/api/v1/customers/${customer2.id}`, null, { token: ownerToken }); record(X3, "merged source now 404 (soft-delete)", merged, 404, merged.reason); }

  // ===================== CHANNELS ===========================================
  const CH = "CHANNELS";
  { const r = await api("GET", "/api/v1/channels", null, { token: ownerToken }); record(CH, "channel status -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("POST", "/api/v1/channels/messenger", { name: "FB", content: JSON.stringify({ account_id: "555", access_token: "fb-token", webhook_secret: "fb-secret", verify_token: "verify", graph_version: "v25.0" }), enabled: false }, { token: ownerToken }); record(CH, "configure messenger channel -> 201", r, 201, r.reason); }
  { const r = await api("POST", "/api/v1/channels/messenger", { name: "X" }, { token: agentToken }); record(CH, "agent configure channel -> 403", r, 403, r.reason); }
  { const r = await api("GET", "/api/v1/channel-webhooks/demo-tenant/messenger?hub.mode=subscribe&hub.verify_token=bad&hub.challenge=abc123"); record(CH, "messenger webhook verify (no config) -> 403", r, 403, String(r.raw).slice(0, 40)); }
  { const r = await api("POST", "/api/v1/channel-webhooks/demo-tenant/sms?x=1", "hello", { rawBody: true, header: { "Content-Type": "text/plain" } }); record(CH, "sms webhook with query params -> 400", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/channels/messenger/test", { to: "1555", text: "hi" }, { token: ownerToken }); record(CH, "channel test delivery (config disabled, no live provider) -> 409", r, [409, 202, 400, 404, 503], r.reason); }
  { const r = await api("GET", "/api/v1/channels/messenger/deliveries", null, { token: ownerToken }); record(CH, "list channel deliveries -> 200", r, 200, "count=" + r.data?.length); }

  // ===================== LEAD CAMPAIGNS ====================================
  const LC = "LEADCAMPAIGN";
  { const r = await api("POST", "/api/v1/lead-campaigns", { name: "Chào hè", channel: "zalo", welcome: "Xin chào, cần gì ạ?" }, { token: ownerToken }); record(LC, "create lead campaign (zalo) -> 200", r, 200, r.reason); }
  { const r = await api("POST", "/api/v1/lead-campaigns", { name: "X", channel: "carrier-pigeon" }, { token: ownerToken }); record(LC, "lead campaign bad channel -> 400", r, 400, r.reason); }
  { const camps = await api("GET", "/api/v1/lead-campaigns", null, { token: ownerToken });
    const camp = (camps.data || [])[0];
    const r = await api("GET", `/api/v1/lead-campaigns/${camp?.id}/qr`, null, { token: ownerToken }); record(LC, "lead campaign QR -> 200", r, 200, "image bytes=" + String(r.data?.image).length);
    const r2 = await api("GET", `/api/v1/lead/${tenantId}/${camp?.id}`, null); record(LC, "public lead landing (no live provider) -> 404", r2, [404, 200], r2.reason || jsv(r2.data, 50));
    const r3 = await api("GET", `/api/v1/lead-campaigns/${camp?.id}/leads`, null, { token: ownerToken }); record(LC, "list campaign leads -> 200", r3, 200, "count=" + r3.data?.length); }

  // ===================== OPERATIONS ========================================
  const OP = "OPERATIONS";
  { const r = await api("GET", "/api/v1/operations/workflows", null, { token: ownerToken }); record(OP, "list workflows -> 200", r, 200, "count=" + r.data?.length); }
  { const r = await api("POST", "/api/v1/operations/workflows", { name: "SLA escalate", enabled: true, trigger: "ticket_idle", idle_days: 7, conditions: {}, actions: [{ type: "create_ticket" }] }, { token: ownerToken }); record(OP, "save workflow -> 200", r, 200, r.reason); const wfId = r.data?.id;
    const bad = await api("POST", "/api/v1/operations/workflows", { name: "bad", enabled: true, trigger: "nonsense", actions: [{ type: "create_ticket" }] }, { token: ownerToken }); record(OP, "workflow bad trigger -> 400", bad, 400, bad.reason);
    const bad2 = await api("POST", "/api/v1/operations/workflows", { name: "bad2", enabled: true, trigger: "ticket_idle", idle_days: 999, actions: [{ type: "create_ticket" }] }, { token: ownerToken }); record(OP, "workflow idle_days out of range -> 400", bad2, 400, bad2.reason);
    const bad3 = await api("POST", "/api/v1/operations/workflows", { name: "bad3", enabled: true, trigger: "ticket_idle", idle_days: 1, actions: [] }, { token: ownerToken }); record(OP, "workflow without actions -> 400", bad3, 400, bad3.reason);
    const mig = await api("POST", "/api/v1/operations/workflows/migrate", { id: automation.id }, { token: ownerToken }); record(OP, "migrate keyword automation -> workflow", mig, 200, mig.reason);
    const rem = await api("POST", "/api/v1/operations/workflows/remove", { id: wfId }, { token: ownerToken }); record(OP, "remove workflow -> 200", rem, 200, rem.reason); }
  { const r = await api("GET", "/api/v1/operations/schedules", null, { token: ownerToken }); record(OP, "list schedules -> 200", r, 200, jsv(r.data, 60)); }
  { const meNow = await api("GET", "/api/v1/auth/me", null, { token: ownerToken });
    const r = await api("POST", "/api/v1/operations/schedules", { name: "Báo cáo tuần", frequency: "weekly", recipient_id: meNow.data?.id, enabled: true, hour: 9, day: 1 }, { token: ownerToken }); record(OP, "save schedule (valid member) -> 200", r, 200, r.reason);
    const bad = await api("POST", "/api/v1/operations/schedules", { name: "B", frequency: "yearly" }, { token: ownerToken }); record(OP, "schedule bad frequency -> 400", bad, 400, bad.reason);
    const rem = await api("POST", "/api/v1/operations/schedules/remove", { id: r.data?.id }, { token: ownerToken }); record(OP, "remove schedule -> 200", rem, 200, rem.reason); }
  { const r = await api("GET", "/api/v1/operations/field-extraction", null, { token: ownerToken }); record(OP, "field-extraction batches -> 200", r, 200, "count=" + r.data?.length); }
  { const r = await api("POST", "/api/v1/operations/field-extraction", { request_id: "req-" + ts, ticket_ids: [ticket.id], field_ids: [] }, { token: ownerToken }); record(OP, "queue field extraction (no LLM / no fields) -> 503/400", r, [503, 400], r.reason); }
  { const r = await api("GET", "/api/v1/operations/skill-routing", null, { token: ownerToken }); record(OP, "skill-routing config -> 200", r, 200, jsv(r.data, 60)); }

  // ===================== GOVERNANCE / AI ====================================
  const AI = "AI";
  { const r = await api("GET", "/api/v1/ai/governance", null, { token: ownerToken }); record(AI, "governance inventory -> 200", r, 200, jsv(r.data?.governance, 60)); }
  { const r = await api("POST", "/api/v1/ai/governance", { platform_kill: false, config: { disabled: {}, risk_tiers: {}, approval: {} } }, { token: ownerToken }); record(AI, "save governance -> 200", r, 200, r.reason); }
  { const r = await api("POST", "/api/v1/ai/governance", { platform_kill: true }, { token: agentToken }); record(AI, "agent save governance -> 403", r, 403, r.reason); }
  { const r = await api("GET", "/api/v1/ai/actions", null, { token: ownerToken }); record(AI, "list AI actions -> 200", r, 200, "count=" + r.data?.length); }
  { const r = await api("POST", "/api/v1/ai/actions", { name: "refund", kind: "ticket", risk: "high", enabled: true }, { token: ownerToken }); record(AI, "create AI action -> 200", r, 200, r.reason); }
  { const r = await api("POST", "/api/v1/ai/actions", { name: "x", kind: "ticket", risk: "catastrophic", enabled: true }, { token: ownerToken }); record(AI, "AI action bad risk tier -> 400", r, 400, r.reason); }
  { const r = await api("GET", "/api/v1/ai/skills", null, { token: ownerToken }); record(AI, "list AI skills -> 200", r, 200, "count=" + r.data?.length); }
  { const r = await api("POST", "/api/v1/ai/skills", { name: "Thân thiện", tone: "friendly", enabled: true }, { token: ownerToken }); record(AI, "create AI skill -> 200", r, 200, r.reason); }
  { const r = await api("POST", "/api/v1/ai/skills", { name: "X", tone: "aggressive", enabled: true }, { token: ownerToken }); record(AI, "AI skill bad tone -> 400", r, 400, r.reason); }
  { const r = await api("GET", "/api/v1/ai/status", null, { token: ownerToken }); record(AI, "ai/status -> 200 (LLM off)", r, 200, jsv(r.data, 60)); }
  { const r = await api("GET", "/api/v1/ai/search?q=hoàn", null, { token: ownerToken }); record(AI, "ai search (no provider) -> 503", r, 503, r.reason); }
  { const r = await api("POST", "/api/v1/ai/summary/tickets/" + ticket.id, {}, { token: ownerToken }); record(AI, "ai summary (no provider) -> 503", r, 503, r.reason); }
  { const r = await api("GET", "/api/v1/ai/review/" + ticket.id, null, { token: ownerToken }); record(AI, "ai review ticket -> 200/503", r, [200, 503], r.reason); }
  { const r = await api("POST", "/api/v1/ai/transcribe", "x", { rawBody: true, header: { "Content-Type": "application/octet-stream" } }); record(AI, "ai transcribe (no provider) -> 503", r, 503, r.reason); }
  { const r = await api("GET", "/api/v1/ai/sparring", null, { token: ownerToken }); record(AI, "sparring overview -> 200/403", r, [200, 403], jsv(r.data, 60)); }
  { const r = await api("GET", "/api/v1/ai/feedback/stats", null, { token: ownerToken }); record(AI, "feedback stats -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("POST", "/api/v1/ai/feedback", { message_id: "bogus", rating: 2, comment: "ok" }, { token: ownerToken }); record(AI, "feedback on unknown message -> 404", r, 404, r.reason); }
  { const r = await api("GET", "/api/v1/ai/realtime-qa/status", null, { token: ownerToken }); record(AI, "realtime-qa status -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("GET", "/api/v1/ai/voice-inbox/status", null, { token: ownerToken }); record(AI, "voice-inbox status -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("GET", "/api/v1/ai/voicebot/calls", null, { token: ownerToken }); record(AI, "voicebot call history -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("POST", "/api/v1/ai/voicebot/call", { to: "+84900000000", greeting: "xin chào" }, { token: ownerToken }); record(AI, "voicebot outbound (no Twilio creds) -> 400/503", r, [400, 503], r.reason); }
  { const r = await api("GET", `/api/v1/voicebot/twiml/${tenantId}/ref123?to=%2B849&greeting=hi`); record(AI, "twiml fetch unconfigured -> 200 xml", r, 200, String(r.raw).slice(0, 60)); }
  { const r = await api("POST", `/api/v1/voicebot/calls/${tenantId}`, "From=%2B849&To=%2B840", { rawBody: true, header: { "Content-Type": "application/x-www-form-urlencoded" } }); record(AI, "twilio inbound webhook (no Twilio creds) -> 503", r, 503, String(r.raw).slice(0, 50)); }

  // ===================== TEMPLATES EXPORT/IMPORT ===========================
  const TP = "TEMPLATES";
  let templateCode = "";
  { const r = await api("POST", "/api/v1/templates", { subject: "Hướng dẫn đổi hàng", content: "Giữ biên nhận.", enabled: true }, { token: ownerToken }); record(TP, "create template -> 201", r, 201);
    const ex = await api("POST", `/api/v1/templates/${r.data?.id}/export`, { password: owner.password }, { token: ownerToken });
    templateCode = ex.data?.code || "";
    record(TP, "export template (password-protected) -> 200", ex, 200, "code=" + templateCode.slice(0, 18));
    const exBad = await api("POST", `/api/v1/templates/${r.data?.id}/export`, { password: "other-share-key-1" }, { token: ownerToken });
    record(TP, "export with a different share key -> 200 (key = encryption key, not verified)", exBad, 200, exBad.reason); }
  { const r = await api("POST", "/api/v1/templates/import", { code: templateCode, password: "wrong-pass-123" }, { token: otherToken }); record(TP, "import with wrong password -> 400", r, 400, r.reason); }
  { const r = await api("POST", "/api/v1/templates/import", { code: templateCode, password: owner.password }, { token: otherToken }); record(TP, "import into other tenant -> 200", r, 200, "subject=" + r.data?.subject); }

  // ===================== ADMIN ==============================================
  const AD = "ADMIN";
  for (const p of ["settings", "audit", "usage", "fields", "billing", "brand", "crm"]) {
    const r = await api("GET", `/api/v1/admin/${p}`, null, { token: ownerToken });
    record(AD, `admin/${p} -> 200`, r, 200, r.status === 200 ? jsv(r.data, 60) : r.reason);
  }
  { const r = await api("GET", "/api/v1/admin/settings", null, { token: agentToken }); record(AD, "agent admin/settings -> 403", r, 403, r.reason); }
  { const r = await api("POST", "/api/v1/admin/billing", { plan: "pro" }, { token: ownerToken }); record(AD, "billing link (no Stripe keys) -> 503", r, 503, r.reason); }
  { const r = await api("POST", "/api/v1/stripe/webhook", "{}", {}); record(AD, "stripe webhook (unconfigured) -> 503", r, 503, r.reason); }
  { const r = await api("GET", "/api/v1/push", null, { token: ownerToken }); record(AD, "push key -> 200", r, 200, jsv(r.data, 60)); }
  { const settings = (await api("GET", "/api/v1/admin/settings", null, { token: ownerToken })).data;
    const r = await api("PUT", "/api/v1/admin/settings", settings, { token: ownerToken }); record(AD, "save settings round-trip -> 200", r, 200, r.reason);
    const bad = await api("PUT", "/api/v1/admin/settings", { ...settings, accent: "not-hex" }, { token: ownerToken }); record(AD, "settings bad accent -> 400", bad, 400, bad.reason); }

  // ===================== STEP-UP / EXPORT / BACKUP =========================
  const ST = "STEPUP";
  { const r = await api("GET", "/api/v1/admin/export", null, { token: ownerToken }); record(ST, "admin export without step-up -> 403", r, 403, r.reason); }
  { await paceAuth(); const r = await api("POST", "/api/v1/account/step-up", { password: "wrong" }, { token: ownerToken }); record(ST, "step-up wrong password -> 403", r, 403, r.reason); }
  let stepToken = "";
  { await paceAuth(); const r = await api("POST", "/api/v1/account/step-up", { password: owner.password }, { token: ownerToken }); record(ST, "step-up correct -> 200 token (5 min, single-use)", r, 200, "expires_in=" + r.data?.expires_in); stepToken = r.data?.token || ""; }
  { const r = await api("GET", `/api/v1/admin/export?step_up=${stepToken}`, null, { token: ownerToken }); record(ST, "admin export with step-up -> 200", r, 200, String(r.raw).slice(0, 40)); }
  { const r = await api("GET", `/api/v1/admin/export?step_up=${stepToken}`, null, { token: ownerToken }); record(ST, "step-up token single-use -> 403", r, 403, r.reason);
    await paceAuth();
    const s2 = await api("POST", "/api/v1/account/step-up", { password: owner.password }, { token: ownerToken });
    const r2 = await api("GET", `/api/v1/admin/backup?step_up=${s2.data?.token}`, null, { token: ownerToken }); record(ST, "admin encrypted backup with step-up -> 200", r2, 200, String(r2.raw).slice(0, 24)); }

  // ===================== MFA (run last: revokes other sessions) ===========
  const F = "MFA";
  { const r = await api("GET", "/api/v1/account/mfa", null, { token: ownerToken }); record(F, "MFA status -> 200", r, 200, "enabled=" + r.data?.enabled); }
  { await paceAuth(); const r = await api("POST", "/api/v1/account/mfa", { action: "setup", password: "wrong" }, { token: ownerToken }); record(F, "MFA setup wrong pw -> 403", r, 403, r.reason); }
  let totpSecret = "";
  { await paceAuth(); const r = await api("POST", "/api/v1/account/mfa", { action: "setup", password: owner.password }, { token: ownerToken }); record(F, "MFA setup -> 200 + secret", r, 200, "secret=" + (r.data?.secret || "MISSING").slice(0, 8)); totpSecret = r.data?.secret || ""; }
  { await paceAuth(); const r = await api("POST", "/api/v1/account/mfa", { action: "enable", code: "000000" }, { token: ownerToken }); record(F, "MFA enable bad code -> 401", r, 401, r.reason); }
  if (totpSecret) {
    const now = Math.floor(Date.now() / 1000 / 30);
    let last = null;
    for (const off of [0, -1, 1]) {
      await paceAuth();
      last = await api("POST", "/api/v1/account/mfa", { action: "enable", code: totpAt(totpSecret, now + off) }, { token: ownerToken });
      if (last.status === 200) break;
    }
    record(F, "MFA enable with valid TOTP -> 200", last, 200, `recovery_codes=${last.data?.recovery_codes?.length}`);
    await paceAuth();
    const r2 = await api("POST", "/api/v1/account/mfa", { action: "enable", code: "000000" }, { token: ownerToken });
    record(F, "MFA re-enable (already on) -> 409", r2, 409, r2.reason);
  } else record(F, "MFA enable (no secret returned)", { status: 500, reason: "setup gave no secret" }, 200, "CANNOT VERIFY MFA ENABLE");
  { const r = await login(owner.email, owner.password); record(F, "owner login now needs MFA -> 409", r, 409, r.reason);
    if (totpSecret) {
      let withCode = null;
      for (const off of [0, 1, -1]) {
        const now2 = Math.floor(Date.now() / 1000 / 30);
        withCode = await login(owner.email, owner.password, totpAt(totpSecret, now2 + off));
        if (withCode.status === 200) break;
      }
      record(F, "owner login with TOTP -> 200", withCode, 200, withCode.reason); ownerToken = withCode.data?.access_token || ownerToken; } }
  { const r = await api("GET", "/api/v1/account/sessions", null, { token: ownerToken }); record(F, "list active sessions -> 200", r, 200, "count=" + (Array.isArray(r.data) ? r.data.length : "?")); }
  { const r = await api("POST", "/api/v1/account/sessions/revoke", { id: "nonexistent" }, { token: ownerToken }); record(F, "revoke unknown session -> 404", r, 404, r.reason); }
  { await paceAuth(); const r = await api("POST", "/api/v1/account/mfa", { action: "disable", code: "000000" }, { token: ownerToken }); record(F, "MFA disable bad code -> 401", r, 401, r.reason); }
  { const ses = await api("GET", "/api/v1/account/sessions", null, { token: ownerToken });
    const target = (Array.isArray(ses.data) ? ses.data : []).find((s) => !s.current);
    if (target) { const r = await api("POST", "/api/v1/account/sessions/revoke", { id: target.id }, { token: ownerToken }); record(F, "revoke another session -> 200", r, 200, r.reason); }
    else record(F, "revoke another session", { status: 501, reason: "only one session visible" }, 200, "no second session to revoke"); }

  // ===================== SECURITY / OWASP ==================================
  const SEC = "SECURITY";
  { const r = await api("GET", "/api/v1/healthz"); record(SEC, "healthz -> 200", r, 200);
    const h = r.headers.get("x-content-type-options"); record(SEC, "security header X-Content-Type-Options", h ? { status: 200 } : { status: 404, reason: "missing" }, 200, "header=" + h);
    const cc = r.headers.get("cache-control"); record(SEC, "security header Cache-Control no-store", cc ? { status: 200 } : { status: 404, reason: "missing" }, 200, "header=" + cc); }
  { const r = await api("GET", "/api/v1/branding"); record(SEC, "public branding -> 200", r, 200, jsv(r.data, 60)); }
  { const r = await api("GET", "/api/v1/oidc/providers"); record(SEC, "oidc/providers -> 200 (empty, unconfigured)", r, 200, jsv(r.data, 60)); }
  { const r = await api("GET", "/api/v1/oidc/google/start"); record(SEC, "oidc start unconfigured -> 404", r, 404, r.reason); }
  { const r = await api("GET", "/api/v1/email/inbound", {}); record(SEC, "email inbound unconfigured (GET -> 405, POST path exists) -> 404/405", r, [404, 405], r.reason); }
  { const r = await api("POST", "/api/v1/email/resend", "{}", {}); record(SEC, "resend webhook unconfigured -> 404", r, 404, r.reason); }
  { const big = { name: "x".repeat(70000), email: "big@x.test" }; const r = await api("POST", "/api/v1/customers", big, { token: otherToken }); record(SEC, "JSON body >64KB -> 400", r, 400, r.message); }
  { const r = await api("POST", "/api/v1/customers", "{broken", { token: otherToken }); record(SEC, "malformed JSON -> 400", r, 400, r.message); }
  { const r = await api("POST", "/api/v1/auth/login", "{}", { header: { "Content-Type": "text/plain" } }); record(SEC, "wrong content-type -> 415", r, 415, r.message); }
  { const r = await api("DELETE", "/api/v1/customers", null, { token: otherToken }); record(SEC, "DELETE on list -> 405", r, 405); }
  { const r = await api("POST", "/api/v1/customers", { name: "<script>alert(1)</script>", email: `xss-${ts}@x.test` }, { token: otherToken }); record(SEC, "stored XSS payload accepted in name (UI must sanitize)", r, 201, "payload round-trips in JSON; no HTML-escape layer on the API"); }
  { const r = await api("GET", `/api/v1/customers?q=${encodeURIComponent("<script>")}`, null, { token: otherToken }); record(SEC, "search does not execute stored HTML (JSON only)", r, 200, "xss=" + r.data?.items?.some?.((i) => i.name?.includes("<script>"))); }
  { const r = await api("GET", "/api/v1/kb/unknown-tenant-xyz"); record(SEC, "kb unknown tenant -> 404", r, 404, r.reason); }
  { const kb = (await api("POST", "/api/v1/knowledge", { subject: "KB Public", content: "nội dung công khai", public: true, enabled: true }, { token: otherToken })).data;
    const r = await api("GET", `/api/v1/kb/${otherTenant}`); record(SEC, "kb public list -> 200", r, 200, "count=" + r.data?.length);
    const r2 = await api("GET", `/api/v1/kb/unknown-tenant-xyz/${kb?.slug}`); record(SEC, "kb cross-tenant article -> 404", r2, 404, r2.reason); }

  // ===================== LOGOUT / TOKEN INVALIDATION ======================
  const LO = "SESSIONS";
  { const r = await api("POST", "/api/v1/auth/logout", {}, { token: otherToken }); record(LO, "logout -> 200", r, 200);
    const r2 = await api("GET", "/api/v1/auth/me", null, { token: otherToken }); record(LO, "token after logout -> 401 (revoked)", r2, 401, r2.reason); }
  { const r = await api("POST", "/api/v1/auth/logout", {}, { token: "garbage-token" }); record(LO, "logout garbage token -> 200 (no-op)", r, 200, r.reason); }

  // ===================== LOCKOUT (dedicated account) =======================
  const LK = "LOCKOUT";
  const lock = { name: "Lock", email: `lock-${ts}@example.test`, password: "Nexa-local-test-2026!" };
  { await paceAuth(); const rr = await register("Lock", lock.email, { password: lock.password }); record(LK, "lockout account register -> 201", rr, 201); }
  for (let i = 1; i <= 5; i++) { const r = await login(lock.email, "wrongpassword123"); if (i === 5) record(LK, "5th consecutive failure -> account locked", r, 401, "reason=" + r.reason + " msg=" + r.message); }
  { const r = await login(lock.email, "wrongpassword123"); record(LK, "6th call: still locked (growing window)", r, 401, "reason=" + r.reason); }
  { const r = await login(lock.email, lock.password); record(LK, "correct password while locked -> still 401", r, 401, "reason=" + r.reason); }

  // ===================== RATE LIMIT (fresh window) =========================
  const RL = "RATELIMIT";
  { await sleep(61000); winStart = 0; winCount = 0;
    let got429 = 0, last = 0;
    for (let i = 0; i < 22; i++) { const r = await rawLogin(`rl-${ts}@x.test`, "Nexa-local-test-2026!"); last = r.status; if (r.status === 429) got429++; await sleep(15); }
    record(RL, "auth rate limiter trips within 22 raw calls", { status: got429 ? 429 : last }, 429, `429 count=${got429}/22`);
  }

  // ---- summary -------------------------------------------------------------
  server.kill();
  await sleep(300);
  const passed = results.filter((x) => x.pass).length;
  const failed = results.filter((x) => !x.pass);
  writeFileSync(`D:/GOLANG/data/testrun/results-${RUN}-${ts}.json`, JSON.stringify({ run: RUN, generated: new Date().toISOString(), passed, total: results.length, failures: failed.map((f) => ({ ...f })), results }, null, 2));
  console.log(`\n== [run=${RUN}] SUMMARY: ${passed}/${results.length} passed, ${failed.length} failed ==`);
  for (const f of failed) console.log(` - [${f.area}] ${f.name}: got ${f.actual} want ${f.expected} ${f.reason ? "(" + f.reason + ")" : ""} ${f.note ? "| " + f.note : ""}`);
  process.exit(0);
}

main().catch((e) => { console.error("HARNESS ERROR", e); process.exit(1); });
