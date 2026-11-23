#!/usr/bin/env bash
set -euo pipefail

repo=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
if [[ ! -x "$repo/agentsview" || ! -f "$repo/.env" || ! -f "$HOME/.agentsview/config.toml" ]]; then
    echo "Set up the binary, .env, and config.toml using INSTRUCTIONS.md." >&2
    exit 1
fi

(
    unset R2_BUCKET AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_ENDPOINT_URL AWS_REGION
    unset AWS_REQUEST_CHECKSUM_CALCULATION AWS_RESPONSE_CHECKSUM_VALIDATION AWS_SESSION_TOKEN
    unset AGENTSVIEW_S3_ONLY
    while IFS= read -r line || [[ -n "$line" ]]; do
        line=${line%$'\r'}
        [[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
        if [[ ! "$line" =~ ^[[:space:]]*([A-Z][A-Z0-9_]*)[[:space:]]*=[[:space:]]*(.*)$ ]]; then
            echo "Use NAME=value lines from .env.example." >&2
            exit 1
        fi
        name=${BASH_REMATCH[1]}
        value=${BASH_REMATCH[2]}
        case "$name" in
            R2_BUCKET|AWS_ACCESS_KEY_ID|AWS_SECRET_ACCESS_KEY|AWS_ENDPOINT_URL|AWS_REGION|AWS_REQUEST_CHECKSUM_CALCULATION|AWS_RESPONSE_CHECKSUM_VALIDATION|AGENTSVIEW_S3_ONLY|AV_WORKER_URL|AV_ADMIN_SECRET)
                export "$name=${value%"${value##*[![:space:]]}"}"
                ;;
            *)
                echo "Use the variable names in .env.example." >&2
                exit 1
                ;;
        esac
    done < "$repo/.env"
    : "${R2_BUCKET:?Set R2_BUCKET in .env}" "${AWS_ACCESS_KEY_ID:?Set AWS_ACCESS_KEY_ID in .env}"
    : "${AWS_SECRET_ACCESS_KEY:?Set AWS_SECRET_ACCESS_KEY in .env}" "${AWS_ENDPOINT_URL:?Set AWS_ENDPOINT_URL in .env}"
    : "${AWS_REGION:?Set AWS_REGION in .env}"
    : "${AWS_REQUEST_CHECKSUM_CALCULATION:?Set AWS_REQUEST_CHECKSUM_CALCULATION in .env}"
    : "${AWS_RESPONSE_CHECKSUM_VALIDATION:?Set AWS_RESPONSE_CHECKSUM_VALIDATION in .env}"
    if [[ ${AGENTSVIEW_S3_ONLY-} != 1 ]]; then
        echo "Set AGENTSVIEW_S3_ONLY=1 in .env." >&2
        exit 1
    fi
    export AGENTSVIEW_DATA_DIR="$HOME/.agentsview" AGENTSVIEW_NO_DAEMON=1
    export AWS_S3_ENDPOINT="$AWS_ENDPOINT_URL"
    "$repo/agentsview" daemon stop
    "$repo/agentsview" source sync --rescan
    exec "$repo/agentsview" serve --host 127.0.0.1
)
