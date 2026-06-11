import { test, expect } from "./fixtures";
import { sel, pages, gotoApp, navTo, send } from "./helpers";

// End-to-end behaviour of the htmx + SSE web UI, driven against the scripted
// demo server. These exercise the real browser, real htmx swaps, and the real
// SSE stream — the contract the Go unit tests can only approximate.

test.describe("shell & navigation", () => {
  test("boots the chat shell with composer, nav, and cost meter", async ({ page }) => {
    await gotoApp(page);
    await expect(page).toHaveTitle(/my-orchestra/);
    await expect(page.locator(sel.composer)).toBeVisible();
    await expect(page.locator(sel.cost)).toBeVisible();
    await expect(page.locator(sel.nav)).toHaveCount(pages.length);
  });

  test("every nav link swaps its page into #main without a full reload", async ({ page }) => {
    await gotoApp(page);
    for (const p of pages) {
      await navTo(page, p.label);
      // #main is htmx-swapped; the page's landmark must appear inside it.
      await expect(page.locator(`#main ${p.landmark}`).first()).toBeVisible();
    }
  });

  test("the SSE stream stays connected across page navigation", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Telemetry");
    await navTo(page, "Chat");
    // The composer is back and live; a send still streams (proves SSE survived
    // nav). Use a unique marker and match the *committed* turn (excluding the
    // live #cur buffer) so a backlogged earlier turn cannot satisfy or race this
    // assertion — the demo runs one shared session across the whole suite.
    const marker = "nav-survives-sse-check";
    await send(page, marker);
    await expect(
      page.locator(`${sel.agentTurn}:not(#cur)`).filter({ hasText: `You said: ${marker}` }).last(),
    ).toBeVisible({ timeout: 25_000 });
  });
});

