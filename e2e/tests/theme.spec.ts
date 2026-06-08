import { test, expect } from "./fixtures";
import { gotoApp } from "./helpers";

// Light/dark theme toggle (ADR-0025). The choice is client-only: a topbar button
// flips <html data-theme> and persists it to localStorage; a synchronous <head>
// script restores it before first paint (no flash). With nothing stored the
// attribute stays off so color-scheme:light dark follows the OS preference.

const toggle = ".theme-toggle";

test("the toggle flips data-theme and persists across a reload", async ({ page }) => {
  await gotoApp(page);
  // Fresh context: nothing stored → attribute absent (OS preference drives it).
  expect(await page.locator("html").getAttribute("data-theme")).toBeNull();

  await page.locator(toggle).click();
  const first = await page.locator("html").getAttribute("data-theme");
  expect(first === "light" || first === "dark").toBe(true);

  // Persists across a full reload — the <head> script restores it before paint.
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", first!);

  // Toggling again flips to the other theme, and that also persists.
  await page.locator(toggle).click();
  const second = await page.locator("html").getAttribute("data-theme");
  expect(second).not.toBe(first);
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("data-theme", second!);
});

test("the toggle resolves from the OS preference when nothing is stored", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" });
  await gotoApp(page);
  // Nothing stored → attribute off; the OS (dark) drives color-scheme.
  expect(await page.locator("html").getAttribute("data-theme")).toBeNull();
  // The first toggle flips off the OS default (dark) → light.
  await page.locator(toggle).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
});
