import { test, expect } from "@playwright/test";
import { gotoApp, navTo } from "./helpers";

// Motion & polish (V24, ADR-0028). Page navigation opts into htmx's same-document
// View Transitions (a cross-fade of #main), interactive controls ease their
// hover/focus states instead of snapping, and BOTH are fully neutralized under
// prefers-reduced-motion. The opt-in is PER-NAVIGATION (`transition:true` on the
// nav links), not global: globalViewTransitions would also wrap the streaming SSE
// deltas and the hx-swap-oob updates (timeline/status/lanes), colliding with the
// constant stream and dropping swaps. View Transitions are progressive
// enhancement, so every assertion anchors on the settled end state, never on
// animation timing.

test("page navigation opts into View Transitions, scoped to #main", async ({ page }) => {
  await gotoApp(page);

  // Each nav link carries the per-swap opt-in, so a sidebar click cross-fades the
  // page (the ⌘K palette routes through these same links, inheriting it).
  const navSwap = await page
    .locator(".nav a").first()
    .getAttribute("hx-swap");
  expect(navSwap).toContain("transition:true");

  // #main carries its own transition name, so only the main panel cross-fades —
  // the sidebar shell stays put.
  const name = await page
    .locator("#main")
    .evaluate((el) => getComputedStyle(el).viewTransitionName);
  expect(name).toBe("main");
});

test("streaming swaps do NOT opt into the transition", async ({ page }) => {
  await gotoApp(page);
  // The high-frequency swaps must stay plain (no `transition:`), so a per-token
  // delta and the live cost/lanes/status updates never wrap the shell in a
  // transition — the failure mode that a global opt-in would reintroduce.
  const streaming = await page.evaluate(() => {
    const get = (s: string) => document.querySelector(s)?.getAttribute("hx-swap") || "";
    return {
      delta: get('[sse-swap="delta"]'),
      lanes: get('[sse-swap="lanes"]'),
      status: get('[sse-swap="status"]'),
    };
  });
  expect(streaming.delta).not.toContain("transition");
  expect(streaming.lanes).not.toContain("transition");
  expect(streaming.status).not.toContain("transition");
});

test("navigation still settles under prefers-reduced-motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await gotoApp(page);
  // The transition must degrade to an instant swap, never hang the navigation.
  await navTo(page, "Telemetry");
  await expect(page.locator("#main h2")).toContainText(/Telemetry/);
  await navTo(page, "Runs");
  await expect(page.locator("#main h2")).toContainText(/Runs/);
});

test("interactive controls ease their state, and reduced-motion zeroes it", async ({ page }) => {
  await gotoApp(page);
  await navTo(page, "Telemetry");
  // The component polish pass gives interactive controls a non-zero transition so
  // hover/focus eases in (the same motion language as the page cross-fade).
  const dur = await page
    .locator(".window").first()
    .evaluate((el) => getComputedStyle(el).transitionDuration);
  expect(parseFloat(dur)).toBeGreaterThan(0);

  // Under reduced motion the global reset (ADR-0025) collapses it to ~0 — motion
  // is opt-out, an a11y requirement, not a nicety.
  await page.emulateMedia({ reducedMotion: "reduce" });
  const reduced = await page
    .locator(".window").first()
    .evaluate((el) => getComputedStyle(el).transitionDuration);
  expect(parseFloat(reduced)).toBeLessThan(0.05);
});