test.describe("streaming a turn", () => {
  test("a prompt streams reasoning, a tool call, and the assistant answer", async ({ page }) => {
    await gotoApp(page);
    await send(page, "summarize the repo");

    // Reasoning renders as its own collapsible block, separate from the answer.
    await expect(page.locator(sel.reasoning).last()).toBeVisible();
    // The tool call renders as a first-class timeline card with its name.
    await expect(page.locator(sel.tool).filter({ hasText: "bash" }).last()).toBeVisible();
    // The streamed answer finalizes into a committed assistant turn echoing the
    // prompt. Target the committed turn (`:not(#cur)`) rather than `.last()`, which
    // also matches the transient `#cur` streaming buffer — otherwise the assertion
    // races the buffer reset that fires when EvMessage commits (mirrors the
    // `:not(#cur)` idiom already used by the budget-guardrails spec below).
    await expect(
      page.locator(`${sel.agentTurn}:not(#cur)`).filter({ hasText: "You said: summarize the repo" }).last(),
    ).toBeVisible({ timeout: 15_000 });
  });

  test("the cost footer and context meter update after a turn", async ({ page }) => {
    await gotoApp(page);
    const costBefore = await page.locator(sel.cost).innerText();
    await send(page, "spend some credits");
    // Usage + context-window events arrive late in the scripted turn.
    await expect(page.locator(sel.ctx)).toContainText(/tok|context/, { timeout: 15_000 });
    await expect
      .poll(async () => page.locator(sel.cost).innerText(), { timeout: 15_000 })
      .not.toBe(costBefore);
  });

  test("the statusline shows a pre-flight cost estimate once context is known", async ({ page }) => {
    await gotoApp(page);
    await send(page, "spend some credits");
    // The scripted turn emits a context-window reading; once it lands the
    // statusline projects the next turn's cost at the current context.
    await expect(page.locator(sel.statline)).toContainText(/next turn ~.*cr/, { timeout: 15_000 });
  });

  test("the statusline shows the model and counts the message sent", async ({ page }) => {
    await gotoApp(page);
    // The demo pins the model to gpt-5, so the statusline names it from the start.
    await expect(page.locator(sel.statline)).toContainText("gpt-5");
    // The demo is one shared session, so the counter carries prior tests' sends;
    // assert it increments rather than a fixed value.
    const msgs = async () => {
      const m = (await page.locator(sel.statline).innerText()).match(/✉\s*(\d+)/);
      return m ? Number(m[1]) : -1;
    };
    const before = await msgs();
    await send(page, "spend some credits");
    // Sending a prompt bumps the messages-sent counter immediately (OOB refresh).
    await expect.poll(msgs, { timeout: 15_000 }).toBeGreaterThan(before);
  });

  test("the sub-agent list surfaces each agent's description", async ({ page }) => {
    await gotoApp(page);
    await send(page, "explore");
    // While the scripted sub-agent runs, its row carries the agent's description
    // as a title tooltip so a watcher can see WHAT each concurrent agent is doing.
    // Assert STRUCTURE (a non-empty title attribute), never the exact text.
    const row = page.locator(`${sel.subagents} .subagent-row`).first();
    await expect(row).toHaveAttribute("title", /\S/, { timeout: 15_000 });
  });

  test("the sub-agent list shows a status label and live credits per row", async ({ page }) => {
    await gotoApp(page);
    await send(page, "explore");
    // The live list (issue 0071): each row carries a TEXTUAL status label
    // (working / done / failed — never color or icon alone), the agent's name,
    // and a credits cell (0.00 cr until S3 prices the tagged usage).
    const row = page.locator(`${sel.subagents} .subagent-row`).first();
    await expect(row).toBeVisible({ timeout: 15_000 });
    await expect(row.locator(".sa-status")).toContainText(/working|done/);
    await expect(row.locator(".sa-credits")).toContainText("cr");
  });

  test("a sub-agent's tagged stream is parked, not leaked into the root transcript", async ({ page }) => {
    await gotoApp(page);
    await send(page, "explore");
    // The scripted sub-agent streams its OWN tagged delta/tool while it runs
    // (demo "scanning internal/…", a grep tool). Those AgentID-tagged events are
    // parked by the reducer (epic 0069 S1) — they must NOT interleave into the
    // user-facing transcript. Wait for the committed root reply, then assert the
    // sub-agent's text never appears in the timeline.
    await expect(
      page.locator(`${sel.agentTurn}:not(#cur)`).filter({ hasText: "You said: explore" }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(page.locator(sel.timeline)).not.toContainText("scanning internal/");
  });

  test("a finished sub-agent stays listed with a done status", async ({ page }) => {
    await gotoApp(page);
    await send(page, "explore");
    // The scripted sub-agent starts and finishes before the answer streams. The
    // live list REPLACES the transient strip (issue 0071): once the turn lands,
    // the row stays on the roster with its terminal textual status — done, and
    // verified (the demo completion reports tokens), so no "(unverified)" flag.
    // Target the committed turn (`:not(#cur)`), not `.last()` which also matches
    // the transient `#cur` streaming buffer: once the turn commits, the reply
    // moves out of #cur and #cur resets empty, so `.last()` races the commit (the
    // `:not(#cur)` idiom the other streaming specs above already use).
    await expect(
      page.locator(`${sel.agentTurn}:not(#cur)`).filter({ hasText: "You said: explore" }),
    ).toBeVisible({ timeout: 15_000 });
    const done = page
      .locator(`${sel.subagents} .subagent-row.sa-done`)
      .filter({ hasText: "Explore" })
      .first();
    await expect(done).toBeVisible();
    await expect(done.locator(".sa-status")).not.toContainText("unverified");
  });

  test("opening a sub-agent shows its own transcript in a focus-trapped overlay", async ({ page }) => {
    await gotoApp(page);
    await send(page, "explore");
    // The row persists on the roster after the demo sub-agent finishes, and so does
    // its bounded transcript. Open the drill-down from the row's button (S5): a
    // native <dialog> with the sub-agent's OWN streamed activity — the demo's
    // "scanning internal/…" delta and its grep tool call — which never appeared in
    // the root timeline (asserted above). Structure, not exact wording.
    const row = page
      .locator(`${sel.subagents} .subagent-row`)
      .filter({ hasText: "Explore" })
      .first();
    await expect(row).toBeVisible({ timeout: 15_000 });
    await row.locator(".sa-open").click();

    const dialog = page.locator("dialog#subagent-dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.locator(".sa-transcript-region")).toContainText("scanning internal/");
    await expect(dialog.locator(".sa-transcript .sa-tool-name")).toContainText("grep");

    // ⎋ closes the modal (native dialog), and reopening renders the same bounded
    // transcript — no duplicate turns (idempotent re-render).
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await row.locator(".sa-open").click();
    await expect(page.locator("dialog#subagent-dialog")).toBeVisible();
    await expect(
      page.locator("dialog#subagent-dialog .sa-transcript .sa-t-tool"),
    ).toHaveCount(1);
  });
});

test.describe("inline interaction forms", () => {
  // The demo runs a single shared in-memory session, so fixed-id forms from
  // earlier turns accumulate; we therefore assert on the resolution note the
  // server appends to the timeline rather than on a total form count.
  test("an ask_user question renders inline with choices and resolves on click", async ({ page }) => {
    await gotoApp(page);
    await send(page, "ask me something");
    const ask = page.locator(`${sel.asks} .ask`).last();
    await expect(ask).toBeVisible({ timeout: 10_000 });
    await expect(ask.locator(".ask-choice").first()).toHaveText(/short|detailed/);
    await ask.locator(".ask-choice", { hasText: "short" }).click();
    await expect(page.locator(sel.timeline)).toContainText("answered: short", { timeout: 10_000 });
  });

  test("a plan review renders inline and can be approved", async ({ page }) => {
    await gotoApp(page);
    await send(page, "make a plan");
    const plan = page.locator(`${sel.plans} .plan`).last();
    await expect(plan).toBeVisible({ timeout: 10_000 });
    await expect(plan.locator(".plan-action.recommended")).toBeVisible();
    await plan.locator(".plan-action", { hasText: "proceed" }).click();
    await expect(page.locator(sel.timeline)).toContainText("plan approved: proceed", { timeout: 10_000 });
  });

  test("an MCP elicitation form renders its fields and submits", async ({ page }) => {
    await gotoApp(page);
    await send(page, "configure deploy");
    const form = page.locator(`${sel.elicits} .elicit`).last();
    await expect(form).toBeVisible({ timeout: 10_000 });
    // Schema-driven fields: a select, a number, and a checkbox.
    await expect(form.locator("select")).toBeVisible();
    await expect(form.locator("input[type=number]")).toBeVisible();
    await form.locator(".elicit-ok").click();
    await expect(page.locator(sel.timeline)).toContainText("form submitted", { timeout: 10_000 });
  });

  test("a file-write permission renders a diff review lane and approves", async ({ page }) => {
    await gotoApp(page);
    await send(page, "write a file");
    const review = page.locator(`${sel.perms} .perm-review`).last();
    await expect(review).toBeVisible({ timeout: 10_000 });
    // The proposed change reads as a side-numbered inline diff with typed lines.
    await expect(review).toContainText("internal/summary.go");
    await expect(review.locator(".diff-line.diff-add").first()).toBeVisible();
    await expect(review.locator(".diff-line.diff-del").first()).toBeVisible();
    // Approve/reject post through the existing /perm flow; the server notes it.
    await review.locator("button.ok", { hasText: "approve" }).click();
    await expect(page.locator(sel.timeline)).toContainText("permission approved", { timeout: 10_000 });
  });
});

test.describe("abort & type-ahead", () => {
  test("the stop button is offered while a turn is in flight", async ({ page }) => {
    await gotoApp(page);
    await send(page, "long running");
    await expect(page.locator(sel.abort)).toBeVisible({ timeout: 10_000 });
  });

  test("typing ahead while busy queues the prompt and shows a queued status", async ({ page }) => {
    await gotoApp(page);
    await send(page, "first");
    // Immediately queue a second prompt while the first turn is still streaming.
    await send(page, "queued second");
    await expect(page.locator(sel.status)).toContainText(/queued/, { timeout: 10_000 });
  });
});

test.describe("forge management", () => {
  test("toggling a skill flips its row state", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Skills");
    const firstRow = page.locator(sel.rows).first();
    await expect(firstRow).toBeVisible();
    const wasOn = (await firstRow.getAttribute("class"))?.includes("on");
    await firstRow.locator(".toggle").click();
    // The page re-renders into #main; the same row's on-state must have flipped.
    await expect
      .poll(async () => {
        const cls = (await page.locator(sel.rows).first().getAttribute("class")) ?? "";
        return cls.includes("on");
      })
      .toBe(!wasOn);
  });

  test("selecting an agent marks it active", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Agents");
    const row = page.locator(sel.rows).first();
    await expect(row).toBeVisible();
    await row.locator(".toggle").click();
    await expect(page.locator(`${"" + sel.rows}.on`).first()).toBeVisible();
  });

  test("switching the model marks the new one current", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Models");
    const useBtn = page.locator(`${sel.rows} .toggle`, { hasText: "use" }).first();
    await expect(useBtn).toBeVisible();
    await useBtn.click();
    // After selecting, that row becomes the disabled "current" one.
    await expect(page.locator(`${sel.rows}.on`).first()).toBeVisible();
  });
});

