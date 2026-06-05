import { test, expect } from "@playwright/test";
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
    // The streamed answer finalizes into an assistant turn echoing the prompt.
    await expect(page.locator(sel.agentTurn).last()).toContainText("You said: summarize the repo", {
      timeout: 15_000,
    });
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

  test("the sub-agent activity strip does not leak a chip after the turn settles", async ({ page }) => {
    await gotoApp(page);
    await send(page, "explore");
    // The scripted sub-agent starts and finishes before the answer streams, so
    // by the time the assistant turn lands the activity strip must be empty —
    // the indicator is transient and never leaks once the work is done.
    await expect(page.locator(sel.agentTurn).last()).toContainText("You said: explore", {
      timeout: 15_000,
    });
    await expect(page.locator(`${sel.subagents} .subagent-chip`)).toHaveCount(0);
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

test.describe("sessions", () => {
  test("lists persisted sessions and resumes one, rebuilding its transcript", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Sessions");
    await expect(page.locator("#main")).toContainText("Refactor the auth flow");

    // Resume the first session; its history rehydrates into the chat timeline.
    await page.locator(`#main button[hx-post="/sessions/demo-sess-1/resume"]`).click();
    await expect(page.locator(sel.userTurn).filter({ hasText: "Help me refactor the auth flow" })).toBeVisible();
    // The committed agent turn is markdown-rendered: the fenced code becomes a <pre>.
    await expect(page.locator(`${sel.agentTurn} pre`)).toContainText("parseCredentials");
  });

  test("starts a fresh chat from the sessions page", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Sessions");
    await page.locator(`#main button[hx-post="/sessions/new"]`).click();
    await expect(page.locator(sel.composer)).toBeVisible();
    await expect(page.locator(sel.timeline)).toContainText("new chat");
  });
});
