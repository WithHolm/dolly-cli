#!/usr/bin/env bash
# Tag the latest local commit (HEAD) with the next semver tag.
# Asks for major, minor, or patch. Does not push: it prints the command.
set -euo pipefail

git fetch --tags --quiet origin
target=$(git rev-parse HEAD)

last=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -vE -- '-' | head -n1 || true)
last=${last:-v0.0.0}
IFS=. read -r major minor patch <<<"${last#v}"
echo "Latest tag: $last"

read -rp "Bump major, minor, or patch? [major/minor/patch] " kind
case "$kind" in
  major) next="v$((major + 1)).0.0" ;;
  minor) next="v$major.$((minor + 1)).0" ;;
  patch) next="v$major.$minor.$((patch + 1))" ;;
  *) echo "Answer major, minor, or patch." >&2; exit 1 ;;
esac

echo "Commit: $(git log -1 --format='%h %s' "$target")"
if ! git branch -r --contains "$target" | grep -q .; then
  echo "Warning: this commit is not pushed. Pushing the tag sends it, but no branch will have it."
fi
read -rp "Tag it $next? [y/N] " ok
[[ "$ok" == [yY] ]] || { echo "No tag made."; exit 1; }

git tag -a "$next" -m "$next" "$target"
echo "Tagged $next. Push it (this starts the release):"
echo "  git push origin $next"
