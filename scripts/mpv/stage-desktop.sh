#!/usr/bin/env bash
set -euo pipefail

version=$1
platform=$2
output=$3
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
stage="$work/stage"
mkdir -p "$stage"

fetch() {
  curl -fsSL --retry 3 -o "$2" "$1"
  printf '%s  %s\n' "$3" "$2" | sha256sum -c -
}

get_signer() {
  local archive=apple-codesign-0.29.0-x86_64-unknown-linux-musl.tar.gz
  fetch "https://github.com/indygreg/apple-platform-rs/releases/download/apple-codesign/0.29.0/$archive" \
    "$work/$archive" dbe85cedd8ee4217b64e9a0e4c2aef92ab8bcaaa41f20bde99781ff02e600002
  tar -xzf "$work/$archive" -C "$work"
  export RCODESIGN="$work/${archive%.tar.gz}/rcodesign"
}

prepare_windows() {
  local arch=$1 repo=$2 prefix=$3 mpv_sum=$4 vulkan_sum=$5
  local mpv_file="windows-$arch-mpv-2.dll.gz"
  local vulkan_file="$prefix-vulkan-loader-1~1.4.363.0-1-any.pkg.tar.zst"
  fetch "https://github.com/vicnoah/mygo-cloudmusic/releases/download/libmpv-0.38.0/$mpv_file" "$work/$mpv_file" "$mpv_sum"
  fetch "https://repo.msys2.org/mingw/$repo/$vulkan_file" "$work/$vulkan_file" "$vulkan_sum"
  gzip -dc "$work/$mpv_file" > "$stage/mpv-2.dll"
  tar --zstd -xOf "$work/$vulkan_file" "$repo/bin/vulkan-1.dll" > "$stage/vulkan-1.dll"
  tar --zstd -xOf "$work/$vulkan_file" "$repo/share/licenses/vulkan-loader/LICENSE" > "$stage/Vulkan-LICENSE.txt"
}

prepare_mac() {
  local arch=$1 sum=$2
  local archive="Supersonic-0.22.1-mac-$arch.zip"
  fetch "https://github.com/supersonic-app/supersonic/releases/download/v0.22.1/$archive" "$work/$archive" "$sum"
  get_signer
  options=(--source "$work/$archive")
}

options=()
case "$platform" in
  darwin-amd64)
    prepare_mac x64 983be4516e140b0691e47063f803c2e0b01eb886334480548c06a5fb694fc875 ;;
  darwin-arm64)
    prepare_mac arm64 54c6bc05216f428a34fffeb739c982cae9d23ff423c8826e381a2ee6fde6d78a ;;
  windows-amd64)
    prepare_windows amd64 ucrt64 mingw-w64-ucrt-x86_64 \
      433603caf42bd989bf90c6327e70e9b40a0f54b34dc0881668a042d625c634c0 \
      065076fe13d39ab97eea4740596426d5ae741b5044e48e1c9495d04835c92dbe ;;
  windows-arm64)
    prepare_windows arm64 clangarm64 mingw-w64-clang-aarch64 \
      e02eb6b341d4c848c3794a59da6c89f4a494620a4a9f0b4f232e4ac13be1dbdf \
      8b8c7b6ef8e8a3e6baa331900f2e97cbd509ae1ed0723b63377ae62589725750 ;;
  *)
    printf 'Unsupported platform %s\n' "$platform" >&2
    exit 1 ;;
esac
python3 scripts/mpv/package.py "$version" "$platform" "$stage" "$output" "${options[@]}"
