#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${1:-}"

if [[ ! "$version" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]]; then
  printf 'usage: %s <vMAJOR.MINOR.PATCH>\n' "$0" >&2
  exit 2
fi
version="v${version#v}"

cd "$repo_root"

origin_url="$(git remote get-url origin)"
if [[ ! "$origin_url" =~ github\.com[:/]o11y-one/terraform-provider-o11y(\.git)?$ ]]; then
  printf 'origin must point to o11y-one/terraform-provider-o11y before releasing\n' >&2
  exit 1
fi
if [[ "$(git branch --show-current)" != "main" ]]; then
  printf 'release tags must be created from main\n' >&2
  exit 1
fi
if [[ -n "$(git status --porcelain)" ]]; then
  printf 'release requires a clean worktree\n' >&2
  exit 1
fi

git fetch origin main --tags
if [[ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]]; then
  printf 'local main must exactly match origin/main\n' >&2
  exit 1
fi
if git rev-parse --verify --quiet "refs/tags/$version" >/dev/null; then
  printf 'tag %s already exists\n' "$version" >&2
  exit 1
fi

go mod verify
go test ./... -count=1
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
./scripts/generate-docs.sh
git diff --exit-code -- docs

git tag -a "$version" -m "Release $version"
git push origin "$version"
printf 'release workflow started for %s\n' "$version"
