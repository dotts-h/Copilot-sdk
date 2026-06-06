import { test, expect } from "@playwright/test";
import { sel, pages, gotoApp, navTo, send } from "./helpers";

// UX-quality checks: the things that make the UI feel right rather than merely
// function — focus management, keyboard operability, responsive layout, clean
// consoles, and visual stability.

test("the composer is autofocused on load and refocused after sending", async ({ page }) => {
  await gotoApp(page);
  await expect(page.locator(sel.prompt)).toBeFocused();

  await send(page, "focus me back");
  // htmx's after-request hook resets the form and refocuses the input.
  await expect(page.locator(sel.prompt)).toBeFocused();
  await expect(page.locator(sel.prompt)).toHaveValue("");
});

// Regression: the composer's after-request handler must reset the input only
// for its own send POST, not for the autocomplete GET that fires on every
// keystroke. Otherwise real char-by-char typing is wiped as you type. (Caught
// by the e2e suite; page.fill() hid it because it does not emit keyup.)
test("composer preserves input while typing character by character", async ({ page }) => {
  await gotoApp(page);
  await page.locator(sel.prompt).click();
  await page.locator(sel.prompt).pressSequentially("a real typed sentence", { delay: 30 });
  await expect(page.locator(sel.prompt)).toHaveValue("a real typed sentence");
});

test("Enter submits the composer from the keyboard", async ({ page }) => {
  await gotoApp(page);
  await page.locator(sel.prompt).fill("sent with enter");
  await page.locator(sel.prompt).press("Enter");
  await expect(page.locator(sel.userTurn).filter({ hasText: "sent with enter" }).last()).toBeVisible();
});

// The composer is a <textarea> (C2 / TECH_DEBT #15): Shift-Enter inserts a newline
// instead of sending, so a multi-line prompt can be composed before submitting.
test("Shift-Enter inserts a newline instead of sending", async ({ page }) => {
  await gotoApp(page);
  const turnsBefore = await page.locator(sel.userTurn).count();
  const box = page.locator(sel.prompt);
  await box.click();
  await box.pressSequentially("first line");
  await box.press("Shift+Enter");
  await box.pressSequentially("second line");
  // The value spans two lines and nothing was sent.
  await expect(box).toHaveValue("first line\nsecond line");
  expect(await page.locator(sel.userTurn).count()).toBe(turnsBefore);
  // A plain Enter then sends the whole multi-line prompt.
  await box.press("Enter");
  await expect(page.locator(sel.userTurn).filter({ hasText: "second line" }).last()).toBeVisible();
});

// REGRESSIONS: a bound keybinding character typed in the textarea is text, not a
// shortcut (the keydown dispatcher ignores INPUT/TEXTAREA/SELECT/contenteditable).
test("typing a bound keybinding char in the composer is text, not an action", async ({ page }) => {
  await gotoApp(page);
  const box = page.locator(sel.prompt);
  await box.click();
  // 'n' is the default newChat binding and 't' the goto-Telemetry binding; typed
  // in the composer they must stay literal and not navigate or reset the chat.
  await box.pressSequentially("note: t and n stay literal", { delay: 20 });
  await expect(box).toHaveValue("note: t and n stay literal");
  await expect(page.locator(sel.composer)).toBeVisible();
});

test("nav links are keyboard reachable and operable", async ({ page }) => {
  await gotoApp(page);
  const telemetry = page.locator(sel.nav, { hasText: /^Telemetry$/ });
  await telemetry.focus();
  await expect(telemetry).toBeFocused();
  await telemetry.press("Enter");
  await expect(page.locator("#main h2")).toContainText(/Telemetry/);
});

// KNOWN-NOISE allowlist. The body-level SSE listeners (sse-connect on <body>)
// outlive the chat page's swap targets (#status, #ctx, #cur, #timeline, …),
// which only exist inside #main on the chat page. If a turn streams while the
// user is on another page, htmx-ext-sse tries to swap into a now-absent target
// and throws "reading 'firstChild' / 'insertBefore'". It is non-fatal (the chat
// re-renders on return) but it is a real finding — see e2e/README.md. We filter
// exactly this signature so the test still fails on any *other* error.
const KNOWN_SSE_NOISE = /(firstChild|insertBefore|removeChild|childNodes)/;
const isUnexpected = (msg: string) => !KNOWN_SSE_NOISE.test(msg);

test("a full chat turn produces no console or page errors (on-page)", async ({ page }) => {
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error") consoleErrors.push(m.text());
  });
  page.on("pageerror", (e) => pageErrors.push(e.message));

  // Stay on the chat page (targets present) for a full streamed turn.
  await gotoApp(page);
  await send(page, "no console noise please");
  await expect(page.locator(sel.agentTurn).last()).toContainText("You said", { timeout: 15_000 });

  expect(pageErrors, `page errors:\n${pageErrors.join("\n")}`).toEqual([]);
  expect(consoleErrors, `console errors:\n${consoleErrors.join("\n")}`).toEqual([]);
});

test("navigating across all pages raises no unexpected errors", async ({ page }) => {
  const errors: string[] = [];
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(m.text());
  });
  page.on("pageerror", (e) => errors.push(e.message));

  await gotoApp(page);
  for (const p of pages) await navTo(page, p.label);
  await navTo(page, "Chat");

  const unexpected = errors.filter(isUnexpected);
  expect(unexpected, `unexpected errors:\n${unexpected.join("\n")}`).toEqual([]);
});

test("no horizontal scroll (no layout overflow) at desktop and mobile widths", async ({ page }) => {
  for (const vp of [
    { width: 1280, height: 800 },
    { width: 768, height: 1024 },
    { width: 375, height: 812 },
  ]) {
    await page.setViewportSize(vp);
    await gotoApp(page);
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    );
    expect(overflow, `horizontal overflow at ${vp.width}px`).toBeLessThanOrEqual(1);
  }
});

test("the composer and nav remain usable on a narrow mobile viewport", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await gotoApp(page);
  await expect(page.locator(sel.prompt)).toBeVisible();
  await expect(page.locator(sel.send)).toBeVisible();
  await navTo(page, "Settings");
  await expect(page.locator("#main h2")).toContainText("Settings");
});

test("destructive delete actions are guarded by a confirm dialog", async ({ page }) => {
  await gotoApp(page);
  await navTo(page, "Skills");
  const del = page.locator(`${sel.rows} .del`).first();
  if ((await del.count()) === 0) test.skip(true, "no deletable rows in the demo forge");
  // Dismiss the confirm: the row must survive (hx-confirm gates the POST).
  page.once("dialog", (d) => {
    expect(d.type()).toBe("confirm");
    d.dismiss();
  });
  const before = await page.locator(sel.rows).count();
  await del.click();
  await expect(page.locator(sel.rows)).toHaveCount(before);
});

test("respects prefers-reduced-motion without breaking the stream", async ({ browser }) => {
  const ctx = await browser.newContext({ reducedMotion: "reduce" });
  const page = await ctx.newPage();
  await gotoApp(page);
  await send(page, "reduced motion turn");
  await expect(page.locator(sel.agentTurn).last()).toContainText("You said", { timeout: 15_000 });
  await ctx.close();
});
