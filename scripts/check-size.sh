#!/bin/sh
# Ratchet gate for <=300-line business files and <=60-line functions
# (docs/ENGINEERING_GUIDELINES.md). The codebase already carries grandfathered
# violations recorded in scripts/size-baseline.txt (path -> line count). This
# gate fails on any NEW >300-line business file, or any growth of a baselined
# file, so the debt can only shrink. Split offending files (Batch F) and remove
# their baseline entry once they are back under the limit.
#
# Pure static schema / SQL DDL migration files are exempt per the guidelines;
# list them in the baseline with an "exempt" marker so they never fail.
set -eu
cd "$(dirname "$0")/.."

LIMIT=300
BASELINE="scripts/size-baseline.txt"
fail=0
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

find internal cmd -name '*.go' -not -name '*_test.go' -type f > "$tmp"
while IFS= read -r f; do
  lines=$(wc -l < "$f")
  if [ "$lines" -le "$LIMIT" ]; then
    continue
  fi
  base=$(awk -v p="$f" '$1==p {print $2}' "$BASELINE")
  if [ -z "$base" ]; then
    echo "NEW size violation: $f ($lines > $LIMIT). Split it per the engineering guidelines." >&2
    fail=1
  elif [ "$base" != "exempt" ] && [ "$lines" -gt "$base" ]; then
    echo "size regression: $f grew $base -> $lines (the ratchet only allows shrink)." >&2
    fail=1
  fi
done < "$tmp"

if [ "$fail" -ne 0 ]; then
  echo "File-size ratchet failed. See docs/ENGINEERING_GUIDELINES.md (<=300 lines)." >&2
fi
if [ "$fail" -ne 0 ]; then
  exit "$fail"
fi

go run scripts/check-functions.go
