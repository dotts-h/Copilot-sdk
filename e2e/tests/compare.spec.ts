import { test, expect } from "./fixtures";
import { gotoApp } from "./helpers";

// Run comparison (O4, issue 0094): a keyed side-by-side diff of two runs of ONE
// workflow, reached from a "compare with" picker on a run's detail page. The demo
// seeds run-demo-1 and run-demo-3 as two runs of build-and-harden (each with an event
// log) so the comparison renders per-field deltas AND both final outputs; run-demo-2
// is a different workflow, so it is never offered as a candidate.

test("a run's detail page offers a compare-with picker for its workflow's other runs", async ({ page }) => {
  await gotoApp(page);
  await page.evaluate(() =>
    (window as any).htmx.ajax("GET", "/page/runs/run-demo-1", { target: "#main", swap: "innerHTML" }),
  );
  await expect(page.locator(".inspector-summary .run-name")).toHaveText("Build & harden");

  // The picker lists the rerun (run-demo-3), the other build-and-harden run.
  const candidate = page.locator(".compare-picker .compare-with").first();
  await expect(candidate).toBeVisible();
  await candidate.click();

  // The comparison renders: a summary table with B−A deltas and a per-lane table.
  await expect(page.locator("#main .inspector.compare")).toBeVisible();
  await expect(page.locator(".compare-summary")).toBeVisible();
  await expect(page.locator(".compare-lanes")).toBeVisible();
  // Two lanes aligned by step.
  await expect(page.locator(".compare-lanes .compare-lane-row")).toHaveCount(2);
});

test("comparing two runs of one workflow renders both final outputs side-by-side", async ({ page }) => {
  await gotoApp(page);
  // Navigate straight to the comparison of run-demo-1 (A) vs run-demo-3 (B).
  await page.evaluate(() =>
    (window as any).htmx.ajax("GET", "/runs/compare?a=run-demo-1&b=run-demo-3", { target: "#main", swap: "innerHTML" }),
  );

  await expect(page.locator("#main .inspector.compare h2")).toContainText("Compare runs");
  // Both runs' final assistant outputs render (both event logs present).
  const outputs = page.locator(".compare-outputs .compare-output");
  await expect(outputs).toHaveCount(2);
  await expect(page.locator(".compare-outputs")).toContainText("Tests green; the feature is hardened.");
  await expect(page.locator(".compare-outputs")).toContainText("hardened and a bit cheaper");
});
