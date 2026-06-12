#!/usr/bin/env bash
# install-recipe.sh — render one recipe's templates into a target repo and
# record it in the target's .recipes/lock.json.
#
#   install-recipe.sh --recipe <recipe-dir> --target <repo-dir> --tier S|M|L \
#                     [--answer key=value]... [--force] [--dry-run]
#
# Contract (see docs/SPEC.md in the cookbook repo):
#   - Reads <recipe-dir>/recipe.yaml: params (defaults), provides.files
#     (template -> dest, tiers, optional `when: param=value` condition).
#   - Renders {{param}} placeholders from defaults merged with --answer pairs.
#   - NEVER clobbers: an existing dest file is skipped (reported as SKIP)
#     unless --force is passed. The repo's copy is canonical-for-that-repo;
#     divergence is the doctor's business, not the installer's.
#   - Writes/updates .recipes/lock.json: {recipes: [{name, version, tier,
#     answers, installedAt}]}, replacing any prior entry for the same recipe.
#   - Exit 0 on success, 2 on usage/manifest error.
#
# Dependency-light: bash + python3 (stdlib; PyYAML used when present, with a
# built-in fallback parser for the strict manifest subset). No jq.
set -euo pipefail

recipe="" target="" tier="" force=0 dry=0
answers=""
while [ $# -gt 0 ]; do case "$1" in
  --recipe) recipe="$2"; shift 2;;
  --target) target="$2"; shift 2;;
  --tier)   tier="$2"; shift 2;;
  --answer) answers="${answers}${2}"$'\n'; shift 2;;
  --force)  force=1; shift;;
  --dry-run) dry=1; shift;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done

[ -n "$recipe" ] && [ -n "$target" ] && [ -n "$tier" ] || {
  echo "usage: install-recipe.sh --recipe <dir> --target <dir> --tier S|M|L [--answer k=v]... [--force]" >&2
  exit 2
}
case "$tier" in S|M|L) ;; *) echo "tier must be S, M or L (got '$tier')" >&2; exit 2;; esac
[ -f "$recipe/recipe.yaml" ] || { echo "no recipe.yaml under $recipe" >&2; exit 2; }
[ -d "$target" ] || { echo "target dir not found: $target" >&2; exit 2; }

RECIPE_DIR="$(cd "$recipe" && pwd)" TARGET_DIR="$(cd "$target" && pwd)" \
TIER="$tier" ANSWERS="$answers" FORCE="$force" DRY="$dry" python3 - <<'PY'
import json, os, re, sys, subprocess

recipe_dir = os.environ["RECIPE_DIR"]
target_dir = os.environ["TARGET_DIR"]
tier = os.environ["TIER"]
force = os.environ["FORCE"] == "1"
dry = os.environ["DRY"] == "1"

# ---- parse recipe.yaml (PyYAML when present; strict-subset fallback) -------
manifest_path = os.path.join(recipe_dir, "recipe.yaml")
text = open(manifest_path).read()

def parse_subset(text):
    """Parse the strict YAML subset used by recipe manifests: nested maps via
    2-space indents, inline lists [a, b], block lists of scalars (- x) and of
    maps (- key: val). Values are plain scalars (no quotes needed/parsed)."""
    root = {}

    def parse_scalar(s):
        s = s.strip()
        if s.startswith("[") and s.endswith("]"):
            inner = s[1:-1].strip()
            return [v.strip() for v in inner.split(",") if v.strip()] if inner else []
        if s in ("", "~", "null"): return None
        if s.strip('"') != s: return s.strip('"')
        return s

    lines = [l for l in text.splitlines()
             if l.strip() and not l.lstrip().startswith("#")]
    # strip trailing comments that follow a value (conservative: ' #')
    lines = [re.sub(r"\s+#.*$", "", l) for l in lines]

    def block(lines, i, indent):
        """Parse a mapping block at `indent`; return (obj, next_i)."""
        obj = {}
        while i < len(lines):
            line = lines[i]
            cur = len(line) - len(line.lstrip())
            if cur < indent: break
            if cur > indent:  # shouldn't happen for a well-formed subset
                i += 1; continue
            s = line.strip()
            if s.startswith("- "):  # list under previous key handled by caller
                break
            m = re.match(r"([A-Za-z_][\w-]*):\s*(.*)$", s)
            if not m:
                i += 1; continue
            key, val = m.group(1), m.group(2)
            if val != "":
                obj[key] = parse_scalar(val); i += 1; continue
            # nested: mapping, list of scalars, or list of maps
            i += 1
            if i < len(lines):
                nxt = lines[i]; nind = len(nxt) - len(nxt.lstrip())
                if nind > indent and nxt.strip().startswith("- "):
                    items, i = parse_list(lines, i, nind)
                    obj[key] = items; continue
                if nind > indent:
                    sub, i = block(lines, i, nind)
                    obj[key] = sub; continue
            obj[key] = None
        return obj, i

    def parse_list(lines, i, indent):
        items = []
        while i < len(lines):
            line = lines[i]
            cur = len(line) - len(line.lstrip())
            if cur != indent or not line.strip().startswith("- "): break
            head = line.strip()[2:]
            m = re.match(r"([A-Za-z_][\w-]*):\s*(.*)$", head)
            if m:  # list of maps: first pair inline, rest indented deeper
                item = {m.group(1): parse_scalar(m.group(2))}
                i += 1
                if i < len(lines):
                    nxt = lines[i]; nind = len(nxt) - len(nxt.lstrip())
                    if nind > indent and not nxt.strip().startswith("- "):
                        sub, i = block(lines, i, nind)
                        item.update(sub)
                items.append(item)
            else:
                items.append(parse_scalar(head)); i += 1
        return items, i

    obj, _ = block(lines, 0, 0)
    return obj

