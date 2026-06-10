import { test, expect } from "./fixtures";
import { gotoApp, navTo } from "./helpers";

// Connection page (A1, issue 0068 / ADR-0039): see the live credential + the
// precedence ladder, choose the auth method (persisted to config, applied at
// next launch), and paste a token that lands only in the process env. The demo
// MockClient pins a deterministic authenticated status (bootstrap.demoClient).

test("the Connection page shows the live credential and the precedence ladder", async ({ page }) => {
  await gotoApp(page);
  await navTo(page, "Connection");
  await expect(page.locator("#main h2")).toHaveText("Connection");
  // The demo credential, rendered from the seam's AuthStatus.
  await expect(page.locator("#main")).toContainText("demo-user");
  await expect(page.locator("#main")).toContainText("authenticated");
  // The ladder names the env rungs and the device-flow note is present.
  await expect(page.locator("#main")).toContainText("COPILOT_GITHUB_TOKEN");
  await expect(page.locator("#main")).toContainText("GH_TOKEN");
  await expect(page.locator("#main")).toContainText("device flow");
});

test("choosing a method persists and confirms it applies at next launch", async ({ page }) => {
  await gotoApp(page);
  await navTo(page, "Connection");
  await page.locator('input[name="authMethod"][value="gh"]').check();
  await page.locator("#main form button[type=submit]").click();
  await expect(page.locator("#main")).toContainText("saved");
  await expect(page.locator("#main")).toContainText("next launch");
  // The re-rendered chooser keeps gh selected (read back from config).
  await expect(page.locator('input[name="authMethod"][value="gh"]')).toBeChecked();
});

test("a pasted token is acknowledged but never echoed back", async ({ page }) => {
  await gotoApp(page);
  await navTo(page, "Connection");
  const secret = "ghp_e2e_secret_never_rendered";
  await page.locator('input[name="authMethod"][value="token"]').check();
  await page.fill('input[name="githubTokenEnv"]', "E2E_CONN_TOKEN");
  await page.fill('input[name="pasteToken"]', secret);
  await page.locator("#main form button[type=submit]").click();
  await expect(page.locator("#main")).toContainText("saved");
  await expect(page.locator("#main")).toContainText("this process only");
  // No secret at rest, no secret in the DOM: only the ${VAR} name renders.
  await expect(page.locator("#main")).toContainText("E2E_CONN_TOKEN");
  expect(await page.content()).not.toContain(secret);
});
