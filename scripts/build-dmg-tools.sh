#!/usr/bin/env bash
set -euo pipefail

output=$(realpath -m "$1")
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
revision=e18579b391950589b9ed16ee630c54ce4319bb29
curl -fsSL --retry 3 "https://codeload.github.com/mozilla/libdmg-hfsplus/tar.gz/$revision" -o "$work/source.tar.gz"
printf '%s  %s\n' f86ff29e206b1ebe68cd30bb88c2e32aeabd28eb2c0cd18aa09045c5ae3d879c "$work/source.tar.gz" | sha256sum -c -
mkdir "$work/source"
tar -xzf "$work/source.tar.gz" -C "$work/source" --strip-components=1
cmake -S "$work/source" -B "$work/build" -DCMAKE_BUILD_TYPE=Release
cmake --build "$work/build" --parallel 4 --target dmg-bin hfsplus
install -D "$work/build/dmg/dmg" "$output/dmg"
install -D "$work/build/hfs/hfsplus" "$output/hfsplus"
