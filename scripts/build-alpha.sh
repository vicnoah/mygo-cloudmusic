#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo"
version=$(jq -er '.version | select(contains("-alpha."))' mygo.json)
slug=$(jq -er .slug mygo.json)
output=$(realpath -m "${1:-$repo/build/release/$version}")
mkdir -p "$output"
build=$(mktemp -d "$output/.build.XXXXXX")
trap 'rm -rf -- "$build"' EXIT

GOWORK=off go tool mygo build -sign - -skip-dmg -skip-notarize -o "$build" \
  -platform "darwin/amd64,darwin/arm64,windows/amd64,windows/arm64,linux/amd64,linux/arm64"

assets=()
for arch in amd64 arm64; do
  mac="$slug-$version-darwin-$arch.zip"
  dmg="$slug-$version-$arch.dmg"
  deb="${slug}_${version/-/\~}_${arch}.deb"
  windows="$slug-setup-$version-$arch.exe"
  linux="$slug-$version-linux-$arch.tar.gz"
  (
    cd "$build/darwin-$arch"
    zip -qry "$output/$mac" ./*.app
    bash "$repo/scripts/build-dmg.sh" ./*.app "$output/$dmg"
  )
  cp "$build/windows-$arch/$windows" "$output/"
  cp "$build/linux-$arch/$linux" "$output/"
  cp "$build/linux-$arch/$deb" "$output/"
  assets+=("$mac" "$dmg" "$windows" "$linux" "$deb")
done
(
  cd "$output"
  sha256sum "${assets[@]}" > SHA256SUMS
)
printf '%s\n' "$output"