test.describe("MCP server management", () => {
  // The demo seeds the curated stdio servers DISABLED by default; the page lists
  // them and supports add/toggle/edit/delete like the other forge entities. The
  // preflight "unavailable" badge depends on what's installed on the CI host, so
  // assert on structure (rows, toggle state, add), never on badge presence.
  test("lists curated servers and toggles one on", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "MCP");
    await expect(page.locator("#main h2")).toContainText("MCP");
    const firstRow = page.locator(sel.rows).first();
    await expect(firstRow).toBeVisible();
    // Curated entries are seeded disabled; enabling flips the row's on-state.
    const wasOn = (await firstRow.getAttribute("class"))?.includes("on");
    await firstRow.locator(".toggle").click();
    await expect
      .poll(async () => {
        const cls = (await page.locator(sel.rows).first().getAttribute("class")) ?? "";
        return cls.includes("on");
      })
      .toBe(!wasOn);
  });

  test("adds a new MCP server through the form", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "MCP");
    const before = await page.locator(sel.rows).count();
    await page.locator(`#main button.add`).click();
    await expect(page.locator(`#main form[hx-post="/mcp"]`)).toBeVisible();
    // Unique id so reruns against the shared demo session don't collide.
    const id = `e2e-${Date.now()}`;
    await page.fill(`#main input[name="id"]`, id);
    await page.fill(`#main input[name="name"]`, "E2E server");
    await page.fill(`#main input[name="command"]`, "echo");
    await page.locator(`#main button[type=submit]`).click();
    // Back on the list with the new row present.
    await expect(page.locator(sel.rows)).toHaveCount(before + 1);
    await expect(page.locator("#main")).toContainText("E2E server");
  });

  // ADR-0020: the Env editor accepts a masked secret row that persists ONLY a
  // ${VAR} reference. We add a server with a secret Env row, then re-open its
  // edit form and assert the forge doc round-tripped the reference (the masked
  // value shows the bare VAR_NAME, secret checked) — never a raw key.
  test("Env editor stores a secret as a masked ${VAR} reference", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "MCP");
    // Unique id AND name so a retry against the shared demo session can't leave two
    // same-named rows that make the edit locator ambiguous (strict-mode violation).
    const stamp = Date.now();
    const id = `e2e-secret-${stamp}`;
    const name = `E2E secret server ${stamp}`;
    await page.locator(`#main button.add`).click();
    await expect(page.locator(`#main form[hx-post="/mcp"]`)).toBeVisible();
    await page.fill(`#main input[name="id"]`, id);
    await page.fill(`#main input[name="name"]`, name);
    await page.fill(`#main input[name="command"]`, "echo");
    // First Env row: the key the server reads + the env var that holds the secret,
    // marked secret. The user names a variable — never types the secret itself.
    await page.fill(`#main input[name="env.key.0"]`, "GITHUB_TOKEN");
    await page.fill(`#main input[name="env.val.0"]`, "E2E_GH_PAT");
    await page.check(`#main input[name="env.secret.0"]`);
    await page.locator(`#main button[type=submit]`).click();

    // Re-open the edit form for the row we just created and assert the persisted
    // reference round-tripped: the secret box is checked, the value is masked and
    // shows the VAR_NAME, and the ${...} wrapper never leaks into the input.
    await page.locator(sel.rows, { hasText: name }).locator("button.edit").click();
    const secretBox = page.locator(`#main input[name="env.secret.0"]`);
    await expect(secretBox).toBeChecked();
    const secretVal = page.locator(`#main input[name="env.val.0"]`);
    await expect(secretVal).toHaveAttribute("type", "password");
    await expect(secretVal).toHaveValue("E2E_GH_PAT");
  });
});

