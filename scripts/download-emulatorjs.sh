#!/bin/bash
set -euo pipefail

VERSION="4.2.3"
DEST="emulatorjs"

if [ -d "$DEST/data" ] && [ -f "$DEST/data/loader.js" ]; then
    echo "EmulatorJS already downloaded in $DEST/"
    exit 0
fi

echo "Downloading EmulatorJS v${VERSION}..."
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

curl -L "https://github.com/EmulatorJS/EmulatorJS/releases/download/v${VERSION}/${VERSION}.7z" \
    -o "$TMPDIR/emulatorjs.7z"

echo "Extracting (this may take a moment)..."
7z x "$TMPDIR/emulatorjs.7z" -o"$TMPDIR/extracted" -y > /dev/null

rm -rf "$DEST"
mkdir -p "$DEST"
cp -r "$TMPDIR/extracted"/* "$DEST/"

echo "EmulatorJS v${VERSION} installed to $DEST/"
echo "Contents:"
ls "$DEST/"
