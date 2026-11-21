#!/usr/bin/env bash
set -euo pipefail

tag=$1
shift
test "$#" -gt 0
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
release_url=$(gh release view "$tag" --json apiUrl --jq .apiUrl)
gh api "$release_url" > "$work/release.json"
gh release download "$tag" --pattern SHA256SUMS --dir "$work"

new=()
for package in "$@"; do
  test -s "$package"
  name=$(basename "$package")
  # A retry can finish updating checksums after the installer was already uploaded.
  if ! jq -e --arg name "$name" '.assets | any(.name == $name and .state == "uploaded")' "$work/release.json" >/dev/null; then
    new+=("$package")
  fi
done
if (("${#new[@]}")); then
  gh release upload "$tag" "${new[@]}"
fi
gh api "$release_url" > "$work/release.json"

python3 - "$work" "$@" <<'PY'
import json
import re
import sys
from pathlib import Path

work = Path(sys.argv[1])
expected = {Path(path).name for path in sys.argv[2:]}
manifest = work / "SHA256SUMS"
checksums = {}
for line in manifest.read_text().splitlines():
    digest, name = line.split(maxsplit=1)
    checksums[name] = digest
assets = {asset["name"]: asset for asset in json.loads((work / "release.json").read_text())["assets"]}
for name in expected:
    asset = assets[name]
    digest = (asset.get("digest") or "").removeprefix("sha256:")
    if asset["state"] != "uploaded" or asset["size"] < 1 or not re.fullmatch(r"[0-9a-f]{64}", digest):
        raise SystemExit(f"invalid uploaded installer: {name}")
    if name in checksums and checksums[name] != digest:
        raise SystemExit(f"checksum was changed: {name}")
    checksums[name] = digest
for name, digest in checksums.items():
    uploaded = assets[name].get("digest")
    if uploaded and uploaded != "sha256:" + digest:
        raise SystemExit(f"release digest mismatch: {name}")
manifest.write_text("".join(f"{checksums[name]}  {name}\n" for name in sorted(checksums)))
PY
gh release upload "$tag" --clobber "$work/SHA256SUMS"