test.describe("multi-agent workflows", () => {
  // A workflow run streams several lanes, each replaying the scripted demo
  // timeline. Under parallel workers the browser competes for CPU rendering the
  // lane SSE swaps, so run-completion (a structural assertion, not a latency
  // budget) can lag — give these tests extra headroom over the default timeout.
  test.describe.configure({ timeout: 60_000 });

  // The demo seeds a sequential "Build & harden" workflow (builder → sdet).
  // Running it streams each step as a lane on the chat page; the scripted demo
  // lanes complete quickly so the run reaches the finished state.
  test("runs the seeded workflow as streaming lanes that finish", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Workflows");
    await expect(page.locator("#main h2")).toContainText("Workflows");
    const row = page.locator(sel.rows, { hasText: "Build & harden" });
    await expect(row).toBeVisible();
    await row.locator("button.run").click();

    // The run lands on the chat page with the lanes panel.
    await expect(page.locator("#composer")).toBeVisible();
    await expect(page.locator(`${sel.lanes} .workflow-run`)).toBeVisible();
    await expect(page.locator(`${sel.lanes} .lane`).first()).toBeVisible();
    // Both lanes settle and the run reports finished.
    await expect(page.locator(`${sel.lanes} .run-status.done`)).toBeVisible({ timeout: 30_000 });
    await expect(page.locator(`${sel.lanes} .lane-done`).first()).toBeVisible();
  });

  // The demo seeds a PARALLEL "Parallel review" workflow (builder ‖ sdet). With
  // the mock handing out distinct session ids and the demo lanes tagging their
  // events with them (B1 / issue 0015), running it drives two concurrent lanes —
  // each surfacing its own tool card and inline permission, not just output.
  test("runs the parallel workflow as concurrent lanes with per-lane tools and permissions", async ({
    page,
  }) => {
    await gotoApp(page);
    await navTo(page, "Workflows");
    const row = page.locator(sel.rows, { hasText: "Parallel review" });
    await expect(row).toBeVisible();
    await row.locator("button.run").click();

    // The run lands on the chat page; both fan-out lanes render at once.
    await expect(page.locator(`${sel.lanes} .workflow-run`)).toBeVisible();
    await expect(page.locator(`${sel.lanes} .lane`)).toHaveCount(2);
    // The header reports the parallel mode.
    await expect(page.locator(`${sel.lanes} .run-mode`)).toContainText("parallel");
    // Each lane surfaces its own tool timeline and an inline permission form
    // (structure, not figures — the shared demo session).
    await expect(page.locator(`${sel.lanes} .lane-tools .turn.tool`).first()).toBeVisible({
      timeout: 30_000,
    });
    await expect(page.locator(`${sel.lanes} .lane-perms form.perm`).first()).toBeVisible();
    // Both lanes settle and the run reports finished.
    await expect(page.locator(`${sel.lanes} .run-status.done`)).toBeVisible({ timeout: 30_000 });
    await expect(page.locator(`${sel.lanes} .lane-done`)).toHaveCount(2);
  });

  // The demo seeds a sequential BRANCHING "Review, then fix" workflow (B2 /
  // ADR-0021): step 1 (sdet) reviews, step 2 (builder) runs only if the review
  // output contains "issues", step 3 runs only if it says "perfect". The demo lane
  // echoes the prompt's first line, so the review output contains "issues" — the fix
  // step RUNS and the celebrate step SKIPS. Assert structure (a skipped lane), never
  // timing.
  test("runs the branching workflow and skips the unsatisfied lane", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Workflows");
    const row = page.locator(sel.rows, { hasText: "Review, then fix" });
    await expect(row).toBeVisible();
    await row.locator("button.run").click();

    // The run lands on the chat page with three lanes.
    await expect(page.locator(`${sel.lanes} .workflow-run`)).toBeVisible();
    await expect(page.locator(`${sel.lanes} .lane`)).toHaveCount(3);
    // The run finishes; the gated fix lane ran (done) and the celebrate lane skipped.
    await expect(page.locator(`${sel.lanes} .run-status.done`)).toBeVisible({ timeout: 30_000 });
    await expect(page.locator(`${sel.lanes} .lane-skipped`)).toHaveCount(1);
    await expect(page.locator(`${sel.lanes} .lane-skipped`)).toContainText("skipped");
    // Two lanes ran to completion (the review + the satisfied fix step).
    await expect(page.locator(`${sel.lanes} .lane-done`)).toHaveCount(2);
  });

  // The demo seeds an "Escalation demo" workflow (S4 / ADR-0043): the Builder lane
  // hits an ambiguity and ESCALATES, parking as input-required until the human
  // resolves the pause. Running it drives the orchestrator's escalate back-channel
  // offline — the lane goes amber, the inline pause form appears, and clicking
  // continue (with a hint) resumes the lane to completion.
  test("parks a lane as input-required and resumes it from the pause form", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Workflows");
    const row = page.locator(sel.rows, { hasText: "Escalation demo" });
    await expect(row).toBeVisible();
    await row.locator("button.run").click();

    // The run lands on the chat page; the lane parks input-required (amber) and the
    // inline pause form appears with only the declared buttons.
    await expect(page.locator(`${sel.lanes} .workflow-run`)).toBeVisible();
    await expect(page.locator(`${sel.lanes} .lane-input-required`)).toBeVisible({ timeout: 30_000 });
    const pause = page.locator(`${sel.pauses} form.pause`);
    await expect(pause).toBeVisible();
    await expect(pause.locator("button.pause-continue")).toBeVisible();
    await expect(pause.locator("button.pause-cancel")).toBeVisible();

    // The out-of-band attention surface (S6): while the pause is pending the tab
    // title is prefixed with the count and the favicon swaps to the amber-dotted
    // mark — the "needs you" signal visible even on another tab. Title is the firm
    // assertion (favicon swaps can be flaky); the favicon href carries the dot SVG.
    await expect(page).toHaveTitle(/^\(1\) my-orchestra$/);
    await expect(page.locator("#favicon")).toHaveAttribute("href", /circle/);

    // The human continues with a hint; the pause clears and the lane finishes,
    // folding the hint into the lane output.
    await pause.locator("input[name=payload]").fill("use the default branch");
    await pause.locator("button.pause-continue").click();
    await expect(page.locator(`${sel.pauses} form.pause`)).toHaveCount(0);

    // The queue drained → the title and favicon dot are restored.
    await expect(page).toHaveTitle("my-orchestra");
    await expect(page.locator("#favicon")).not.toHaveAttribute("href", /circle/);
    await expect(page.locator(`${sel.lanes} .run-status.done`)).toBeVisible({ timeout: 30_000 });
    await expect(page.locator(`${sel.lanes} .lane-done`).first()).toBeVisible();
    await expect(page.locator(sel.lanes)).toContainText("use the default branch");
  });

  // The Runs page (B3 / ADR-0022) lists persisted workflow runs, most recent first,
  // each with a per-lane breakdown. The demo seeds a couple, and running a workflow
  // appends one. Assert structure (a run-record row appears after a run), never
  // figures — the demo run store is shared + append-only across the suite.
  test("records a completed run in the Runs history", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Runs");
    await expect(page.locator("#main h2")).toContainText("Runs");
    // The seeded demo runs render with a per-lane breakdown.
    await expect(page.locator(".run-record").first()).toBeVisible();
    await expect(page.locator(".run-record .run-lane").first()).toBeVisible();
    // The per-workflow roll-up (V1, ADR-0022) renders above the history: a summary
    // table with at least one per-workflow row, and each run carries a duration cell.
    // Assert structure only — the demo run store is shared + append-only across the
    // suite, so figures (counts/averages) are not stable to assert on.
    await expect(page.locator("table.run-summary")).toBeVisible();
    await expect(page.locator(".run-summary-row").first()).toBeVisible();
    // The summary surfaces a workflow's cumulative orchestrated spend (V13) beside its
    // average — a Total cost cell per row.
    await expect(page.locator(".run-summary-totalcost").first()).toBeVisible();
    await expect(page.locator(".run-record .run-duration").first()).toBeVisible();
    // The "Cost by lane" roll-up (V14) — the finest orchestration-attribution grain
    // (which lane in a workflow costs / fails most) — renders below the per-workflow
    // summary as a share list with at least one lane row. Structure only; the demo run
    // store is shared + append-only across the suite, so figures are not stable.
    await expect(page.locator("#main ul.lane-shares")).toBeVisible();
    await expect(page.locator("#main .lane-share-row").first()).toBeVisible();

    // The run history exports as a CSV (the orchestration sibling of the spend export):
    // the link is present and the route streams a CSV with the documented header.
    const runsLink = page.locator('#main a.export[href="/runs/export.csv"]');
    await expect(runsLink).toBeVisible();
    const runsRes = await page.request.get("/runs/export.csv");
    expect(runsRes.status()).toBe(200);
    expect(runsRes.headers()["content-type"]).toContain("text/csv");
    expect(await runsRes.text()).toContain("run,workflow,name");

    // Each recorded run whose workflow still exists offers a Rerun control (V18,
    // ADR-0023) — a DISJOINT `.rerun` button (≠ the Workflows-page button.run / the
    // a.export links) that re-executes the workflow's current definition. The seeded
    // build-and-harden run is rerunnable (its workflow exists); the orphan review-and-fix
    // run shows none (covered by the Go unit test). Structure only.
    await expect(
      page.locator('#main button.rerun[hx-post^="/runs/rerun/build-and-harden"]').first(),
    ).toBeVisible();

    // The time-window selector (V12) mirrors the Telemetry trend's: three windows
    // (14/30/90), exactly one active, defaulting to 14d. Switching re-fetches the Runs
    // page sliced to the chosen window (the seeded demo runs are recent, so they stay
    // visible across windows — assert the selector state, not figures).
    await expect(page.locator("#main .window-row button.window")).toHaveCount(3);
    await expect(page.locator("#main .window-row button.window.on")).toHaveCount(1);
    await expect(page.locator("#main .window-row button.window.on")).toHaveText("14d");
    await page.locator('#main .window-row button.window:has-text("90d")').click();
    await expect(page.locator("#main .window-row button.window.on")).toHaveText("90d");

    const before = await page.locator(".run-record").count();

    // Run a workflow and wait for it to finish.
    await navTo(page, "Workflows");
    const row = page.locator(sel.rows, { hasText: "Build & harden" });
    await row.locator("button.run").click();
    await expect(page.locator(`${sel.lanes} .run-status.done`)).toBeVisible({ timeout: 30_000 });

    // The finished run is now in the history.
    await navTo(page, "Runs");
    await expect.poll(async () => page.locator(".run-record").count()).toBeGreaterThan(before);
  });

  // The Workflows page (V4, epic 0024) badges each row with a last-run outcome glyph
  // + age, a run count, and total spend — joining the demo run store + spend ledger
  // keyed by workflow id. The demo seeds a "Build & harden" run AND workflow-owned
  // spend for that id, so its row carries badges. Assert STRUCTURE only (the badge
  // cell + run/spend classes) — never figures: the demo run/spend stores are shared +
  // append-only across the suite, so counts/credits drift as it runs.
  test("badges the seeded workflow row with last-run + spend", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Workflows");
    const row = page.locator(sel.rows, { hasText: "Build & harden" });
    await expect(row).toBeVisible();
    await expect(row.locator(".wf-badges")).toBeVisible();
    await expect(row.locator(".wf-lastrun")).toBeVisible();
    await expect(row.locator(".wf-runs")).toBeVisible();
    await expect(row.locator(".wf-spend")).toBeVisible();
  });

  test("adds a workflow through the form", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Workflows");
    const before = await page.locator(sel.rows).count();
    await page.locator(`#main button.add`).click();
    await expect(page.locator(`#main form[hx-post="/workflows"]`)).toBeVisible();
    // Unique id so reruns against the shared demo session don't collide.
    const id = `e2e-wf-${Date.now()}`;
    await page.fill(`#main input[name="id"]`, id);
    await page.fill(`#main input[name="name"]`, "E2E flow");
    await page.fill(`#main textarea[name="steps"]`, "builder: do the thing");
    await page.locator(`#main button[type=submit]`).click();
    await expect(page.locator(sel.rows)).toHaveCount(before + 1);
    await expect(page.locator("#main")).toContainText("E2E flow");
  });
});

