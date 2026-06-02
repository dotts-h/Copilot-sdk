#!/usr/bin/env node
// my-orchestra sidecar: bridges the Go TUI to the GitHub Copilot SDK.
//
// Protocol (line-delimited JSON over stdio), mirroring internal/copilot:
//   Go -> us:   {"id":N,"method":"createSession|send|abort","params":{...}}
//   us -> Go:   {"id":N,"result":{...}} | {"id":N,"error":"..."}
//   us -> Go:   {"event":"<name>","data":{...}}   (unsolicited)
//
// The sidecar is intentionally thin: all agentic behavior lives in the SDK. It
// translates SDK events into the wire events the Go telemetry/chat layers
// expect (notably the synthetic "usage" event used for credit accounting).
//
// If @github/copilot-sdk is not installed (offline/dev), the sidecar falls back
// to an "echo" engine so the TUI remains demonstrable end-to-end.

import readline from "node:readline";

const out = process.stdout;
function write(obj) {
  out.write(JSON.stringify(obj) + "\n");
}
function reply(id, result) {
  write({ id, result });
}
function fail(id, error) {
  write({ id, error: String(error && error.message ? error.message : error) });
}
function emit(event, data) {
  write({ event, data });
}

// ---- Engine abstraction -------------------------------------------------

// Attempt to load the real SDK; gracefully degrade to an echo engine.
async function loadEngine() {
  try {
    const sdk = await import("@github/copilot-sdk");
    return new CopilotEngine(sdk);
  } catch (err) {
    process.stderr.write(
      `[sidecar] @github/copilot-sdk unavailable (${err.message}); using echo engine.\n`
    );
    return new EchoEngine();
  }
}

class CopilotEngine {
  constructor(sdk) {
    this.sdk = sdk;
    this.client = null;
    this.sessions = new Map();
  }
  async start() {
    const { CopilotClient } = this.sdk;
    this.client = new CopilotClient({
      mode: "copilot-cli",
      logLevel: "error",
      gitHubToken: process.env.GITHUB_TOKEN,
    });
    await this.client.start();
  }
  async createSession(spec) {
    const opts = {
      streaming: spec.streaming !== false,
    };
    if (spec.model) opts.model = spec.model;
    if (spec.reasoningEffort) opts.reasoningEffort = spec.reasoningEffort;
    if (spec.systemMessage) {
      opts.systemMessage = { mode: "append", content: spec.systemMessage };
    }
    if (spec.autoApproveTools) {
      const { approveAll } = this.sdk;
      opts.onPermissionRequest = approveAll;
    }
    const session = await this.client.createSession(opts);
    this._wire(session);
    this.sessions.set(session.sessionId, session);
    return session.sessionId;
  }
  _wire(session) {
    const sid = session.sessionId;
    session.on("assistant.message_delta", (e) =>
      emit("assistant.message_delta", { sessionId: sid, deltaContent: e.data.deltaContent })
    );
    session.on("assistant.reasoning_delta", (e) =>
      emit("assistant.reasoning_delta", { sessionId: sid, deltaContent: e.data.deltaContent })
    );
    session.on("assistant.message", (e) =>
      emit("assistant.message", { sessionId: sid, content: e.data.content })
    );
    session.on("tool.execution_start", (e) =>
      emit("tool.execution_start", { sessionId: sid, toolName: e.data?.toolName, status: "start" })
    );
    session.on("tool.execution_complete", (e) =>
      emit("tool.execution_complete", { sessionId: sid, toolName: e.data?.toolName, status: "complete" })
    );
    // Token accounting: the SDK surfaces usage on message/idle events depending
    // on version; forward whatever shape we can find as our normalized event.
    const forwardUsage = (e) => {
      const u = e?.data?.usage || e?.data?.tokenUsage;
      if (!u) return;
      emit("usage", {
        sessionId: sid,
        model: e?.data?.model || session.model || "",
        inputTokens: u.inputTokens ?? u.promptTokens ?? 0,
        cachedTokens: u.cachedTokens ?? u.cachedInputTokens ?? 0,
        outputTokens: u.outputTokens ?? u.completionTokens ?? 0,
      });
    };
    session.on("assistant.message", forwardUsage);
    session.on("session.idle", () => emit("session.idle", { sessionId: sid }));
  }
  async send(sessionId, prompt) {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`unknown session ${sessionId}`);
    return await session.send({ prompt });
  }
  async abort(sessionId) {
    const session = this.sessions.get(sessionId);
    if (session) await session.abort();
  }
}

// EchoEngine keeps the TUI fully functional with no network/SDK, and emits a
// representative usage event so credit telemetry is demonstrable.
class EchoEngine {
  async start() {}
  async createSession() {
    return "echo-" + Math.random().toString(36).slice(2, 8);
  }
  async send(sessionId, prompt) {
    const reply = `echo: ${prompt}`;
    for (const word of reply.split(" ")) {
      emit("assistant.message_delta", { sessionId, deltaContent: word + " " });
    }
    emit("assistant.message", { sessionId, content: reply });
    emit("usage", {
      sessionId,
      model: "gpt-5",
      inputTokens: prompt.length,
      cachedTokens: 0,
      outputTokens: reply.length,
    });
    emit("session.idle", { sessionId });
  }
  async abort() {}
}

// ---- Dispatch loop ------------------------------------------------------

async function main() {
  const engine = await loadEngine();
  try {
    await engine.start();
  } catch (err) {
    process.stderr.write(`[sidecar] engine start failed: ${err.message}\n`);
  }

  const rl = readline.createInterface({ input: process.stdin });
  rl.on("line", async (line) => {
    line = line.trim();
    if (!line) return;
    let req;
    try {
      req = JSON.parse(line);
    } catch {
      return;
    }
    try {
      switch (req.method) {
        case "createSession": {
          const id = await engine.createSession(req.params || {});
          reply(req.id, { sessionId: id });
          break;
        }
        case "send": {
          await engine.send(req.params.sessionId, req.params.prompt);
          reply(req.id, { ok: true });
          break;
        }
        case "abort": {
          await engine.abort(req.params.sessionId);
          reply(req.id, { ok: true });
          break;
        }
        default:
          fail(req.id, `unknown method ${req.method}`);
      }
    } catch (err) {
      fail(req.id, err);
    }
  });
  rl.on("close", () => process.exit(0));
}

main().catch((err) => {
  process.stderr.write(`[sidecar] fatal: ${err.stack || err}\n`);
  process.exit(1);
});
