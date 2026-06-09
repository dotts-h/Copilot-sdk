import { test, expect } from "./fixtures";
import { sel, gotoApp, navTo } from "./helpers";

// The Hooks page (ADR-0031): full in-app CRUD over governance hooks, mirroring
// the MCP/skills/workflow pages, plus the read-only built-in policy and a
// pattern preflight. The mandatory dangerous ruleset is shown but unbypassable.
test.describe("Hooks management", () => {
  test("lists the built-in policy read-only and a user section", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Hooks");
    await expect(page.locator("#main h2")).toContainText("Hooks");
    // The safe-read default and a mandatory dangerous rule are listed and badged.
    await expect(page.locator("#main")).toContainText("read-only tool auto-approved by default");
    await expect(page.locator("#main .badge", { hasText: "built-in" }).first()).toBeVisible();
    await expect(page.locator("#main .badge", { hasText: "unbypassable" }).first()).toBeVisible();
    // A built-in row carries no edit/delete control (read-only).
    const builtinRow = page.locator(`${sel.rows}.builtin`).first();
    await expect(builtinRow).toBeVisible();
    await expect(builtinRow.locator("button.edit")).toHaveCount(0);
    await expect(builtinRow.locator("button.del")).toHaveCount(0);
  });

  test("adds a user hook through the form", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Hooks");
    await page.locator(`#main button.add`).click();
    await expect(page.locator(`#main form[hx-post="/hooks"]`)).toBeVisible();
    const id = `e2e-hook-${Date.now()}`;
    await page.fill(`#main input[name="id"]`, id);
    await page.selectOption(`#main select[name="toolKind"]`, "shell");
    await page.fill(`#main input[name="pattern"]`, "git push --force");
    await page.selectOption(`#main select[name="action"]`, "deny");
    await page.fill(`#main input[name="reason"]`, "no force pushes");
    await page.check(`#main input[name="modes"][value="autopilot"]`);
    await page.locator(`#main button[type=submit]`).click();
    // Back on the list, the new user hook is present with its action badge.
    await expect(page.locator("#main")).toContainText("no force pushes");
    await expect(page.locator(`${sel.rows}`, { hasText: "no force pushes" })).toBeVisible();
  });

  test("the preflight tester reports a glob match against a sample command", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Hooks");
    await page.locator(`#main button.add`).click();
    await expect(page.locator(`#main form[hx-post="/hooks"]`)).toBeVisible();
    await page.fill(`#main input[name="pattern"]`, "rm -rf *");
    await page.fill(`#main input[name="sample"]`, "sudo rm -rf /tmp/x");
    await page.locator(`#main button.preflight-test`).click();
    const result = page.locator("#hook-preflight-result");
    await expect(result).toContainText("glob");
    await expect(result).toContainText("matches");
  });

  test("adds a PostToolUse command hook and previews its command preflight", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Hooks");
    await page.locator(`#main button.add`).click();
    await expect(page.locator(`#main form[hx-post="/hooks"]`)).toBeVisible();
    const id = `e2e-cmd-${Date.now()}`;
    await page.fill(`#main input[name="id"]`, id);
    await page.selectOption(`#main select[name="event"]`, "post-tool-use");
    await page.selectOption(`#main select[name="toolKind"]`, "write");
    await page.fill(`#main input[name="reason"]`, "format after write");
    await page.fill(`#main input[name="command"]`, "gofmt");
    await page.fill(`#main input[name="commandArgs"]`, "-w, ${MISSING_E2E_VAR}");
    // The command preflight shows the resolved command + flags the unset ${VAR};
    // it must NOT execute anything.
    await page.locator(`#main button.command-preflight-test`).click();
    const result = page.locator("#hook-command-preflight-result");
    await expect(result).toContainText("gofmt");
    await expect(result).toContainText("MISSING_E2E_VAR");
    // Save: the post-tool-use command hook round-trips onto the list.
    await page.locator(`#main button[type=submit]`).click();
    await expect(page.locator(`${sel.rows}`, { hasText: "format after write" })).toBeVisible();
  });

  test("a malformed hook re-renders the form with an error", async ({ page }) => {
    await gotoApp(page);
    await navTo(page, "Hooks");
    await page.locator(`#main button.add`).click();
    // An invalid slug id passes the client-side `required` check but fails the
    // server-side Validate → the form re-renders in place with the error.
    await page.fill(`#main input[name="id"]`, "Bad ID");
    await page.fill(`#main input[name="pattern"]`, "x");
    await page.locator(`#main button[type=submit]`).click();
    await expect(page.locator(`#main form[hx-post="/hooks"]`)).toBeVisible();
    await expect(page.locator("#main .error")).toBeVisible();
  });
});
