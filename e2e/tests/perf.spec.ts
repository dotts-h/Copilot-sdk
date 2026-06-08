import { test, expect } from "./fixtures";
import { sel, gotoApp, navTo, send } from "./helpers";

// Performance budgets. These are deliberately generous (the demo binary is a
// local in-memory server) so they fail only on a real regression — a runaway
// render, a blocking handler, or a stalled stream — not on noise.

const BUDGET = {
  domContentLoadedMs: 1500,
  loadCompleteMs: 2500,
  firstPaintMs: 1500,
  pageSwapMs: 800, // htmx partial swap should feel instant
  sseFirstByteMs: 1000, // /events must greet promptly
  sendAckMs: 1000, // POST /send OOB echo
};

test("initial shell load is within the navigation-timing budget", async ({ page }) => {
  await page.goto("/", { waitUntil: "load" });
  const timing = await page.evaluate(() => {
    const [nav] = performance.getEntriesByType("navigation") as PerformanceNavigationTiming[];
    const fp = performance.getEntriesByType("paint").find((p) => p.name === "first-paint");
    return {
      domContentLoaded: nav.domContentLoadedEventEnd - nav.startTime,
      load: nav.loadEventEnd - nav.startTime,
      firstPaint: fp ? fp.startTime : -1,
    };
  });
  expect(timing.domContentLoaded).toBeLessThan(BUDGET.domContentLoadedMs);
  expect(timing.load).toBeLessThan(BUDGET.loadCompleteMs);
  if (timing.firstPaint >= 0) expect(timing.firstPaint).toBeLessThan(BUDGET.firstPaintMs);
});

test("htmx page swaps are well under the interactivity budget", async ({ page }) => {
  await gotoApp(page);
  for (const label of ["Telemetry", "Skills", "Settings", "Chat"]) {
    const start = Date.now();
    await navTo(page, label);
    await expect(page.locator("#main").first()).toBeVisible();
    const elapsed = Date.now() - start;
    expect(elapsed, `swap to ${label} took ${elapsed}ms`).toBeLessThan(BUDGET.pageSwapMs);
  }
});

test("the SSE stream sends its first byte promptly", async ({ appURL }) => {
  const start = Date.now();
  const ctrl = new AbortController();
  const res = await fetch(`${appURL}/events`, { signal: ctrl.signal });
  const reader = res.body!.getReader();
  await reader.read(); // first chunk (the 'ready' greeting)
  const elapsed = Date.now() - start;
  ctrl.abort();
  await reader.cancel().catch(() => {});
  expect(elapsed, `SSE first byte took ${elapsed}ms`).toBeLessThan(BUDGET.sseFirstByteMs);
});

test("POST /send acknowledges (OOB echo) promptly", async ({ page }) => {
  await gotoApp(page);
  const start = Date.now();
  await send(page, "latency check");
  const elapsed = Date.now() - start;
  expect(elapsed, `send ack took ${elapsed}ms`).toBeLessThan(BUDGET.sendAckMs);
});

test("the timeline does not leak unbounded DOM nodes across many turns", async ({ page }) => {
  await gotoApp(page);
  // Fire several quick turns; the demo coalesces them, but the DOM should grow
  // roughly linearly with content, not explode (a sanity bound, not a tight one).
  for (let i = 0; i < 3; i++) await send(page, `turn ${i}`);
  const nodes = await page.evaluate(() => document.querySelectorAll(`${"#timeline"} *`).length);
  expect(nodes, `timeline node count = ${nodes}`).toBeLessThan(2000);
});
