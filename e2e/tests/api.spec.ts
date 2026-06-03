import { test, expect, request } from "@playwright/test";
import { BASE_URL } from "../playwright.config";

// HTTP-level API contract, exercised through Playwright's request context (no
// browser). Complements the in-process Go api_test.go by asserting the same
// contract over a real socket against the running demo binary.

test.describe("page & asset contract", () => {
  const htmlPages = ["chat", "telemetry", "skills", "instructions", "agents", "models", "settings", "help"];

  for (const slug of htmlPages) {
    test(`GET /page/${slug} → 200 text/html`, async ({ request }) => {
      const res = await request.get(`/page/${slug}`);
      expect(res.status()).toBe(200);
      expect(res.headers()["content-type"]).toContain("text/html");
      expect((await res.text()).trim().length).toBeGreaterThan(0);
    });
  }

  test("GET / sets a hardened session cookie", async ({ request }) => {
    const res = await request.get("/");
    expect(res.status()).toBe(200);
    const setCookie = res.headers()["set-cookie"] ?? "";
    expect(setCookie).toMatch(/mo_sid=/);
    expect(setCookie).toMatch(/HttpOnly/i);
    expect(setCookie).toMatch(/SameSite=Lax/i);
  });

  test("GET /static/htmx.min.js → 200 javascript", async ({ request }) => {
    const res = await request.get("/static/htmx.min.js");
    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toMatch(/javascript/);
  });

  test("GET /static/missing.js → 404", async ({ request }) => {
    const res = await request.get("/static/definitely-missing.js");
    expect(res.status()).toBe(404);
  });

  test("an unknown page slug degrades to the chat composer (200)", async ({ request }) => {
    const res = await request.get("/page/not-a-real-page");
    expect(res.status()).toBe(200);
    expect(await res.text()).toContain('id="composer"');
  });
});

test.describe("send contract", () => {
  test("empty prompt → 204 No Content", async ({ request }) => {
    const res = await request.post("/send", { form: { prompt: "   " } });
    expect(res.status()).toBe(204);
  });

  test("a prompt is echoed back in an OOB timeline refresh", async ({ request }) => {
    const res = await request.post("/send", { form: { prompt: "api-contract-prompt" } });
    expect(res.status()).toBe(200);
    const body = await res.text();
    expect(body).toContain('hx-swap-oob="innerHTML"');
    expect(body).toContain("api-contract-prompt");
  });

  test("an injection payload is reflected escaped, never as live markup", async ({ request }) => {
    const res = await request.post("/send", { form: { prompt: "<script>alert('xss')</script>" } });
    const body = await res.text();
    expect(body).not.toContain("<script>alert");
    expect(body).toContain("&lt;script&gt;");
  });
});

test.describe("bridge routes tolerate unknown ids", () => {
  const cases = [
    { path: "/perm/ghost", form: { approve: "1" } },
    { path: "/ask/ghost", form: { answer: "hi" } },
    { path: "/plan/ghost", form: { action: "proceed" } },
    { path: "/elicit/ghost", form: { action: "accept" } },
  ];
  for (const c of cases) {
    test(`POST ${c.path} (unknown id) → 200 OOB timeline`, async ({ request }) => {
      const res = await request.post(c.path, { form: c.form });
      expect(res.status()).toBe(200);
      expect(await res.text()).toContain('id="timeline"');
    });
  }
});

test.describe("SSE transport", () => {
  test("GET /events streams text/event-stream and greets with 'ready'", async () => {
    // A raw fetch with a hard timeout: the stream never closes, so we read just
    // enough of the first chunk to see the greeting, then abort.
    const ctrl = new AbortController();
    const res = await fetch(`${BASE_URL}/events`, { signal: ctrl.signal });
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("text/event-stream");
    expect(res.headers.get("cache-control")).toContain("no-cache");

    const reader = res.body!.getReader();
    const { value } = await reader.read();
    const chunk = new TextDecoder().decode(value);
    expect(chunk).toContain("event: ready");
    ctrl.abort();
    await reader.cancel().catch(() => {});
  });
});

// A standalone request context proves the contract holds without any cookie jar
// shared from the browser fixtures.
test("cold request context can drive the API", async () => {
  const ctx = await request.newContext({ baseURL: BASE_URL });
  const res = await ctx.get("/page/telemetry");
  expect(res.status()).toBe(200);
  expect(await res.text()).toContain("Telemetry");
  await ctx.dispose();
});
