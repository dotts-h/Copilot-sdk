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

test("the transcript view flattens the run into chat order through the markdown renderer", async ({ page }) => {
  // run-demo-1's seeded log carries committed messages with designed markdown and
  // priced usage — the transcript (O3, issue 0093) renders them as chat turns.
  await gotoApp(page);
  await page.evaluate(() =>
    (window as any).htmx.ajax("GET", "/page/runs/run-demo-1?view=transcript", { target: "#main", swap: "innerHTML" }),
  );

  // The chat-order container renders user + assistant turns, not the lane-grouped timeline.
  await expect(page.locator(".transcript")).toBeVisible();
  await expect(page.locator(".inspector-lane")).toHaveCount(0);
  await expect(page.locator(".transcript .turn.user").first()).toContainText("Build the feature");

  // The assistant body went through the block-AST renderer: the table and code block render designed.
  const assistant = page.locator(".transcript .turn.assistant").first();
  await expect(assistant.locator("table")).toBeVisible();
  await expect(assistant.locator("pre code")).toContainText("func Feature()");
  // Per-turn pricing (O2) shows on the assistant turn.
  await expect(assistant.locator(".tx-cost")).toContainText("2.60");

  // The view toggle switches back to the timeline (the same events, lane-grouped).
  await page.locator(".view-row .view", { hasText: /^timeline$/ }).click();
  await expect(page.locator(".inspector-lane").first()).toBeVisible();
  await expect(page.locator(".transcript")).toHaveCount(0);
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
