import { Page, expect } from "@playwright/test";

// Central selector map so a markup change is a one-line fix, not a sweep.
export const sel = {
  composer: "#composer",
  prompt: "#composer textarea[name=prompt]",
  send: "#composer button[type=submit]",
  timeline: "#timeline",
  userTurn: ".turn.user",
  agentTurn: ".turn.assistant",
  reasoning: ".turn.reasoning",
  tool: ".turn.tool",
  status: "#status",
  abort: ".abort",
  cost: "#cost-footer",
  ctx: "#ctx",
  statline: "#statline",
  nav: "header nav a",
  perms: "#perms",
  asks: "#asks",
  plans: "#plans",
  elicits: "#elicits",
  budget: "#budget",
  subagents: "#subagents",
  lanes: "#lanes",
  cmdMenu: "#cmd-menu",
  rows: ".rows .row",
} as const;

// The nav pages, in nav order, with a landmark string each test can anchor on.
export const pages = [
  { slug: "chat", label: "Chat", landmark: "#composer" },
  { slug: "sessions", label: "Sessions", landmark: "h2" },
  { slug: "telemetry", label: "Telemetry", landmark: "h2" },
  { slug: "skills", label: "Skills", landmark: "h2" },
  { slug: "instructions", label: "Instructions", landmark: "h2" },
  { slug: "agents", label: "Agents", landmark: "h2" },
  { slug: "workflows", label: "Workflows", landmark: "h2" },
  { slug: "runs", label: "Runs", landmark: "h2" },
  { slug: "mcp", label: "MCP", landmark: "h2" },
  { slug: "snippets", label: "Snippets", landmark: "h2" },
  { slug: "hooks", label: "Hooks", landmark: "h2" },
  { slug: "connection", label: "Connection", landmark: "h2" },
  { slug: "models", label: "Models", landmark: "h2" },
  { slug: "settings", label: "Settings", landmark: "h2" },
  { slug: "help", label: "Help", landmark: "h2" },
] as const;

// gotoApp opens the shell and waits for the composer to be interactive, which
// only happens after htmx has booted and the SSE stream is connecting. It also
// installs a settle counter (see navTo) so navigation can be awaited even though
// V24's View-Transition page swaps (ADR-0028) defer the #main DOM update.
export async function gotoApp(page: Page) {
  // Count every htmx settle on the document. afterSettle fires AFTER the swap's
  // DOM update — even inside a View Transition's update callback — so a test can
  // wait for it before reading the just-swapped DOM. Registered before goto so it
  // runs at document-start; re-runs (resetting to 0) on any reload, which is fine.
  await page.addInitScript(() => {
    (window as unknown as { __settles: number }).__settles = 0;
    (window as unknown as { __sseOpen: boolean }).__sseOpen = false;
    document.addEventListener("htmx:afterSettle", () => {
      (window as unknown as { __settles: number }).__settles++;
    });
    // htmx-ext-sse fires htmx:sseOpen (bubbling to document) when the body-level
    // sse-connect="/events" stream actually opens.
    document.addEventListener("htmx:sseOpen", () => {
      (window as unknown as { __sseOpen: boolean }).__sseOpen = true;
    });
  });
  await page.goto("/");
  await expect(page.locator(sel.prompt)).toBeVisible();
  // Wait for the SSE stream to be open before returning. Otherwise the first
  // send() can outrace the /events subscription on a cold connection: the user
  // bubble still appears (the synchronous /send OOB swap), but the streamed
  // assistant turn is broadcast to zero subscribers and never arrives. This bit
  // the first turn against a freshly-spawned per-worker server (tests/fixtures.ts).
  await page.waitForFunction(
    () => (window as unknown as { __sseOpen?: boolean }).__sseOpen === true,
  );
}

// navTo clicks a nav link and waits for the target page to swap into #main.
// V24 (ADR-0028) opts the nav swap into a same-document View Transition, which
// defers the #main DOM update by a frame; without waiting, a synchronous read
// right after navTo (e.g. a baseline `.count()`) would see the OLD page. Waiting
// for htmx's next settle makes navigation deterministic again for every caller.
export async function navTo(page: Page, label: string) {
  const before = await page.evaluate(
    () => (window as unknown as { __settles?: number }).__settles ?? 0,
  );
  await page.locator(sel.nav, { hasText: new RegExp(`^${label}$`) }).click();
  await page.waitForFunction(
    (n) => ((window as unknown as { __settles?: number }).__settles ?? 0) > n,
    before,
  );
}

// send types a prompt and submits it, then waits for the user bubble to appear
// in the timeline (the synchronous OOB swap from POST /send).
export async function send(page: Page, text: string) {
  await page.fill(sel.prompt, text);
  await page.locator(sel.send).click();
  await expect(page.locator(sel.userTurn).filter({ hasText: text }).last()).toBeVisible();
}
