---
id: 0088
title: "Split server.go — lift the interaction + forge-mutation handlers out of the god file"
status: open
severity: low
group: 0086
depends_on: []
github:
links:
  adr: []
  prs: []
  issues: [0086]
  regression:
assets: []
---

## Summary

`internal/web/server.go` (1006 LOC) mixes the `Server` struct definition,
subscription/broadcast, `handleSend` + budget/leash gating, and ~200 lines of
permission/ask/plan/elicit handlers plus the forge toggle/delete handlers. The send/gating
core belongs on `Server`, but the interaction and forge-mutation handlers are natural
extractions (the split is already partly done — `session.go`, `commands.go`, `budget.go`).

## Why now

Pure cleanup; second-biggest navigation risk after `workflow.go` (0087). No behavior change.

## Touches

- `internal/web/server.go` → move (same `package web`):
  - the perm/ask/plan/elicit handlers → `interactions.go`;
  - the forge toggle/select/delete handlers → `forge_handlers.go`.
- Keep the `Server` struct, subscription/broadcast, `handleSend`, and budget gating in
  `server.go`.
- Regenerate `docs/CODEMAP.md` (`make codemap`).

## Acceptance

- [ ] The interaction + forge-mutation handlers move to focused same-package files; the
      `Server` struct + send/budget core stay; no symbol renamed, no behavior changed.
- [ ] `make lint && make test` (floor 65%) green; CODEMAP regenerated.
- [ ] No ADR.

## Notes

Same package as 0087 (`internal/web`) but disjoint files — parallel-safe apart from the
shared import block; doing 0087 then 0088 in one session avoids any conflict. SemVer **patch**.
</parameter>