try:
    import yaml
    manifest = yaml.safe_load(text)
except ImportError:
    manifest = parse_subset(text)

name = manifest.get("name")
version = str(manifest.get("version", "0.0.0"))
if not name:
    print(f"ERROR: {manifest_path} has no `name`", file=sys.stderr); sys.exit(2)

# ---- merge answers: defaults from manifest params, overridden by --answer --
params = ((manifest.get("requires") or {}).get("params")) or []
answers = {}
for p in params:
    if isinstance(p, dict) and p.get("name") is not None:
        answers[str(p["name"])] = "" if p.get("default") is None else str(p["default"])
for line in os.environ["ANSWERS"].splitlines():
    line = line.strip()
    if not line or "=" not in line: continue
    k, v = line.split("=", 1)
    answers[k.strip()] = v.strip()

# ---- select files for this tier (+ `when` condition), render, write --------
files = ((manifest.get("provides") or {}).get("files")) or []
placeholder = re.compile(r"\{\{\s*([A-Za-z_][\w]*)\s*\}\}")
written, skipped = [], []

for f in files:
    tiers = [str(t) for t in (f.get("tiers") or ["S", "M", "L"])]
    if tier not in tiers: continue
    when = f.get("when")
    if when:
        k, _, v = str(when).partition("=")
        if answers.get(k.strip(), "") != v.strip(): continue
    src = os.path.join(recipe_dir, f["template"])
    dest = os.path.join(target_dir, f["dest"])
    body = open(src).read()
    unknown = []
    def sub(m):
        key = m.group(1)
        if key in answers: return answers[key]
        unknown.append(key); return m.group(0)
    rendered = placeholder.sub(sub, body)
    if unknown:
        print(f"ERROR: {f['template']}: unbound placeholders {sorted(set(unknown))} "
              f"(declare them under requires.params or pass --answer)", file=sys.stderr)
        sys.exit(2)
    if os.path.exists(dest) and not force:
        skipped.append(f["dest"]); continue
    if not dry:
        os.makedirs(os.path.dirname(dest) or ".", exist_ok=True)
        with open(dest, "w") as out: out.write(rendered)
        if dest.endswith(".sh"): os.chmod(dest, 0o755)
    written.append(f["dest"])

# ---- update .recipes/lock.json ---------------------------------------------
lock_path = os.path.join(target_dir, ".recipes", "lock.json")
lock = {"recipes": []}
if os.path.exists(lock_path):
    try:
        lock = json.load(open(lock_path))
    except Exception:
        print(f"WARN: unreadable {lock_path}; rewriting", file=sys.stderr)
        lock = {"recipes": []}
lock.setdefault("recipes", [])
lock["recipes"] = [r for r in lock["recipes"] if r.get("name") != name]
now = subprocess.run(["date", "-u", "+%Y-%m-%dT%H:%M:%SZ"],
                     capture_output=True, text=True).stdout.strip()
lock["recipes"].append({"name": name, "version": version, "tier": tier,
                        "answers": answers, "installedAt": now})
lock["recipes"].sort(key=lambda r: r["name"])
if not dry:
    os.makedirs(os.path.dirname(lock_path), exist_ok=True)
    with open(lock_path, "w") as out:
        json.dump(lock, out, indent=2); out.write("\n")

# ---- report ------------------------------------------------------------------
mode = " (dry-run)" if dry else ""
print(f"installed recipe '{name}' v{version} tier {tier} into {target_dir}{mode}")
for d in written: print(f"  WROTE {d}")
for d in skipped: print(f"  SKIP  {d} (exists; repo copy is canonical — use --force to overwrite)")
print(f"  LOCK  .recipes/lock.json")
PY
