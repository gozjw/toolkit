#!/bin/bash

# go install github.com/akavel/rsrc@latest

set -euo pipefail

BUILD_DIR="${HOME}/bin"
PROJ_DIR="$(cd "$(dirname "${BASH_SOURCE}")" && pwd)"

TARGET_NAME="${1:-}"
if [ -z "$TARGET_NAME" ]; then
    mapfile -t TARGETS < <(find "$PROJ_DIR/tools" -mindepth 1 -maxdepth 1 -type d)
else
    mapfile -t TARGETS < <(find "$PROJ_DIR/tools" -mindepth 1 -maxdepth 1 -type d -name "${TARGET_NAME}*")
fi

if [ ${#TARGETS[@]} -eq 0 ]; then
    exit 1
fi

SUFFIX=""
if [[ "$(uname)" =~ (MINGW|MSYS|CYGWIN) ]]; then
    SUFFIX=".exe"
fi

mkdir -p "$BUILD_DIR"

for dir in "${TARGETS[@]}"; do
    name=$(basename "$dir")
    
    if [ -f "$dir/package.json" ]; then
        (cd "$dir" && npm run build)
         echo "$dir/dist/index.html"
    fi

    if [ -f "$dir/main.go" ]; then
        if [ -f "$dir/fn/package.json" ]; then
            (cd "$dir/fn" && npm run build)
            cp -f "$dir/fn/dist/index.html" "$dir/index.html"
        fi

        PNG_FILE="$dir/app.png"
        ICON_FILE="$dir/app.ico"
        SYZO_FILE="$dir/resource.syso"
        EXE_FILE="$BUILD_DIR/${name}${SUFFIX}"

        if [ -f "$PNG_FILE" ] && [ -n "$SUFFIX" ]; then
            png2ico "$PNG_FILE"
            rsrc -ico "$ICON_FILE" -o "$SYZO_FILE"
        fi

        (
            cd "$PROJ_DIR"
            go build -ldflags="-s -w" -trimpath -o "$EXE_FILE" "./tools/${name}"
        )

        echo "$EXE_FILE"

        rm -f "$SYZO_FILE" "$ICON_FILE"
    fi
done
