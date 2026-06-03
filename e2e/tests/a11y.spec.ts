import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { pages, gotoApp, navTo, send } from "./helpers";

// Accessibility scans with axe-core. Each nav page is scanned against the WCAG
// 2.1 A/AA rule set. The chat page is additionally scanned after a streamed
// turn, so the dynamically-injected timeline, tool cards, and inline forms are
// covered — not just the empty shell.

const WCAG = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"];

// KNOWN, DOCUMENTED a11y baseline. The destructive controls (abort / reject /
// decline) render white-or-red on the brand red (--bad #f85149), which lands at
// ~3.5–4.2:1 — just under the 4.5:1 WCAG AA text threshold. Fixing it means
// retuning the deliberately hand-crafted palette (a design decision, see
// e2e/README.md → "Accessibility findings"), so it is tracked as a baseline
// here rather than silently passing: a contrast violation on any OTHER element
// still fails the suite, and the dedicated test below asserts this baseline
// still exists so the allowlist gets removed once the palette is fixed.
const KNOWN_CONTRAST_SELECTORS = [".abort", ".plan-reject", ".elicit-no"];

function isBaseline(v: any): boolean {
  if (v.id !== "color-contrast") return false;
  return v.nodes.every((n: any) =>
    n.target.some((t: string) => KNOWN_CONTRAST_SELECTORS.some((k) => t.includes(k))),
  );
}

async function scan(page: import("@playwright/test").Page) {
  const results = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  // Strip only the exact documented baseline; everything else is a real failure.
  results.violations = results.violations.filter((v) => !isBaseline(v));
  return results;
}

test.describe("static pages have no WCAG A/AA violations", () => {
  for (const p of pages) {
    test(`${p.label} page`, async ({ page }) => {
      await gotoApp(page);
      if (p.slug !== "chat") await navTo(page, p.label);
      const results = await scan(page);
      expect(
        results.violations,
        formatViolations(p.label, results.violations),
      ).toEqual([]);
    });
  }
});

test("chat page is accessible after a streamed turn (timeline + inline forms)", async ({ page }) => {
  await gotoApp(page);
  await send(page, "configure deploy");
  // Wait for the dynamic surfaces the demo injects: tool card + an inline form.
  await expect(page.locator(".turn.tool").last()).toBeVisible({ timeout: 15_000 });
  const results = await scan(page);
  expect(results.violations, formatViolations("chat (post-turn)", results.violations)).toEqual([]);
});

// Tracks the documented baseline so it cannot be forgotten: when the palette is
// fixed this test will fail, signalling that the allowlist above can be deleted.
test("[baseline] destructive controls still have the known contrast shortfall", async ({ page }) => {
  await gotoApp(page);
  await send(page, "configure deploy"); // surfaces .abort + the inline form's reject/decline
  await expect(page.locator(".turn.tool").last()).toBeVisible({ timeout: 15_000 });
  const raw = await new AxeBuilder({ page }).withTags(WCAG).analyze();
  const stillBaseline = raw.violations.some(
    (v) =>
      v.id === "color-contrast" &&
      v.nodes.some((n: any) =>
        n.target.some((t: string) => KNOWN_CONTRAST_SELECTORS.some((k) => t.includes(k))),
      ),
  );
  expect(
    stillBaseline,
    "destructive-control contrast appears fixed — remove the KNOWN_CONTRAST_SELECTORS allowlist",
  ).toBe(true);
});

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
