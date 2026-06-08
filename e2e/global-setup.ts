import { execSync } from "node:child_process";
import path from "node:path";

// Build the demo binary ONCE before any worker starts. Each Playwright worker
// then launches its own isolated copy of this binary on its own port (see
// tests/fixtures.ts), so the suite can run fully in parallel without the workers
// contending over a single shared in-memory demo session.
//
// Paths resolve from the e2e dir (process.cwd() — `make e2e` runs playwright from
// there), one level under the repo root that holds ./cmd and ./bin.
export default function globalSetup() {
  const repoRoot = path.resolve(process.cwd(), "..");
  execSync("go build -o bin/my-orchestra ./cmd/my-orchestra", {
    cwd: repoRoot,
    stdio: "inherit",
  });
}
