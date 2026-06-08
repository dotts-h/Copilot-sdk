import { test, expect } from "./fixtures";
import { gotoApp, navTo } from "./helpers";

// Telemetry KPI dashboard (V23, ADR-0027): a row of big-number cards each with a
// period-over-period delta + a sparkline, plus the cumulative trend band and the
// spend-vs-budget bullet. Everything is server-rendered inline SVG, zero JS — the
// charts re-render through the existing ?window= htmx swap. The offline demo
// self-seeds spend across the current and prior windows, so the deltas and charts
// have real data offline. The both-theme axe scan (a11y.spec.ts) covers the SVGs.

test.beforeEach(async ({ page }) => {
  await gotoApp(page);
  await navTo(page, "Telemetry");
  await expect(page.locator("#main h2")).toContainText("Telemetry");
});

test("the KPI cards render with a numeric value and a delta badge", async ({ page }) => {
  const cards = page.locator(".kpi .kpi-card");
  await expect(cards).toHaveCount(4);

  // Each card has a non-empty big-number value and a delta badge.
  for (let i = 0; i < 4; i++) {
    const card = cards.nth(i);
    await expect(card.locator(".kpi-value")).not.toBeEmpty();
    await expect(card.locator(".kpi-delta")).toBeVisible();
  }

  // The labelled cards are present.
  await expect(page.locator(".kpi-label", { hasText: /^Total spend$/ })).toBeVisible();
  await expect(page.locator(".kpi-label", { hasText: /^Burn rate$/ })).toBeVisible();

  // At least one delta carries a real direction class (▲ up / ▼ down) — the demo
  // seeds a prior-window baseline, so a metric moved rather than reading "new".
  const directional = page.locator(".kpi-delta.up, .kpi-delta.down");
  expect(await directional.count()).toBeGreaterThan(0);
});

test("each KPI card has a sparkline svg with an accessible name", async ({ page }) => {
  const sparks = page.locator(".kpi-card svg.spark");
  await expect(sparks).toHaveCount(4);
  for (let i = 0; i < 4; i++) {
    const spark = sparks.nth(i);
    await expect(spark).toHaveAttribute("role", "img");
    // role=img needs an accessible name for axe (svg-img-alt): a <title> + aria-label.
    const label = await spark.getAttribute("aria-label");
    expect(label && label.length).toBeTruthy();
    await expect(spark.locator("title")).toHaveCount(1);
    await expect(spark.locator("polyline")).toHaveCount(1);
  }
});

test("the trend band and spend-vs-budget bullet svgs render with accessible names", async ({ page }) => {
  const band = page.locator("svg.band");
  await expect(band).toHaveCount(1);
  await expect(band).toHaveAttribute("role", "img");
  await expect(band.locator("title")).toHaveCount(1);
  // The band has the cumulative actuals area and the dashed forecast continuation.
  await expect(band.locator("path.band-area")).toHaveCount(1);
  await expect(band.locator("polyline.band-forecast")).toHaveCount(1);

  // The demo's default config sets a monthly budget, so the bullet renders.
  const bullet = page.locator("svg.bullet");
  await expect(bullet).toHaveCount(1);
  await expect(bullet).toHaveAttribute("role", "img");
  await expect(bullet.locator("title")).toHaveCount(1);
  await expect(bullet.locator("rect.bullet-track")).toHaveCount(1);
  await expect(bullet.locator("rect.bullet-target")).toHaveCount(1);
});

test("re-selecting a window re-renders the dashboard server-side (no JS)", async ({ page }) => {
  // The KPI cards + charts are present at the default window.
  await expect(page.locator(".kpi .kpi-card")).toHaveCount(4);
  await expect(page.locator("svg.band")).toHaveCount(1);

  // Click the 90-day window button — the whole partial (including the dashboard)
  // re-renders via the existing hx-get; the cards/charts come back, no new route.
  await page.locator(".window-row button.window", { hasText: "90d" }).click();
  await expect(page.locator(".window.on", { hasText: "90d" })).toBeVisible();

  // The sparklines now span the wider window — still 4 cards, band + bullet intact.
  await expect(page.locator(".kpi .kpi-card")).toHaveCount(4);
  await expect(page.locator(".kpi-card svg.spark")).toHaveCount(4);
  await expect(page.locator("svg.band")).toHaveCount(1);
  await expect(page.locator("svg.bullet")).toHaveCount(1);
});
