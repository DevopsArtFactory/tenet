#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 2 || $# -gt 3 || ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: $0 VERSION OUTPUT_DIRECTORY [COMMIT]" >&2
  exit 2
fi
version=$1
output=$2
root=$(git rev-parse --show-toplevel)
commit=$(git -C "$root" rev-parse --verify "${3:-HEAD}^{commit}")
mkdir -p "$output"
output=$(cd "$output" && pwd)
archive="tenet-${version}-source.tar.gz"
for asset in "$archive" SHA256SUMS SOURCE_COMMIT; do
  if [[ -e "$output/$asset" ]]; then
    echo "refusing to overwrite $output/$asset" >&2
    exit 1
  fi
done
staging=$(mktemp -d "$output/.tenet-release.XXXXXX")
trap 'rm -rf "$staging"' EXIT
# Only committed source enters the archive; local logs/configuration stay out.
git -C "$root" archive --format=tar --prefix="tenet-${version}/" "$commit" | gzip -n > "$staging/$archive"
printf '%s\n' "$commit" > "$staging/SOURCE_COMMIT"
(cd "$staging" && shasum -a 256 "$archive" SOURCE_COMMIT > SHA256SUMS)
for asset in "$archive" SOURCE_COMMIT SHA256SUMS; do
  mv "$staging/$asset" "$output/$asset"
done
printf 'Packaged %s at %s in %s\n' "$version" "$commit" "$output"
