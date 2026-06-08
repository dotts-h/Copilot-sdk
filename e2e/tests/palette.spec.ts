import { test, expect } from "@playwright/test";
import { sel, gotoApp } from "./helpers";

// ⌘/Ctrl-K command palette (V22, ADR-0026): a body-level aria-modal dialog with
// a filter input over a server-rendered {slug,label,group} list, filtered
// client-side and navigating the match through the existing keymap dispatch
// (navClick) — no new server route. ⌘K is a fixed modifier chord, opened ahead
// of the configurable single-key keymap.

const palette = "#cmdk-overlay";
const input = ".cmdk-input";

// The chord uses Meta on macOS, Control elsewhere; both are wired in the
// dispatcher, so press Control here (the CI runner is Linux).
const CHORD = "Control+k";

test("⌘/Ctrl-K opens the palette, Esc closes it and returns focus to the composer", async ({ page }) => {
  await gotoApp(page);
  await expect(page.locator(palette)).toBeHidden();

  // Open from the composer (the chord fires even while a text field is focused).
  await page.locator(sel.prompt).focus();
  await page.keyboard.press(CHORD);
  await expect(page.locator(palette)).toBeVisible();
  await expect(page.locator(palette)).toHaveAttribute("aria-modal", "true");
  await expect(page.locator(input)).toBeFocused();

  // Esc closes and returns focus to the composer (still on the chat page).
  await page.keyboard.press("Escape");
  await expect(page.locator(palette)).toBeHidden();
  await expect(page.locator(sel.prompt)).toBeFocused();
});

test("typing filters the list and Enter navigates to the top match", async ({ page }) => {
  await gotoApp(page);
  await page.keyboard.press(CHORD);
  await expect(page.locator(palette)).toBeVisible();

  // Typing filters the list down to the matching item(s).
  await page.locator(input).fill("telem");
  const visible = page.locator(`${palette} .cmdk-item:visible`);
  await expect(visible).toHaveCount(1);
  await expect(visible.first()).toContainText("Telemetry");

  // Enter navigates to the top match and closes the palette.
  await page.locator(input).press("Enter");
  await expect(page.locator(palette)).toBeHidden();
  await expect(page.locator("#main h2")).toContainText(/Telemetry/);
});

test("the palette is keyboard reachable and an item click navigates", async ({ page }) => {
  await gotoApp(page);
  await page.keyboard.press(CHORD);
  await expect(page.locator(palette)).toBeVisible();

  // Every page is reachable from the palette; clicking one navigates.
  await page.locator(`${palette} .cmdk-item`, { hasText: /^Runs/ }).first().click();
  await expect(page.locator(palette)).toBeHidden();
  await expect(page.locator("#main h2")).toContainText(/Runs/);
});

// Regression: while the palette is open, a bound shortcut (the default '?' help
// key) must not fire and stack a second aria-modal dialog behind it. The modal
// guard blocks every bound key while the palette is open.
test("an open palette suppresses other shortcuts (no second modal)", async ({ page }) => {
  await gotoApp(page);
  await page.keyboard.press(CHORD);
  await expect(page.locator(palette)).toBeVisible();

  // Move focus off the filter input (as a backdrop click would, falling to
  // <body>), then press the help shortcut — the help overlay must stay closed.
  await page.locator(input).evaluate((el) => (el as HTMLElement).blur());
  await page.keyboard.press("?");
  await expect(page.locator("#help-overlay")).toBeHidden();
  await expect(page.locator(palette)).toBeVisible();
});

test("the chord toggles the palette closed again", async ({ page }) => {
  await gotoApp(page);
  await page.keyboard.press(CHORD);
  await expect(page.locator(palette)).toBeVisible();
  await page.keyboard.press(CHORD);
  await expect(page.locator(palette)).toBeHidden();
});
