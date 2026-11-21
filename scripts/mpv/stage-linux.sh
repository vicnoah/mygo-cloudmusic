#!/usr/bin/env bash
set -euo pipefail

output=$1
mkdir -p "$output"
entry=$(ldconfig -p | awk '$1 == "libmpv.so.2" && !found++ {print $NF}')
test -n "$entry"
cp -L "$entry" "$output/libmpv.so.2"
dependencies=$(ldd "$entry")
if [[ $dependencies == *'not found'* ]]; then
  printf '%s\n' "$dependencies" >&2
  exit 1
fi
while read -r name path; do
  cp -L "$path" "$output/$name"
done < <(awk '$2 == "=>" && $3 ~ /^\// {print $1,$3}' <<< "$dependencies")

dpkg-query -W -f='${binary:Package}	${Version}	${source:Package}	${source:Version}\n' > "$output/Debian-packages.txt"
for license in /usr/share/doc/*/copyright; do
  printf '\n--- %s ---\n' "$license"
  cat "$license"
done > "$output/Debian-copyrights.txt"
