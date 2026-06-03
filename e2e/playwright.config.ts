import { defineConfig, devices } from "@playwright/test";

// The suite runs against the offline demo server (`my-orchestra -demo`), which
// drives a scripted MockClient so every surface — streaming reply, tool
// timeline, inline permission/ask/plan/elicit forms, sub-agent strip, cost
// meter — renders deterministically with no Copilot runtime or network.
const PORT = Number(process.env.MO_PORT ?? 8799);
const HOST = "127.0.0.1";
export const BASE_URL = `http://${HOST}:${PORT}`;

export default defineConfig({
  testDir: "./tests",
  // The demo replies on a scripted ~2s timeline; give assertions headroom.
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false, // the demo binary is a single shared in-memory session
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI
    ? [["github"], ["html", { open: "never" }], ["list"]]
    : [["html", { open: "never" }], ["list"]],
  use: {
    baseURL: BASE_URL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
  // Build the binary, then launch the deterministic demo server. Playwright
  // waits for the URL to answer before the first test and tears it down after.
  webServer: {
    // Builds the binary (requires `go` on PATH) then launches the demo server.
    // Already running it locally? `reuseExistingServer` skips the rebuild.
    command: `cd .. && go build -o bin/my-orchestra ./cmd/my-orchestra && ./bin/my-orchestra -demo -addr ${HOST}:${PORT}`,
    url: BASE_URL,
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
    stdout: "pipe",
    stderr: "pipe",
  },
});
