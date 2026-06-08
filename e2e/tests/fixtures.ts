import { test as base, expect } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import net from "node:net";
import os from "node:os";
import fs from "node:fs";
import path from "node:path";

// Per-worker server isolation. The demo binary holds a single shared in-memory
// session (one forge/config/store/MockClient event stream), so running the suite
// against ONE server with multiple workers cross-contaminates: a CRUD test in one
// worker mutates the forge another worker reads, and the session-less demo event
// stream misroutes concurrent chats. Instead, each Playwright **worker** gets its
// OWN my-orchestra -demo process on its own port and its own temp config dir —
// the same process-isolation that makes CI sharding safe, applied locally. With
// that, fullyParallel + workers > 1 is safe.

// freePort asks the OS for an unused TCP port (listen on :0, read it back, close).
function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.once("error", reject);
    srv.listen(0, "127.0.0.1", () => {
      const { port } = srv.address() as net.AddressInfo;
      srv.close(() => resolve(port));
    });
  });
}

// waitReady polls the server's index until it answers (or the deadline passes).
async function waitReady(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let lastErr: unknown;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url, { redirect: "manual" });
      if (res.ok) return;
    } catch (err) {
      lastErr = err;
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`demo server at ${url} not ready in ${timeoutMs}ms: ${lastErr}`);
}

type WorkerServer = { url: string; proc: ChildProcess; configDir: string };

export const test = base.extend<{ appURL: string }, { appServer: WorkerServer }>({
  // One demo server per worker, reused across every test the worker runs.
  appServer: [
    async ({}, use) => {
      const port = await freePort();
      const url = `http://127.0.0.1:${port}`;
      const configDir = fs.mkdtempSync(path.join(os.tmpdir(), "mo-e2e-"));
      // process.cwd() is the e2e dir (make e2e runs playwright there); the binary
      // global-setup builds sits one level up under the repo root's bin/.
      const bin = path.resolve(process.cwd(), "..", "bin", "my-orchestra");
      const proc = spawn(
        bin,
        ["-demo", "-addr", `127.0.0.1:${port}`, "-config-dir", configDir],
        { stdio: "ignore" },
      );
      // Reject fast if the process exits early OR fails to spawn (e.g. a missing
      // binary emits 'error', not 'exit'), so a startup failure surfaces
      // immediately instead of after the full waitReady timeout.
      const failed = new Promise<never>((_, reject) => {
        proc.once("exit", (code) => reject(new Error(`demo server exited early (code ${code})`)));
        proc.once("error", (err) => reject(new Error(`demo server failed to start: ${err}`)));
      });
      // The teardown kill below fires 'exit', rejecting `failed` again with nobody
      // awaiting it; swallow that to avoid an unhandled rejection.
      failed.catch(() => {});
      try {
        await Promise.race([waitReady(url, 30_000), failed]);
        await use({ url, proc, configDir });
      } finally {
        proc.kill("SIGKILL");
        fs.rmSync(configDir, { recursive: true, force: true });
      }
    },
    { scope: "worker" },
  ],

  // Point the browser (page.goto("/"), relative requests, the `request` fixture)
  // at this worker's server by overriding the built-in baseURL option.
  baseURL: async ({ appServer }, use) => {
    await use(appServer.url);
  },

  // appURL exposes the same base for the few raw-fetch / newContext tests that
  // can't go through a relative path (SSE streams, cold request contexts).
  appURL: async ({ appServer }, use) => {
    await use(appServer.url);
  },
});

export { expect };
