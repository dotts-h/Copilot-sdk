import { test, expect } from "@playwright/test";
import { sel, pages, gotoApp } from "./helpers";

// The canonical start-state smoke: the offline demo binary self-seeds an
// in-memory session (no setup needed), so this spec just asserts the app boots
// to the interactive start state every other spec assumes. If this fails, the
// shared fixture is broken and the rest of the suite's failures are noise — run
// this first. (The demo's one-shared-session constraint is why workers: 1 /
// fullyParallel: false; see playwright.config.ts.)
test.describe("start state", () => {
  test("the shell boots to an interactive composer", async ({ page }) => {
    await gotoApp(page);
    // Composer is present and ready to send.
    await expect(page.locator(sel.prompt)).toBeEnabled();
    await expect(page.locator(sel.send)).toBeEnabled();
    // The cost footer renders (the cost-awareness surface is always on).
    await expect(page.locator(sel.cost)).toBeVisible();
  });

  test("the nav lists exactly the known pages", async ({ page }) => {
    await gotoApp(page);
    // The nav link count is coupled to the pages array (a new page must be added
    // there in nav order) — assert it here so the coupling is checked once,
    // canonically, in the seed spec. See REGRESSIONS "adding a nav page".
    await expect(page.locator(sel.nav)).toHaveCount(pages.length);
    for (const p of pages) {
      await expect(
        page.locator(sel.nav, { hasText: new RegExp(`^${p.label}$`) }),
      ).toBeVisible();
    }
  });
});
