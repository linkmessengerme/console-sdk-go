#!/usr/bin/env bash
set -euo pipefail
version="${1:?version required}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "Invalid release version" >&2; exit 1; }
tag="v$version"
head="$(git rev-parse HEAD)"
git config user.name 'Link Messenger'
git config user.email 'dev@linkmessenger.me'
if git rev-parse --verify "refs/tags/$tag" >/dev/null 2>&1; then
  test "$(git rev-list -n 1 "$tag")" = "$head" || { echo "Release tag points to another commit" >&2; exit 1; }
else
  TZ=UTC git -c tag.gpgsign=false tag -a "$tag" -m "Release $tag"
  git push origin "refs/tags/$tag"
fi
if ! gh release view "$tag" >/dev/null 2>&1; then
  awk '/^## / { section++; if (section == 2) exit } section == 1 { print }' CHANGELOG.md > "$RUNNER_TEMP/release-notes.md"
  gh release create "$tag" --verify-tag --title "$tag" --notes-file "$RUNNER_TEMP/release-notes.md"
fi
