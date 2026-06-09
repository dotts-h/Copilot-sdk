#!/usr/bin/env bash
# reserve-ids.sh --issues N [--adrs M] [--stub] [--for "laneA,laneB,..."]
#
# Parallel lanes collide on MONOTONIC numbering: two lanes that each compute
# "next issue id" off the same base both pick the same number, and two ADRs cut
# in parallel both grab NNNN. This reserves a contiguous block of issue (and
# optionally ADR) numbers UP FRONT so the orchestrator can hand each lane its own.
#
#   --issues N   reserve the next N issue numbers (docs/issues/)
#   --adrs M     reserve the next M adr numbers   (docs/adr/)
#   --for a,b    label the reservations with lane names (printed alongside)
#   --stub       hard-reserve by creating placeholder files (commit them on the
#                shared base BEFORE fanning out, so every lane sees them taken)
#
# Prints the assignment table; with --stub also writes placeholder files.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

n_issues=0; n_adrs=0; stub=0; lanes=""
while [ $# -gt 0 ]; do case "$1" in
  --issues) n_issues="$2"; shift 2;;
  --adrs)   n_adrs="$2";   shift 2;;
  --for)    lanes="$2";    shift 2;;
  --stub)   stub=1;        shift;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done

next_after() { # <dir> -> next zero-padded 4-digit number after the max NNNN-*.md
  local dir="$1" last=0 n
  for f in "$dir"/[0-9][0-9][0-9][0-9]-*.md; do
    [ -e "$f" ] || continue; n=$(basename "$f" | cut -c1-4); n=$((10#$n))
    [ "$n" -gt "$last" ] && last=$n
  done
  printf '%d' $((last+1))
}

IFS=',' read -ra lane_arr <<< "$lanes"
lane_at() { local i="$1"; echo "${lane_arr[$i]:-lane$((i+1))}" | xargs; }

echo "# Reserved id block (cut from $(git rev-parse --short HEAD))"
echo
if [ "$n_issues" -gt 0 ]; then
  start=$(next_after docs/issues)
  echo "## issues"
  for i in $(seq 0 $((n_issues-1))); do
    id=$(printf '%04d' $((start+i))); lane=$(lane_at "$i")
    printf '  %s  -> %s\n' "$id" "$lane"
    if [ "$stub" -eq 1 ]; then
      printf -- '---\nid: %s\nstatus: reserved\nnote: reserved for %s — fill via tracking-issues new-issue.sh\n---\n' \
        "$id" "$lane" > "docs/issues/${id}-reserved.md"
    fi
  done
  echo
fi
if [ "$n_adrs" -gt 0 ]; then
  start=$(next_after docs/adr)
  echo "## adrs"
  for i in $(seq 0 $((n_adrs-1))); do
    id=$(printf '%04d' $((start+i))); lane=$(lane_at "$i")
    printf '  %s  -> %s\n' "$id" "$lane"
    if [ "$stub" -eq 1 ]; then
      printf -- '# %s — RESERVED (%s)\n\nStatus: reserved\n\nPlaceholder so parallel lanes do not reuse this number. Replace with the real ADR.\n' \
        "$id" "$lane" > "docs/adr/${id}-reserved.md"
    fi
  done
  echo
fi
[ "$stub" -eq 1 ] && echo "Stub files written — COMMIT them on the shared base before fanning out, then each lane renames its reserved file to the real slug."
[ "$n_issues" -eq 0 ] && [ "$n_adrs" -eq 0 ] && { echo "nothing requested — pass --issues N and/or --adrs M" >&2; exit 2; }
