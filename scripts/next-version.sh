#!/usr/bin/env bash
# Prints version=vX.Y.Z for the next release from the conventional commit
# messages since the last v* tag. Prints nothing when HEAD is already tagged.
set -euo pipefail

last=$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)
if [[ -n $last ]]; then
  range="$last..HEAD"
else
  range=HEAD
fi
log=$(git log "$range" --format=%B)
if [[ -z $log ]]; then
  exit 0
fi

IFS=. read -r major minor patch <<<"${last#v}"
major=${major:-0} minor=${minor:-0} patch=${patch:-0}

if grep -qE '^[a-z]+(\([^)]*\))?!:|^BREAKING[ -]CHANGE:' <<<"$log"; then
  major=$((major + 1)) minor=0 patch=0
elif grep -qE '^feat(\([^)]*\))?:' <<<"$log"; then
  minor=$((minor + 1)) patch=0
else
  patch=$((patch + 1))
fi
echo "version=v$major.$minor.$patch"
