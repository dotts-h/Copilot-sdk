import { test, expect } from "@playwright/test";
import { sel, gotoApp, navTo } from "./helpers";

// Keyboard-shortcut surface (item 3.3): a config-backed keymap rendered onto the
// body, dispatched by a small vanilla-JS handler, surfaced in a help overlay and
// the Settings form. These checks exercise the real htmx/JS behaviour the Go
// unit tests can't: the overlay toggle, single-key navigation, and the
// don't-hijack-typing guard. They read shared config but never mutate it, so no
// afterEach reset is needed (cf. the shared-config gotcha in REGRESSIONS).

test("the shortcut key opens the help overlay and Escape closes it", async ({ page }) => {
  await gotoApp(page);
  const overlay = page.locator("#help-overlay");
  await expect(overlay).toBeHidden();

  // Move focus off the autofocused composer, then press the help key.
  await page.locator(sel.timeline).click();
  await page.keyboard.press("?");
  await expect(overlay).toBeVisible();
  await expect(overlay).toContainText("Keyboard shortcuts");

  await page.keyboard.press("Escape");
  await expect(overlay).toBeHidden();
});

test("other shortcuts don't fire behind the open overlay", async ({ page }) => {
  await gotoApp(page);
  await page.locator(sel.timeline).click();
  await page.keyboard.press("?");
  await expect(page.locator("#help-overlay")).toBeVisible();
  // ',' is bound to gotoSettings; with the modal open it must be suppressed.
  await page.keyboard.press(",");
  await expect(page.locator("#help-overlay")).toBeVisible();
  await expect(page.locator(sel.composer)).toBeVisible(); // still on chat, not Settings
});

test("a single-key shortcut navigates to a page", async ({ page }) => {
  await gotoApp(page);
  await page.locator(sel.timeline).click();
  await page.keyboard.press(","); // gotoSettings
  await expect(page.locator("#main h2")).toHaveText("Settings");
});

test("shortcuts are ignored while typing in the composer", async ({ page }) => {
  await gotoApp(page);
  // These characters are bound to shortcuts, but typed into a field they must be
  // text, not actions — the overlay stays closed and the value is preserved.
  await page.locator(sel.prompt).pressSequentially("c,n?x", { delay: 20 });
  await expect(page.locator(sel.prompt)).toHaveValue("c,n?x");
  await expect(page.locator("#help-overlay")).toBeHidden();
});

test("the Settings form lists the keyboard-shortcut fields", async ({ page }) => {
  await gotoApp(page);
  await navTo(page, "Settings");
  await expect(page.locator("#main")).toContainText("Keyboard shortcuts");
  await expect(page.locator('input[name="key_help"]')).toHaveValue("?");
});

test("the Help page lists the shortcuts", async ({ page }) => {
  await gotoApp(page);
  await navTo(page, "Help");
  await expect(page.locator("#main")).toContainText("Keyboard shortcuts");
  await expect(page.locator("#main kbd").first()).toBeVisible();
});
