# Retro 0003 — get-next stumbled twice before W1 shipped clean

- Date: 2026-06-09
- Scope: the session that ran `/get-next` twice (a mis-pick of 0060, then the
  corrected pick of 0063) and built **W1 / issue 0063** (OKLCH token foundation,
  ADR-0036) end to end. The build itself went clean; the lessons are entirely in
  the **session-start ritual** — the part get-next exists to make deterministic.

## Context

The session opened with `/get-next`. The skill's first step is `scripts/next-issue.sh`;
the run failed (exit 127), the whole ritual was hand-rolled instead, and the pick
landed on the wrong pillar — the user had to re-invoke with explicit args
("and start building the new ui") before the right item (0063) was set up. From
there the build was textbook (ADR first, red→green contrast guard, axe gate held,
close-out folded in). Two distinct process failures are worth recording.

## What worked

- **The base-verification habit caught a real hazard.** The session-mandated branch
  pre-existed at a stale cut (0 ahead / 7 behind a fresh `origin/main`, missing PRs
  #98–#100 — the exact RETROS-0002 failure mode). Resolved via
  `refs/remotes/origin/main` and fast-forwarded before any work. Even hand-rolled,
  the ritual's core check held.
- **The manual reconciliation was accurate.** Re-running the real `next-issue.sh`
  afterwards confirmed the hand-derived facts (open epics, blocked/unblocked sets)
  matched — the INDEX + `depends_on` frontmatter is legible enough to reconcile by
  hand when needed.
- **Once the pick was right, the loop compounded.** ADR-0036 first, failing guard
  first, gates green, docs folded in — and the new contrast guard caught two
  out-of-gamut chroma targets the eyeball never would have.

## What cost too much

- **A 127 was read as "the scripts don't exist" (the big one).** The skill prompt
  *prints its base directory* (`.claude/skills/get-next`), and `next-issue.sh`,
  `start-fresh.sh`, and `reserve-ids.sh` all live in `scripts/` under it. The run
  used the repo root as cwd, got `No such file or directory`, and concluded the
  scripts were missing from the clone — then spent the session re-deriving their
  behavior (epic/child reconciliation, dependency edges, base verification) by
  hand. One `ls` of the stated base directory would have falsified the conclusion
  in two seconds. **Lesson: a failed probe localizes a problem, it doesn't
  diagnose it — verify the premise (paths, cwd) before concluding absence,
  especially when the artifact is referenced by documentation you're holding.**
  → **A1.**
- **The pillar was guessed from session plumbing.** Three epics were open (0050
  billing, 0051 auth, 0062 UI) — exactly the skill's "multiple open epics → ask
  the user, don't guess" case, and the ambiguity was even noticed in-session. It
  was then overridden by inference: the auto-generated session branch name
  (`claude/merge-billing-fidelity-epic-0050-75hdib`) was taken as the user's
  intent, and 0060 was fully set up. The user actually wanted the UI epic; the
  whole first setup round-trip was wasted, and the W1 work now ships on a
  billing-named branch. **Lesson: harness metadata (auto-generated branch names,
  session titles) is plumbing, not product intent — it never substitutes for the
  ask-the-user rule.** → **A2.**

## Action items

- **A1 — make the skill's script paths unambiguous.** ✅ Done this session:
  SKILL.md now states that `scripts/*.sh` resolve against the skill's base
  directory (not the repo root / cwd) and shows the resolved form, so a 127 reads
  as "wrong directory", not "missing script".
- **A2 — codify the plumbing-is-not-intent rule.** ✅ Done this session: the
  skill's "When the pick is ambiguous" section now says explicitly that
  auto-generated session/branch metadata must not break the tie between open
  epics — ask.
- **A3 — keep the recommender authoritative.** When the script *is* unavailable
  (truly broken environment), reconcile by hand but say so and re-run the script
  as soon as the discrepancy is noticed — the hand-rolled path skips
  `start-fresh.sh`'s codified `--expect-sha`/`--require` assertions, which exist
  because RETROS-0002 bit twice. (No doc change; this retro is the record.)
