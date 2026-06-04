# Playwright config standards

A good `playwright.config.ts` makes failures debuggable and runs stable. Check these.

## Essentials
```ts
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,          // this repo: one shared demo session — keep it
  workers: 1,                    // ditto; document why so no one "fixes" it
  retries: process.env.CI ? 2 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.BASE_URL ?? 'http://127.0.0.1:8765',
    trace: 'on-first-retry',     // capture a trace only when a test retries — cheap + invaluable
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  webServer: {                   // let Playwright own the demo lifecycle in CI
    command: '../bin/my-orchestra -demo -addr 127.0.0.1:8765',
    url: 'http://127.0.0.1:8765',
    reuseExistingServer: !process.env.CI,
  },
});
```

## Why each setting
- **trace on-first-retry** — full action/network/DOM timeline for exactly the runs that failed, with no
  cost on green runs. The single most useful debugging setting.
- **retries in CI only** — masks nothing locally (you see flakes), absorbs infra blips in CI.
- **screenshot/video on failure** — evidence without bloating green runs.
- **webServer** — Playwright starts/stops the app, so CI and local behave the same and there's no
  "did you start the server?" failure mode.

## Smells to flag
- No trace/reporter configured (failures are un-debuggable).
- `fullyParallel: true` against a shared-state demo (guaranteed flake here).
- Hard-coded `baseURL` with no env override (can't point at a different server).
- Timeouts cranked way up to paper over slow/hard-waited tests (fix the waits instead).
