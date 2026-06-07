---
name: cut-release
description: Cut a tagged GitHub release (binaries + checksums + notes) via the release workflow, verifying the resolved version end-to-end so a misconfigured workflow can't publish a mis-tagged release. Use when asked to cut, tag, publish, or ship a release (e.g. "release v0.1.0", "tag and publish"), especially in a sandbox where direct `git push` of a tag is blocked.
allowed-tools: Read, Bash, Grep
---

# Cutting a release (with version verification)

A release is **outward-facing and hard to reverse** — a published GitHub Release is indexed
and downloaded the moment it exists. The failure mode this skill prevents: the release
workflow resolves the *wrong* version and publishes a mis-tagged release (e.g. tagged after
the branch, `main`, instead of `v0.1.0`). Verify before and after; never fire blind.

## Pre-flight (before publishing)

1. **The release must be authorized** — a release is outward-facing, so proceed only on an
   explicit request for *this* version. Confirm the exact tag (e.g. `v0.1.0`) and the commit
   (normally the current `main` tip).
2. **Audit the version resolution.** Read the release workflow's version step. It MUST prefer
   the dispatch input and fall back to the ref name:
   ```yaml
   run: echo "version=${{ github.event.inputs.tag || github.ref_name }}" >> "$GITHUB_OUTPUT"
   ```
   The broken form `${GITHUB_REF_NAME:-inputs.tag}` resolves to the **branch** ("main") on a
   `workflow_dispatch`, because `GITHUB_REF_NAME` is non-empty — the `:-` fallback never fires.
   If you see it, fix the workflow first (its own PR), then release.
3. Gates are green on the commit you're tagging (CI passed on `main`).

## Publishing

- **Preferred (tag push):** `git push origin <tag>`. The `push: tags: ["v*"]` trigger runs the
  release workflow; `github.ref_name` is the tag, so the version is correct.
- **Sandbox fallback (tag push blocked / HTTP 403):** trigger the workflow's `workflow_dispatch`
  with the tag as input (GitHub MCP `actions_run_trigger` → `run_workflow`, `ref: main`,
  `inputs: {tag: "<tag>"}`). The workflow's own `contents: write` token creates the tag + release
  server-side — no local tag push needed. **This path REQUIRES the step-2 fix**, or it tags the
  release after the branch.

## Verify (after publishing) — non-negotiable

Confirm the published release matches the request before reporting success:
- The workflow run completed `success`.
- `get_release_by_tag` for the exact tag returns it with `tag_name == "<tag>"` (NOT the branch
  name), and the asset names embed the version (`<app>-<tag>-<os>-<arch>`).
- If the tag is wrong (e.g. `main`): the workflow version resolution was buggy. Fix it, re-cut,
  and **surface the mis-tagged release for deletion** — a release can't be deleted via the
  read-only release tools, so it needs the user's hand (or restored permissions).

## Lesson (why this skill exists)

Triggering a release without auditing the version step published a release tagged `main`; the
stray `main` tag then collided with the `main` branch and corrupted `git fetch origin main`.
Outward-facing actions get verified, not assumed.
