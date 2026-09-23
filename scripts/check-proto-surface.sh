#!/usr/bin/env bash
# Fails on any vendored proto or generated Go file outside alerts/v1 and the
# o11y_one/common files alerts/v1 imports; importing one is how the surface widens.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

alerts=proto/o11y_one/alerts/v1
common="$(grep -rhE '^[[:space:]]*import[[:space:]]' --include='*.proto' "$alerts" | grep -oE 'o11y_one/common/[^"]+\.proto' | sed 's|^|proto/|' | sort -u || true)"
common_dirs="$(sed 's|/[^/]*$||' <<<"$common")"

allowed() {
  case "$1" in
    "$alerts"/*) ;;
    *.proto) grep -qxF "$1" <<<"$common" ;;
    *) grep -qxF "$(dirname "$1")" <<<"$common_dirs" ;;
  esac
}

bad=0
while IFS= read -r f; do
  allowed "${f#internal/gen/}" || { printf 'outside the alerts-only surface: %s\n' "$f" >&2; bad=1; }
done < <(find proto/o11y_one -name '*.proto'; find internal/gen -type f)

[[ $bad -eq 0 ]] && echo 'proto surface: alerts-only'
exit "$bad"
