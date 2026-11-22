#!/usr/bin/env bash
set -euo pipefail

args=()
if [[ ${1-} == --clean ]]; then
    args+=(-Clean)
    shift
fi
if (( $# )); then
    echo "Usage: bash scripts/r2-sync.sh [--clean]" >&2
    exit 2
fi
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$(cygpath -w "$script_dir/r2-sync.ps1")" "${args[@]}"
