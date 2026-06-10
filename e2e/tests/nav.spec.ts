import { test, expect } from "./fixtures";
import { sel, gotoApp } from "./helpers";

// Grouped left-sidebar navigation (V22, ADR-0026). The flat 13-item top bar
// becomes a left sidebar whose pages are clustered into intent groups —
// Primary · Build · Observe · Config · Help — with Config + Help pinned to the
// bottom. The banner is still the single <header> containing <nav class="nav">,
// so the existing `header nav a` selectors keep working.

const GROUPS = ["Primary", "Build", "Observe", "Config", "Help"] as const;

// The pages under each group, in render order (Config + Help pinned last).
const MEMBERS: Record<string, string[]> = {
  Primary: ["Chat", "Sessions"],
  Build: ["Agents", "Workflows", "Skills", "Instructions", "Snippets", "Hooks"],
  Observe: ["Runs", "Telemetry"],
  Config: ["Connection", "Models", "MCP", "Settings"],
  Help: ["Help"],
};

test("the sidebar renders the intent groups in order with Help last", async ({ page }) => {
  await gotoApp(page);
  await expect(page.locator(".sidebar")).toBeVisible();

  const labels = (await page.locator(".nav-group-label").allTextContents()).map((s) => s.trim());
  expect(labels).toEqual([...GROUPS]);

  // Help is the final nav destination (config + help pinned to the bottom).
  await expect(page.locator(sel.nav).last()).toHaveText("Help");
});

test("each group lists exactly its pages", async ({ page }) => {
  await gotoApp(page);
  for (const group of GROUPS) {
    const groupEl = page.locator(".nav-group", {
      has: page.locator(".nav-group-label", { hasText: new RegExp(`^${group}$`) }),
    });
    const links = (await groupEl.locator("a").allTextContents()).map((s) => s.trim());
    expect(links, `members of ${group}`).toEqual(MEMBERS[group]);
  }
});

test("clicking a sidebar item swaps the main panel", async ({ page }) => {
  await gotoApp(page);
  await page.locator(sel.nav, { hasText: /^Runs$/ }).click();
  await expect(page.locator("#main h2")).toContainText(/Runs/);
  await page.locator(sel.nav, { hasText: /^Telemetry$/ }).click();
  await expect(page.locator("#main h2")).toContainText(/Telemetry/);
});

test("the theme toggle and cost footer remain reachable in the sidebar", async ({ page }) => {
  await gotoApp(page);
  await expect(page.locator(".sidebar .theme-toggle")).toBeVisible();
  await expect(page.locator(".sidebar #cost-footer")).toBeVisible();
});