test.describe("prompt/snippet library", () => {
  // The demo seeds a couple of snippets (review-pr, explain).
  test("adds a snippet through the form", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Snippets");
    await expect(page.locator("#main h2")).toContainText("Snippets");
    const before = await page.locator(sel.rows).count();
    await page.locator(`#main button.add`).click();
    await expect(page.locator(`#main form[hx-post="/snippets"]`)).toBeVisible();
    // Unique id so reruns against the shared demo session don't collide.
    const id = `e2e-snip-${Date.now()}`;
    await page.fill(`#main input[name="id"]`, id);
    await page.fill(`#main input[name="name"]`, "E2E snippet");
    await page.fill(`#main textarea[name="body"]`, "Do the E2E thing.");
    await page.locator(`#main button[type=submit]`).click();
    await expect(page.locator(sel.rows)).toHaveCount(before + 1);
    await expect(page.locator("#main")).toContainText("E2E snippet");
  });

  test("inserts a snippet body into the composer from the autocomplete", async ({ page }) => {
    await gotoApp(page);
    // Typing the snippet trigger prefix surfaces it as a snippet entry.
    await page.locator(sel.prompt).pressSequentially("/expl", { delay: 60 });
    const item = page.locator(`${sel.cmdMenu} .cmd-snippet`).first();
    await expect(item).toBeVisible({ timeout: 5_000 });
    await item.click();
    // The composer now holds the snippet body (not the "/trigger"), ready to edit.
    await expect(page.locator(sel.prompt)).toHaveValue(/Explain what the selected code does/);
  });

  // C2 / TECH_DEBT #15: the composer is a <textarea>, so inserting a multi-line
  // snippet keeps its line breaks (the old single-line <input> flattened them).
  test("inserts a multi-line snippet keeping its line breaks", async ({ page }) => {
    await gotoApp(page);
    // The demo seeds a multi-line "checklist" snippet.
    await page.locator(sel.prompt).pressSequentially("/checkl", { delay: 60 });
    const item = page.locator(`${sel.cmdMenu} .cmd-snippet`).first();
    await expect(item).toBeVisible({ timeout: 5_000 });
    await item.click();
    // The inserted value still contains newlines — it did not flatten to one line.
    await expect(page.locator(sel.prompt)).toHaveValue(/Review checklist:\n1\. .+\n2\. /);
  });
});

