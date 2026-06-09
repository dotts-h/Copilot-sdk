#!/usr/bin/env bash
# next-issue.sh — read docs/issues/INDEX.md and recommend the next thing to build.
#
# Precedence (see references/picking-the-next-item.md):
#   1) an OPEN leaf issue whose parent epic is OPEN        -> build it
#   2) an OPEN epic with NO child issues yet (children '—')-> break it down, file child #1
#   3) an OPEN epic whose children are ALL closed          -> STALE: close the epic (or it has
#                                                             un-filed follow-ups) — needs a human call
#   4) nothing open                                         -> run a NEXT_FEATURES research pass
#
# Reconciles epic status against child status and flags drift. Prints a ranked
# recommendation to stdout; does not mutate anything.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
exec python3 - "$@" <<'PY'
import re, sys, pathlib

idx = pathlib.Path("docs/issues/INDEX.md")
if not idx.exists():
    print("no docs/issues/INDEX.md — is this the right repo?"); sys.exit(2)
lines = idx.read_text().splitlines()

def rows(after_header_contains):
    """Yield cell-lists for markdown table rows after a header line matching the marker."""
    started = False
    for ln in lines:
        if not started:
            if ln.strip().startswith("|") and after_header_contains in ln:
                started = True
            continue
        if not ln.strip().startswith("|"):
            if started: break
            continue
        cells = [c.strip() for c in ln.strip().strip("|").split("|")]
        if set("".join(cells)) <= set("-: "):  # separator row
            continue
        yield cells

def idnum(cell):
    m = re.search(r"\[(\d+)\]", cell) or re.search(r"(\d+)", cell)
    return m.group(1) if m else cell

# --- parse epics table (| id | title | status | children |)
epics = {}
for c in rows("children"):
    if len(c) < 4: continue
    eid = idnum(c[0])
    kids = [k.strip() for k in re.split(r"[,\s]+", c[-1]) if k.strip() and k.strip() != "—"]
    epics[eid] = {"title": c[1], "status": c[2].lower(), "children": kids}

# --- parse issues table (| id | title | status | severity | group | links |)
# Newer epics are sometimes filed in THIS table (title starts "Epic:") rather
# than the epics table above — recognize them either way.
issues = {}
inline_epics = {}
for c in rows("severity"):
    if len(c) < 5: continue
    iid = idnum(c[0])
    grp = idnum(c[4]) if c[4] and c[4] != "—" else ""
    rec = {"title": c[1], "status": c[2].lower(), "severity": c[3].lower(), "group": grp}
    issues[iid] = rec
    if re.match(r"\s*Epic\b", c[1]) and iid not in epics:
        inline_epics[iid] = {"title": c[1], "status": c[2].lower(), "children": []}

def is_open(s): return s not in ("closed", "done", "shipped", "resolved")

all_epics = {**epics, **inline_epics}
open_epics = {e:v for e,v in all_epics.items() if is_open(v["status"])}
recs, flags = [], []

for eid, ev in sorted(open_epics.items()):
    # children = those listed on the epic ∪ issues whose group back-references it
    kids = sorted(set([k for k in ev["children"] if k in issues] +
                      [i for i, iv in issues.items() if iv["group"] == eid]), key=int)
    ev = {**ev, "children": kids}
    open_kids = [k for k in kids if is_open(issues[k]["status"])]
    if not ev["children"]:
        recs.append((1, eid, f"[2] OPEN epic {eid} has NO child issues yet — break it down and FILE child #1.\n      epic: {ev['title']}\n      -> use the tracking-issues skill: scripts/new-issue.sh \"<first slice>\" --group {eid}"))
    elif open_kids:
        sev_rank = {"critical":0,"high":1,"medium":2,"low":3}
        nxt = sorted(open_kids, key=lambda k: (sev_rank.get(issues[k]["severity"],9), int(k)))[0]
        recs.append((0, nxt, f"[1] BUILD issue {nxt} (epic {eid}, sev {issues[nxt]['severity']}).\n      {issues[nxt]['title']}\n      -> read docs/issues/{nxt}-*.md, then branch + TDD."))
    else:
        flags.append(f"[3] STALE: epic {eid} is OPEN but every child is closed — close the epic, or it has un-filed follow-ups. Human call.\n      epic: {ev['title']}")

print("# Next-item recommendation (from docs/issues/INDEX.md)\n")
if recs:
    for _, _, msg in sorted(recs, key=lambda r: (r[0], int(r[1]))):
        print("  " + msg + "\n")
else:
    print("  [4] No open epics with actionable work. Run a NEXT_FEATURES research pass to seed roadmap vN+1.\n")
if flags:
    print("## Flags (reconcile before picking)\n")
    for f in flags: print("  " + f + "\n")
print("Tip: also read docs/NEXT_FEATURES.md '#Recommended sequencing' — it ranks items the INDEX can't.")
PY
