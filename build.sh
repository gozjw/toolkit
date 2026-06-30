#!/bin/bash

# go install github.com/akavel/rsrc@latest

set -e

BUILD_DIR="$HOME/bin"

cd "$(dirname "$0")"
PROJ_DIR="$(pwd)"

if [ -z "$1" ]; then
    TARGETS=$(find tools -mindepth 1 -maxdepth 1 -type d)
else
    TARGETS=$(find tools -mindepth 1 -maxdepth 1 -type d -name "${1}*")
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

    if [ -f "$dir/package.json" ]; then
        echo "构建：tools/$name ..."
        cd $dir
        npm run build
        cd $PROJ_DIR
    fi

    if [ -f "$dir/main.go" ]; then
        echo "构建：tools/$name ..."

        PNG_FILE="$dir/app.png"
        ICON_FILE="$dir/app.ico"
        SYZO_FILE="$dir/resource.syso"

        if [ -f "$PNG_FILE" ]; then
            echo " - png2ico：$PNG_FILE"
            png2ico -i $PNG_FILE
            echo " - rsrc：$ICON_FILE"
            rsrc -ico "$ICON_FILE" -o "$SYZO_FILE"
        fi

        go build -ldflags="-s -w" -trimpath -o "$BUILD_DIR/${name}${SUFFIX}" "./tools/${name}"

        echo "生成：$BUILD_DIR/${name}${SUFFIX}"
        echo

        if [ -f "$SYZO_FILE" ]; then
            rm -f "$SYZO_FILE"
        fi

        if [ -f "$ICON_FILE" ]; then
            rm -f "$ICON_FILE"
        fi
    fi
done
