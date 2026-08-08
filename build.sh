#!/bin/bash

# go install github.com/akavel/rsrc@latest

set -euo pipefail

BUILD_BIN_DIR="/d/a/bin"
BUILD_GUI_DIR="/d/a/gui"
PROJ_DIR="$(cd "$(dirname "${BASH_SOURCE}")" && pwd)"

TARGET_NAME="${1:-}"
if [ -z "$TARGET_NAME" ]; then
  mapfile -t TARGETS < <(find "$PROJ_DIR/tools" -mindepth 1 -maxdepth 1 -type d)
else
  mapfile -t TARGETS < <(find "$PROJ_DIR/tools" -mindepth 1 -maxdepth 1 -type d -name "${TARGET_NAME}*")
fi

if [ ${#TARGETS[@]} -eq 0 ]; then
  echo "错误：未匹配到任何工具目录，前缀=${TARGET_NAME}"
  exit 1
fi

SUFFIX=""
if [[ "$(uname)" =~ (MINGW|MSYS|CYGWIN) ]]; then
  SUFFIX=".exe"
fi

mkdir -p "$BUILD_GUI_DIR"
mkdir -p "$BUILD_BIN_DIR"

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

    BUILD_GUI="0"
    PNG_FILE="$dir/app.png"
    ICON_FILE="$dir/app.ico"
    SYZO_FILE="$dir/resource.syso"
    README_File="$dir/README.md"

    if [[ -f "$README_File" ]]; then
      read -r line < "$README_File"
      if [[ "$line" == *gui* ]]; then
        BUILD_GUI="1"
      fi
    fi

    if [ -f "$PNG_FILE" ] && [ -n "$SUFFIX" ]; then
      png2ico "$PNG_FILE"
      rsrc -ico "$ICON_FILE" -o "$SYZO_FILE"
    fi

    LDFLAGS="-s -w"
    ouputDir="${BUILD_BIN_DIR}"
    if [[ "${BUILD_GUI}" == "1" ]]; then
      LDFLAGS+=" -H windowsgui"
      ouputDir="${BUILD_GUI_DIR}"
    fi
    EXE_FILE="${ouputDir}/${name}${SUFFIX}"

    (
      cd "$PROJ_DIR"
      go build -ldflags="${LDFLAGS}" -trimpath -o "$EXE_FILE" "./tools/${name}"
    )

    echo "输出文件: $EXE_FILE"
    rm -f "$SYZO_FILE"
  fi
done
