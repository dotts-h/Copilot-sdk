import { test, expect } from "./fixtures";
import { sel, gotoApp } from "./helpers";

// Run inspector (ADR-0052, issue 0091): a read-only step-timeline detail page
// reached from a Runs-page row. The demo seeds run-demo-1's event log (so its
// detail page renders a real lane-grouped timeline) and leaves run-demo-2 without
// one (so its detail page exercises the "no event log" degradation). Zero new JS —
// the disclosure is a native <details>.

test("a run row links to its step-timeline detail page", async ({ page }) => {
  await gotoApp(page);
  await page.locator(sel.nav, { hasText: /^Runs$/ }).click();
  await expect(page.locator("#main h2")).toContainText(/Runs/);

  // The first (most recent) run row's name links into the inspector.
  const link = page.locator(".run-record .run-name-link").first();
  await expect(link).toBeVisible();
  await link.click();

  // The detail page renders the inspector with a summary card and the timeline.
  await expect(page.locator("#main .inspector")).toBeVisible();
  await expect(page.locator(".inspector-summary .run-name")).toHaveText("Review & fix");
  await expect(page.locator(".inspector .back")).toBeVisible();
});

test("the seeded demo run renders a lane-grouped step timeline with disclosures", async ({ page }) => {
  // run-demo-1 ("Build & harden") has a seeded event log — navigate to it directly.
  await gotoApp(page);
  await page.evaluate(() =>
    (window as any).htmx.ajax("GET", "/page/runs/run-demo-1", { target: "#main", swap: "innerHTML" }),
  );

  await expect(page.locator(".inspector-summary .run-name")).toHaveText("Build & harden");

  // Two lanes (builder + sdet), each a labelled group of steps.
  const lanes = page.locator(".inspector-lane");
  await expect(lanes).toHaveCount(2);
  await expect(lanes.first().locator(".inspector-lane-head")).toContainText("step 1");

  // A joined tool step shows its name and, behind the disclosure, its args + result.
  const toolStep = page.locator(".tstep.tstep-tool").first();
  await expect(toolStep.locator(".tstep-label")).toHaveText("write");
  await toolStep.locator("summary").click();
  await expect(toolStep.locator(".tstep-pre").first()).toContainText("internal/feature/feature.go");
});

test("a run without an event log degrades to a note, not an error", async ({ page }) => {
  // run-demo-2 ("Review & fix") has no seeded log → the summary card + a note.
  await gotoApp(page);
  await page.evaluate(() =>
    (window as any).htmx.ajax("GET", "/page/runs/run-demo-2", { target: "#main", swap: "innerHTML" }),
  );

  await expect(page.locator(".inspector-summary .run-name")).toHaveText("Review & fix");
  await expect(page.locator("#main")).toContainText("no event log");
  await expect(page.locator(".inspector-lane")).toHaveCount(0);
});
