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
  { slug: "models", label: "Models", landmark: "h2" },
  { slug: "settings", label: "Settings", landmark: "h2" },
  { slug: "help", label: "Help", landmark: "h2" },
] as const;

// gotoApp opens the shell and waits for the composer to be interactive, which
// only happens after htmx has booted and the SSE stream is connecting.
export async function gotoApp(page: Page) {
  await page.goto("/");
  await expect(page.locator(sel.prompt)).toBeVisible();
}

// navTo clicks a nav link and waits for the target page to swap into #main.
export async function navTo(page: Page, label: string) {
  await page.locator(sel.nav, { hasText: new RegExp(`^${label}$`) }).click();
}

// send types a prompt and submits it, then waits for the user bubble to appear
// in the timeline (the synchronous OOB swap from POST /send).
export async function send(page: Page, text: string) {
  await page.fill(sel.prompt, text);
  await page.locator(sel.send).click();
  await expect(page.locator(sel.userTurn).filter({ hasText: text }).last()).toBeVisible();
}