test.describe("slash commands", () => {
  test("typing '/' opens the command autocomplete menu", async ({ page }) => {
    await gotoApp(page);
    // pressSequentially fires real keyup events, which the htmx hx-trigger needs
    // (page.fill() sets the value without dispatching keystrokes).
    await page.locator(sel.prompt).pressSequentially("/", { delay: 60 });
    await expect(page.locator(`${sel.cmdMenu} .cmd-item`).first()).toBeVisible({ timeout: 5_000 });
  });

  test("/help renders the command reference inline as a system note, not a model turn", async ({ page }) => {
    await gotoApp(page);
    const agentTurnsBefore = await page.locator(sel.agentTurn).count();
    await page.fill(sel.prompt, "/help");
    await page.locator(sel.send).click();
    // The command is intercepted: it appends a system note listing the commands…
    await expect(page.locator(sel.timeline)).toContainText(/\/model|\/clear|\/cost/, {
      timeout: 10_000,
    });
    // …and is never dispatched to the model (no new assistant turn).
    await expect(page.locator(sel.agentTurn)).toHaveCount(agentTurnsBefore);
  });
});

test.describe("persisted spend history + trends", () => {
  test("the Telemetry page shows the spend trend and exports CSV", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Telemetry");

    // The demo seeds a multi-day, multi-model ledger, so the trend view renders
    // without any live turn. (The store is append-only and shared across the
    // suite, so assert structure, not exact figures.)
    const main = page.locator("#main");
    // The predictive burn-rate line: from the seeded ledger the page projects when
    // the budget is reached (or hints to set one). Assert the label, never figures
    // — the shared append-only demo ledger grows as the suite runs (A3 / ADR-0019).
    await expect(main).toContainText("Forecast");
    await expect(main).toContainText("Spend history");
    await expect(main).toContainText("Spend over time");
    await expect(main).toContainText("Per-model share");
    // The orchestration-aware cost attribution view: spend broken down by the
    // agent that incurred it and by the workflow that owned it (A2 / ADR-0018).
    // The demo ledger seeds agent ids and a couple of workflow-owned turns.
    await expect(main).toContainText("Cost by agent");
    await expect(main).toContainText("Cost by workflow");
    // The cost⋈run reconciliation (V15): per-workflow ledger spend vs recorded-run
    // spend with their delta — the convergence of the two persisted stores. The demo
    // seeds a workflow that agrees across both and one that diverges. Assert STRUCTURE
    // only (heading + the comparison table + a row); the shared append-only demo grows
    // as the suite runs, so figures/amber drift (same gotcha family as the bars).
    await expect(main).toContainText("Ledger vs runs");
    await expect(page.locator("#main table.recon")).toBeVisible();
    await expect(page.locator("#main tr.recon-row").first()).toBeVisible();
    // The per-lane reconciliation (V16): the same ledger-vs-runs comparison one grain
    // finer — per (workflow, lane). The demo seeds build-and-harden lane-tagged on both
    // sides. Structure only (heading + the per-lane table + a row), same drift gotcha.
    await expect(main).toContainText("Ledger vs runs by lane");
    await expect(page.locator("#main table.lane-recon")).toBeVisible();
    await expect(page.locator("#main tr.lane-recon-row").first()).toBeVisible();
    // The reconciliation export (V17): the ledger-vs-runs divergence leaves the tool as a
    // CSV the way spend and runs already do. A DISJOINT marker class (reconcile-export) so
    // this link can't collide with the spend export's a.export selector below. One file
    // carries both grains — assert the documented header only (figures drift; same gotcha).
    const reconLink = page.locator(
      '#main a.reconcile-export[href="/telemetry/reconcile.csv"]',
    );
    await expect(reconLink).toBeVisible();
    const reconRes = await page.request.get("/telemetry/reconcile.csv");
    expect(reconRes.status()).toBe(200);
    expect(reconRes.headers()["content-type"]).toContain("text/csv");
    expect(await reconRes.text()).toContain(
      "grain,workflow,lane,ledgerCredits,runCredits,delta",
    );
    // The bucketed burn trajectory (F3): beside each agent/workflow share bar the
    // page projects that bucket's recent pace. Assert the trajectory STRUCTURE, never
    // figures — the demo ledger is shared + append-only across the suite (the same
    // gotcha family as the trend bars above; A2 ⋈ A3 / ADR-0018+0019).
    await expect(page.locator("#main li.trajectory").first()).toBeVisible();
    // At least one day bar and one share bar are present.
    await expect(page.locator("#main ul.trend .trend-row").first()).toBeVisible();

    // The export link downloads a CSV with the documented header.
    const link = page.locator('#main a.export[href="/telemetry/export.csv"]');
    await expect(link).toBeVisible();
    const res = await page.request.get("/telemetry/export.csv");
    expect(res.status()).toBe(200);
    expect(res.headers()["content-type"]).toContain("text/csv");
    expect(await res.text()).toContain("at,session,model");
  });

  test("the spend-over-time window selector switches and re-renders the trend", async ({
    page,
  }) => {
    await gotoApp(page);
    await navTo(page, "Telemetry");

    const main = page.locator("#main");
    await expect(main).toContainText("Spend over time");
    // The three windows (14/30/90) are offered, and exactly one is active. Assert
    // STRUCTURE only — the shared append-only demo ledger grows as the suite runs,
    // so day counts/figures are not stable (same gotcha family as the trend bars).
    const buttons = page.locator("#main .window-row button.window");
    await expect(buttons).toHaveCount(3);
    await expect(page.locator("#main .window-row button.window.on")).toHaveCount(1);
    // Default is the 14-day window (the historical behavior).
    await expect(page.locator("#main .window-row button.window.on")).toHaveText("14d");

    // Switching widens the window: the 90-day button becomes active and the trend
    // re-renders (a day bar is still present afterwards).
    await page.locator('#main .window-row button.window:has-text("90d")').click();
    await expect(page.locator("#main .window-row button.window.on")).toHaveText("90d");
    await expect(page.locator("#main ul.trend .trend-row").first()).toBeVisible();
  });
});

