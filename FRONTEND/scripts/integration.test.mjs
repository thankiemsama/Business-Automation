import assert from "node:assert/strict";
import { test } from "node:test";

// Run only against an isolated backend store. This suite creates test tenants.
const base = process.env.TEST_FRONTEND_URL || "http://127.0.0.1:3001";
const origin = new URL(base).origin;
const password = "Nexa-local-test-2026!";
const suffix = Date.now();
let cookie = "";
async function api(path, method = "GET", body, options = {}) {
  const response = await fetch(`${base}/api/platform/${path}`, {
    method,
    headers: {
      "Content-Type": "application/json",
      Origin: origin,
      Cookie: cookie,
      ...options.headers,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const envelope = await response.json();
  return { response, data: envelope.data, message: envelope.message };
}
async function success(path, method, body) {
  const result = await api(path, method, body);
  assert.ok(result.response.ok, `${path}: ${result.message}`);
  return result.data;
}
test("BFF: session, CRUD, knowledge reply, automation, roles, isolation and logout", async (t) => {
  let customer, conversation, ticket, knowledge, automation;
  const owner = {
    name: "Nexa QA",
    email: `owner-${suffix}@example.test`,
    password,
  };
  await t.test(
    "unauthenticated and cross-origin requests are rejected",
    async () => {
      assert.equal((await api("dashboard")).response.status, 401);
      assert.equal(
        (
          await api("auth/register", "POST", owner, {
            headers: { Origin: "https://other.example" },
          })
        ).response.status,
        403,
      );
      assert.equal((await api("unexpected")).response.status, 404);
    },
  );
  await t.test(
    "register then login keeps token in HttpOnly cookie",
    async () => {
      await success("auth/register", "POST", owner);
      const login = await api("auth/login", "POST", {
        email: owner.email,
        password,
      });
      assert.equal(login.response.status, 200);
      assert.deepEqual(login.data, { authenticated: true });
      const header = login.response.headers.get("set-cookie");
      assert.match(header, /HttpOnly/i);
      assert.match(header, /SameSite=lax/i);
      cookie = header.split(";")[0];
      assert.equal((await success("auth/me")).role, "owner");
    },
  );
  await t.test("creates domain records and uses exact PUT DTOs", async () => {
    customer = await success("customers", "POST", {
      name: "Khách hàng kiểm thử",
      email: "customer@example.test",
      phone: "0900000000",
    });
    const update = await success(`customers/${customer.id}`, "PUT", {
      version: customer.version,
      name: "Khách hàng đã cập nhật",
      email: "updated@example.test",
      phone: "0911111111",
    });
    assert.equal(update.email, "updated@example.test");
    assert.equal(update.version, customer.version + 1);
    const fixture = { name: "Thay đổi từ bản cũ" };
    assert.equal(
      (await api(`customers/${customer.id}`, "PUT", fixture)).response.status,
      428,
    );
    assert.equal(
      (
        await api(`customers/${customer.id}`, "PUT", {
          ...fixture,
          version: customer.version,
        })
      ).response.status,
      409,
    );
    assert.equal((await success(`customers/${customer.id}`)).name, update.name);
    knowledge = await success("knowledge", "POST", {
      subject: "Chính sách hoàn tiền",
      content: "Hoàn tiền trong 7 ngày sau khi xác nhận.",
      enabled: true,
    });
    automation = await success("automations", "POST", {
      name: "Hỗ trợ hoàn tiền",
      keywords: ["hoàn tiền"],
      enabled: true,
    });
    conversation = await success("conversations", "POST", {
      customer_id: customer.id,
      subject: "Cần hoàn tiền",
      channel: "web",
      status: "open",
    });
    await success(`conversations/${conversation.id}/messages`, "POST", {
      content: "Tôi cần hoàn tiền",
      sender: "customer",
    });
  });
  await t.test(
    "customer message triggers ticket and knowledge reply records source",
    async () => {
      const tickets = await success("tickets");
      assert.equal(tickets.total, 1);
      ticket = tickets.items[0];
      assert.equal(ticket.automation_id, automation.id);
      const reply = await success(
        `conversations/${conversation.id}/reply`,
        "POST",
      );
      assert.equal(reply.source_id, knowledge.id);
      assert.equal(reply.needs_human, false);
      assert.equal(reply.message.sender, "assistant");
      const messages = await success(
        `conversations/${conversation.id}/messages?page=2&limit=1`,
      );
      assert.equal(messages.total, 2);
      assert.equal(messages.items[0].sender, "assistant");
      assert.equal(
        (await api(`conversations/${conversation.id}/reply`, "POST")).response
          .status,
        409,
      );
    },
  );
  await t.test(
    "search, pagination, ticket resolution, disabling and conversation closure",
    async () => {
      assert.equal(
        (await success("customers?q=updated&page=1&limit=1")).total,
        1,
      );
      assert.equal((await success("customers?q=not-found")).total, 0);
      await success(`tickets/${ticket.id}`, "PUT", {
        version: ticket.version,
        subject: ticket.subject,
        content: "Đã hỗ trợ",
        customer_id: customer.id,
        conversation_id: conversation.id,
        status: "resolved",
        priority: "normal",
      });
      await success(`knowledge/${knowledge.id}`, "PUT", {
        version: knowledge.version,
        subject: knowledge.subject,
        content: knowledge.content,
        enabled: false,
      });
      await success(`automations/${automation.id}`, "PUT", {
        version: automation.version,
        name: automation.name,
        keywords: automation.keywords,
        enabled: false,
      });
      const latest = await success(`conversations/${conversation.id}`);
      await success(`conversations/${conversation.id}`, "PUT", {
        version: latest.version,
        customer_id: customer.id,
        subject: conversation.subject,
        channel: "web",
        status: "closed",
      });
      assert.equal(
        (
          await api(`conversations/${conversation.id}/messages`, "POST", {
            sender: "agent",
            content: "test",
          })
        ).response.status,
        409,
      );
      const dashboard = await success("dashboard");
      assert.equal(dashboard.open_tickets, 0);
      assert.equal(dashboard.customers, 1);
    },
  );
  await t.test("agent can read but cannot modify owner resources", async () => {
    const agent = {
      name: "QA Agent",
      email: `agent-${suffix}@example.test`,
      password,
    };
    await success("team", "POST", agent);
    const ownerCookie = cookie;
    const login = await api("auth/login", "POST", {
      email: agent.email,
      password,
    });
    cookie = login.response.headers.get("set-cookie").split(";")[0];
    assert.equal((await success("auth/me")).role, "agent");
    assert.equal((await success("knowledge")).total, 1);
    assert.equal(
      (
        await api("knowledge", "POST", {
          subject: "Denied",
          content: "Denied",
          enabled: true,
        })
      ).response.status,
      403,
    );
    assert.equal((await api("team", "POST", agent)).response.status, 403);
    await success("auth/logout", "POST");
    cookie = ownerCookie;
  });
  await t.test(
    "separate tenants cannot read each other's records; logout revokes session",
    async () => {
      const other = {
        name: "Other tenant",
        email: `other-${suffix}@example.test`,
        password,
      };
      await success("auth/register", "POST", other);
      const ownerCookie = cookie;
      const login = await api("auth/login", "POST", {
        email: other.email,
        password,
      });
      cookie = login.response.headers.get("set-cookie").split(";")[0];
      assert.equal((await success("customers")).total, 0);
      assert.equal(
        (await api(`customers/${customer.id}`)).response.status,
        404,
      );
      await success("auth/logout", "POST");
      cookie = ownerCookie;
      await success("auth/logout", "POST");
      assert.equal((await api("auth/me")).response.status, 401);
    },
  );
});
