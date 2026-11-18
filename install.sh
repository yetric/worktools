#!/usr/bin/env bash
# Build a single Go binary and install user-local symlinks.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFIX="${WORKTOOLS_PREFIX:-$HOME/.local}"
BINARY=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --help|-h)
      echo 'Usage: ./install.sh [--prefix DIRECTORY] [--binary PREBUILT_FILE]'
      echo 'Installs worktools, dev, svc, db, dots, and Zsh completion.'
      exit 0 ;;
    --prefix|--binary)
      [[ $# -ge 2 ]] || { echo "Missing value for $1" >&2; exit 2; }
      if [[ "$1" == --prefix ]]; then PREFIX="$2"; else BINARY="$2"; fi
      shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done
if [[ -n "$BINARY" ]]; then
  [[ -f "$BINARY" && -x "$BINARY" ]] || { echo 'Prebuilt binary must be an executable file' >&2; exit 2; }
  BINARY="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"
fi
names=(worktools dev svc db dots)
destinations=()
sources=()
for name in "${names[@]}"; do
  destinations+=("$PREFIX/bin/$name")
  sources+=("$ROOT/build/worktools")
done
destinations+=("$PREFIX/share/zsh/site-functions/_worktools")
sources+=("$ROOT/completions/_worktools")
# Check all paths before building or linking. Recognize only our previous Python
# entry-point symlinks when migrating an installation from the initial version.
for i in "${!destinations[@]}"; do
  dst="${destinations[$i]}"
  src="${sources[$i]}"
  if [[ -e "$dst" || -L "$dst" ]]; then
    prior="$ROOT/bin/$(basename "$dst")"
    if [[ ! -L "$dst" ]] || { [[ "$(readlink "$dst")" != "$src" ]] && [[ "$(readlink "$dst")" != "$prior" ]]; }; then
      echo "Refusing to replace existing path: $dst" >&2
      exit 1
    fi
  fi
done
mkdir -p "$ROOT/build"
if [[ -z "$BINARY" ]]; then
  command -v go >/dev/null || { echo 'Go 1.23+ is required to build; alternatively pass --binary FILE' >&2; exit 1; }
  (cd "$ROOT" && go build -trimpath -o "$ROOT/build/worktools.new" ./cmd/worktools)
  mv "$ROOT/build/worktools.new" "$ROOT/build/worktools"
elif [[ "$BINARY" != "$ROOT/build/worktools" ]]; then
  cp "$BINARY" "$ROOT/build/worktools.new"
  chmod +x "$ROOT/build/worktools.new"
  mv "$ROOT/build/worktools.new" "$ROOT/build/worktools"
fi
for i in "${!destinations[@]}"; do
  dst="${destinations[$i]}"
  mkdir -p "$(dirname "$dst")"
  ln -sfn "${sources[$i]}" "$dst"
  echo "Linked $dst"
done
echo 'Keep this checkout in place. Add <prefix>/bin to PATH.'
echo 'For Zsh, add <prefix>/share/zsh/site-functions to fpath before compinit.'
