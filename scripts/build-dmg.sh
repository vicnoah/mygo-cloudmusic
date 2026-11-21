#!/usr/bin/env bash
set -euo pipefail

app=$(realpath "$1")
dest=$(realpath -m "$2")
bundle=$(basename "$app")
volume=${bundle%.app}
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
test -f "$app/Contents/Info.plist"
executable=$(python3 -c 'import plistlib, sys; print(plistlib.load(open(sys.argv[1], "rb"))["CFBundleExecutable"])' "$app/Contents/Info.plist")
test -x "$app/Contents/MacOS/$executable"

mkdir "$work/source"
cp -a -- "$app" "$work/source/"
ln -s /Applications "$work/source/Applications"
size=$(du -sm --apparent-size "$work/source" | cut -f1)
truncate -s "$((size + 32))M" "$work/volume.hfs"
mkfs.hfsplus -s -v "$volume" "$work/volume.hfs"
hfsplus --symlinks clone_link --special-modes no "$work/volume.hfs" addall "$work/source"
fsck.hfsplus -fn "$work/volume.hfs"
dmg -c zlib build "$work/volume.hfs" "$work/image.dmg" > "$work/dmg.log" || {
  cat "$work/dmg.log" >&2
  exit 1
}
7z t "$work/image.dmg" >/dev/null
7z x -y "-o$work/extracted" '-xr!Applications' "$work/image.dmg" >/dev/null
extracted="$work/extracted/$volume/$bundle"
test -x "$extracted/Contents/MacOS/$executable"
diff -qr --no-dereference "$app" "$extracted"
test "$(7z x -so "$work/image.dmg" "$volume/Applications")" = /Applications
mkdir -p "$(dirname "$dest")"
mv -- "$work/image.dmg" "$dest"
