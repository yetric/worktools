#!/usr/bin/env bash
# Install symlinks into a user prefix; never download or install dependencies.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFIX="${WORKTOOLS_PREFIX:-$HOME/.local}"
if [[ "${1:-}" == "--help" ]]; then
  echo 'Usage: ./install.sh [--prefix DIRECTORY]'
  echo 'Installs svc, db, dots, and Zsh completion using symlinks.'
  exit 0
fi
if [[ $# -gt 0 ]]; then
  if [[ $# != 2 || $1 != --prefix ]]; then
    echo 'Usage: ./install.sh [--prefix DIRECTORY]' >&2
    exit 2
  fi
  PREFIX="$2"
fi
python3 -c 'import sys; sys.exit(0 if sys.version_info >= (3, 11) else "Python 3.11+ is required")'

destinations=("$PREFIX/bin/svc" "$PREFIX/bin/db" "$PREFIX/bin/dots" "$PREFIX/share/zsh/site-functions/_worktools")
sources=("$ROOT/bin/svc" "$ROOT/bin/db" "$ROOT/bin/dots" "$ROOT/completions/_worktools")
# Preflight the whole installation to avoid overwriting unrelated commands.
for i in "${!destinations[@]}"; do
  dst="${destinations[$i]}"
  src="${sources[$i]}"
  if [[ -e "$dst" || -L "$dst" ]]; then
    if [[ ! -L "$dst" || "$(readlink "$dst")" != "$src" ]]; then
      echo "Refusing to replace existing path: $dst" >&2
      exit 1
    fi
  fi
done
for i in "${!destinations[@]}"; do
  dst="${destinations[$i]}"
  mkdir -p "$(dirname "$dst")"
  [[ -L "$dst" ]] || ln -s "${sources[$i]}" "$dst"
  echo "Linked $dst"
done
echo 'Keep this checkout in place. Add <prefix>/bin to PATH.'
echo 'For Zsh, add <prefix>/share/zsh/site-functions to fpath before compinit.'
