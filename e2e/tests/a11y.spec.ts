import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { pages, gotoApp, navTo, send } from "./helpers";

// Accessibility scans with axe-core. Each nav page is scanned against the WCAG
// 2.1 A/AA rule set, in BOTH the light and dark theme (ADR-0025) — the palette
// retune cleared the former destructive-control contrast baseline, so there is
// no allowlist: a contrast violation on any element in either theme fails the
// suite. The chat page is additionally scanned after a streamed turn, in both
// themes, so the dynamically-injected timeline, tool cards, and inline forms
// (including the destructive abort / reject / decline controls) are covered.

const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"];
const THEMES = ["light", "dark"] as const;

// setTheme forces a color scheme by setting <html data-theme> (→ color-scheme),
// so light-dark() tokens resolve deterministically regardless of the emulated OS
// preference. The attribute lives on <html>, so it survives htmx #main swaps.
async function setTheme(page: import("@playwright/test").Page, theme: string) {
  await page.evaluate((t) => document.documentElement.setAttribute("data-theme", t), theme);
}

async function scan(page: import("@playwright/test").Page) {
  return new AxeBuilder({ page }).withTags(WCAG).analyze();
}

test.describe("static pages have no WCAG A/AA violations (both themes)", () => {
  for (const p of pages) {
    for (const theme of THEMES) {
      test(`${p.label} page — ${theme}`, async ({ page }) => {
        await gotoApp(page);
        await setTheme(page, theme);
        if (p.slug !== "chat") await navTo(page, p.label);
        const results = await scan(page);
        expect(
          results.violations,
          formatViolations(`${p.label} (${theme})`, results.violations),
        ).toEqual([]);
      });
    }
  }
});

for (const theme of THEMES) {
  test(`chat page is accessible after a streamed turn — ${theme} (timeline + inline forms)`, async ({ page }) => {
    await gotoApp(page);
    await setTheme(page, theme);
    await send(page, "configure deploy");
    // Wait for the dynamic surfaces the demo injects: tool card, the diff review
    // lane (item 3.1), and another inline form — so the scan covers them, plus
    // the destructive abort / reject / decline controls, not the empty shell.
    await expect(page.locator(".turn.tool").last()).toBeVisible({ timeout: 15_000 });
    await expect(page.locator("#perms .perm-review").last()).toBeVisible({ timeout: 15_000 });
    // Wait for the schema-driven elicitation form too, so its secondary text
    // (.elicit-desc / .elicit-source) is scanned in both themes — it dimmed via
    // `opacity` and fell to ~4.13:1 on the light theme until tokenized (REGRESSIONS #20).
    await expect(page.locator("#elicits .elicit-desc").last()).toBeVisible({ timeout: 15_000 });
    const results = await scan(page);
    expect(
      results.violations,
      formatViolations(`chat post-turn (${theme})`, results.violations),
    ).toEqual([]);
  });
}

// The grouped sidebar (V22) is always visible, so it is covered by every page
// scan above. The command palette is hidden until opened, so scan it open, in
// both themes, to cover the dialog + its filter input and items (ADR-0026).
for (const theme of THEMES) {
  test(`command palette is accessible when open — ${theme}`, async ({ page }) => {
    await gotoApp(page);
    await setTheme(page, theme);
    await page.keyboard.press("Control+k");
    await expect(page.locator("#cmdk-overlay")).toBeVisible();
    const results = await scan(page);
    expect(
      results.violations,
      formatViolations(`command palette (${theme})`, results.violations),
    ).toEqual([]);
  });
}

test("the page exposes a single top-level landmark structure", async ({ page }) => {
  await gotoApp(page);
  await expect(page.locator("header")).toHaveCount(1);
  await expect(page.locator("main#main")).toHaveCount(1);
  // <html lang> is required for screen-reader language selection.
  const lang = await page.locator("html").getAttribute("lang");
  expect(lang).toBe("en");
});

// formatViolations renders a readable failure message listing each rule, its
// impact, and the offending selectors.
function formatViolations(label: string, violations: any[]): string {
  if (violations.length === 0) return `${label}: no violations`;
  return (
    `${label}: ${violations.length} a11y violation(s)\n` +
    violations
      .map(
        (v) =>
          `  [${v.impact}] ${v.id}: ${v.help}\n` +
          v.nodes.map((n: any) => `      ${n.target.join(" ")}`).join("\n"),
      )
      .join("\n")
  );
}
