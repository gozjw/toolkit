#!/bin/bash

set -e

BUILD_DIR="$HOME/bin"

cd "$(dirname "$0")"

if [ -z "$1" ]; then
    TARGETS=$(find cmd -mindepth 1 -maxdepth 1 -type d)
else
    TARGETS=$(find cmd -mindepth 1 -maxdepth 1 -type d -name "${1}*")
fi

if [ -z "$TARGETS" ]; then
    exit 1
fi

OS=$(uname)

SUFFIX=""
if echo "$OS" | grep -qi "mingw\|msys\|cygwin"; then
    SUFFIX=".exe"
fi

mkdir -p "$BUILD_DIR"

for dir in $TARGETS; do
    name=$(basename "$dir")
    echo "构建：cmd/$name ..."

    ICON_FILE="$dir/favicon.ico"
    SYZO_FILE="$dir/resource.syso"

    if [ -f "$ICON_FILE" ]; then
        echo " - rsrc：$ICON_FILE"
        rsrc -ico "$ICON_FILE" -o "$SYZO_FILE"
    fi

    go build -ldflags "-s -w" -trimpath -o "$BUILD_DIR/${name}${SUFFIX}" "./cmd/${name}"

    echo "生成：$BUILD_DIR/${name}${SUFFIX}"
    echo

    if [ -f "$SYZO_FILE" ]; then
        rm -f "$SYZO_FILE"
    fi
done