test.describe("budget guardrails (hard cap)", () => {
  // Set the hard cap through the Settings form. Saving applies it to the live
  // session immediately (the gate reads the refreshed value on the next turn).
  async function setHardCap(page: import("@playwright/test").Page, credits: number) {
    await navTo(page, "Settings");
    await expect(page.locator("#main form.forge-form")).toBeVisible();
    await page.fill('#main input[name="hardCap"]', String(credits));
    await page.locator("#main button[type=submit]").click();
    await expect(page.locator("#main p.ok")).toContainText("saved");
  }

  // Always lift the cap again so it can't leak into the shared demo session and
  // gate unrelated tests that run afterwards.
  test.afterEach(async ({ page }) => {
    await setHardCap(page, 0);
  });

  test("a turn over the cap pauses inline, then proceeds on confirmation", async ({ page }) => {
    await gotoApp(page);
    // Warm up so a context-window reading and some spend exist — the projected
    // next-turn cost is then non-trivial and will breach a tiny cap. The demo is
    // one shared session, so the statusline already carries a stale "next turn ~";
    // wait for *this* turn to commit and the stop button to clear so the next send
    // is dispatched fresh, not queued behind an in-flight warm-up.
    await send(page, "warm up the meter");
    await expect(
      page.locator(`${sel.agentTurn}:not(#cur)`).filter({ hasText: "You said: warm up the meter" }).last(),
    ).toBeVisible({ timeout: 25_000 });
    await expect(page.locator(sel.abort)).toHaveCount(0);

    await setHardCap(page, 1);
    await navTo(page, "Chat");

    await send(page, "an over-budget turn");
    // The inline gate appears instead of dispatching the turn (whether the turn
    // gated directly or — under any residual race — on queue drain over SSE).
    const gate = page.locator(`${sel.budget} .budget`);
    await expect(gate).toContainText(/exceed your budget cap/, { timeout: 15_000 });

    // Proceeding releases the held turn; the assistant reply streams in and the
    // gate clears.
    await gate.getByRole("button", { name: "proceed" }).click();
    await expect(page.locator(`${sel.agentTurn}:not(#cur)`).filter({ hasText: "You said: an over-budget turn" }).last())
      .toBeVisible({ timeout: 25_000 });
    await expect(page.locator(sel.budget)).toBeEmpty();
  });
});

