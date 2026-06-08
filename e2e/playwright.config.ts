import { defineConfig, devices } from "@playwright/test";
import os from "node:os";

// The suite runs against the offline demo server (`my-orchestra -demo`), which
// drives a scripted MockClient so every surface — streaming reply, tool
// timeline, inline permission/ask/plan/elicit forms, sub-agent strip, cost
// meter — renders deterministically with no Copilot runtime or network.
//
// Parallelism: global-setup.ts builds the binary once, then each worker launches
// its OWN demo server on its own port (tests/fixtures.ts), so the workers never
// share the demo's single in-memory session. That isolation is what lets the
// suite run fullyParallel — no per-test serial/parallel tagging needed.

// cpus/2 + 1: enough parallelism to cut wall-clock while leaving CPU headroom so
// the latency-budget perf tests still measure the app, not scheduler contention.
const workerCount = Math.floor(os.cpus().length / 2) + 1;

export default defineConfig({
  testDir: "./tests",
  // The demo replies on a scripted ~2s timeline; give assertions headroom.
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: true, // safe: each worker has its own isolated demo server
  workers: workerCount,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI
    ? [["github"], ["html", { open: "never" }], ["list"]]
    : [["html", { open: "never" }], ["list"]],
  globalSetup: "./global-setup.ts",
  use: {
    // baseURL is overridden per worker in tests/fixtures.ts to that worker's
    // server; tests navigate with relative paths (page.goto("/")).
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
  ],
});
