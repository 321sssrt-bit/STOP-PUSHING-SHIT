#!/bin/sh
# Developer build on macOS; downloads and installation are intentionally separate.
set -eu
cd "$(dirname "$0")/.."
out=${1:?Pass an output directory}
mkdir -p "$out"
out=$(cd "$out" && pwd)
go_cmd=${SPS_GO:-go}
zig_cmd=${SPS_ZIG:-zig}
export CGO_ENABLED=1 CGO_CFLAGS='-O2 -g0' CGO_LDFLAGS='-O2 -g0'
for target in darwin-arm64 darwin-amd64 linux-arm64 linux-amd64 windows-amd64; do
  export GOOS=${target%-*} GOARCH=${target#*-}
  flags='-s -w -buildid='
  suffix=
  case "$target" in
    darwin-*) export CC=clang ;;
    linux-arm64) export CC="$zig_cmd cc -target aarch64-linux-musl" ;;
    linux-amd64) export CC="$zig_cmd cc -target x86_64-linux-musl" ;;
    windows-amd64) export CC="$zig_cmd cc -target x86_64-windows-gnu"; suffix=.exe ;;
  esac
  case "$GOOS" in
    linux|windows) flags="$flags -linkmode=external -extldflags=-static" ;;
  esac
  "$go_cmd" build -trimpath -ldflags="$flags" -o "$out/guard-$target$suffix" .
  printf 'built %s\n' "$target"
done