test.describe("settings price overrides (G1)", () => {
  // The per-model rate table mutates the shared demo config, so always clear the
  // override again afterwards — a left-over rate would skew every later test's
  // cost figures (same shared-config gotcha as the hard-cap reset above).
  test.afterEach(async ({ page }) => {
    await navTo(page, "Settings");
    await expect(page.locator("#main form.forge-form")).toBeVisible();
    const row = page.locator("#main .price-row", { hasText: "claude-opus-4.7" });
    const inputs = row.locator("input[type=number]");
    for (let i = 0; i < 3; i++) await inputs.nth(i).fill("");
    await page.locator("#main button[type=submit]").click();
    await expect(page.locator("#main p.ok")).toContainText("saved");
  });

  test("renders the per-model rate table and saves an override", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Settings");
    // The section and its structure: a subhead and one row per model, each with
    // four numeric fields (input / cached / output / cache-write per-MTok; the
    // cache-write column is ADR-0034).
    await expect(page.locator("#main")).toContainText("Per-model price overrides");
    const row = page.locator("#main .price-row", { hasText: "claude-opus-4.7" });
    await expect(row).toBeVisible();
    await expect(row.locator("input[type=number]")).toHaveCount(4);
    await expect(row.locator('input[type=hidden][value="claude-opus-4.7"]')).toHaveCount(1);

    // Drive a save — never assert exact figures (the demo config is shared).
    await row.locator("input[type=number]").first().fill("123");
    await page.locator("#main button[type=submit]").click();
    await expect(page.locator("#main p.ok")).toContainText("saved");
  });
});

test.describe("sessions", () => {
  test("lists persisted sessions and resumes one, rebuilding its transcript", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Sessions");
    await expect(page.locator("#main")).toContainText("Refactor the auth flow");

    // Resume the first session; its history rehydrates into the chat timeline.
    await page.locator(`#main button[hx-post="/sessions/demo-sess-1/resume"]`).click();
    await expect(page.locator(sel.userTurn).filter({ hasText: "Help me refactor the auth flow" })).toBeVisible();
    // The committed agent turn is markdown-rendered: the fenced code becomes a
    // designed code block (R3) — a token-styled frame whose head carries the
    // language label + a progressive copy affordance, over the <pre> body.
    const codeBlock = page.locator(`${sel.agentTurn} .code-block`).first();
    await expect(codeBlock).toBeVisible();
    await expect(codeBlock.locator(".code-lang")).toHaveText("go");
    await expect(codeBlock.locator(".code-copy")).toBeVisible();
    await expect(codeBlock.locator("pre code.language-go")).toContainText("parseCredentials");
    // GitHub-alert blockquotes render as designed callout components (R2): the
    // kind-classed surface carries a glyph + text label (never color alone).
    const note = page.locator(`${sel.agentTurn} .callout.callout-note`);
    await expect(note).toBeVisible();
    await expect(note.locator(".callout-title")).toHaveText("Note");
    await expect(note.locator(".callout-glyph")).toBeVisible();
    await expect(page.locator(`${sel.agentTurn} .callout.callout-warning .callout-title`)).toHaveText("Warning");
    // GFM pipe tables render as a token-styled table component (R4): a header
    // row + body rows, with per-column alignment carried on a fixed class.
    const table = page.locator(`${sel.agentTurn} table.md-table`).first();
    await expect(table).toBeVisible();
    await expect(table.locator("thead th").first()).toHaveText("Check");
    await expect(table.locator("thead th.ta-center")).toHaveText("Status");
    await expect(table.locator("tbody tr")).toHaveCount(2);
    // Container directives (R5) render as designed, model-authorable blocks
    // resolved against the registry allowlist: :::card is a token-styled surface,
    // :::details a native (JS-free) collapsible whose summary comes from the attr.
    const card = page.locator(`${sel.agentTurn} .directive.dir-card`).first();
    await expect(card).toBeVisible();
    await expect(card).toContainText("parsing from policy");
    const details = page.locator(`${sel.agentTurn} details.dir-details`).first();
    await expect(details.locator("summary.dir-summary")).toHaveText("Migration steps");
    // Native <details> is collapsed by default; the body shows once toggled open.
    await expect(details).not.toHaveAttribute("open", /.*/);
    await details.locator("summary").click();
    await expect(details).toHaveAttribute("open", /.*/);
    await expect(details.locator(".dir-body")).toContainText("Rotate the signing key");
  });

  test("starts a fresh chat from the sessions page", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Sessions");
    await page.locator(`#main button[hx-post="/sessions/new"]`).click();
    await expect(page.locator(sel.composer)).toBeVisible();
    await expect(page.locator(sel.timeline)).toContainText("new chat");
  });

  test("shows a per-session cost cell on each listed session (G2)", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Sessions");
    // The demo ledger tags spend with demo-sess-1, so its row carries a cost cell.
    // The shared demo ledger is append-only across the suite — assert the cost-cell
    // STRUCTURE, never exact figures (same gotcha family as the trend view).
    const row = page.locator(`#main li.session-row`).filter({
      has: page.locator(`button[hx-post="/sessions/demo-sess-1/resume"]`),
    });
    await expect(row.locator(".session-cost")).toBeVisible();
  });
});
