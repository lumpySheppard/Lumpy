#!/bin/sh
# Builds Lumpy.app (universal: Apple Silicon and Intel) on a Mac with the
# Xcode command line tools and Go installed. Usage: macos/build-app.sh [version]
set -eu
cd "$(dirname "$0")/.."
VERSION="${1:-$(sed -n 's/^var version = "\(.*\)"/\1/p' cmd/lumpy/main.go)}"
OUT=dist
APP="$OUT/Lumpy.app"
rm -rf "$APP" "$OUT/build"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources" "$OUT/build"

for arch in arm64 amd64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$OUT/build/lumpy-$arch" ./cmd/lumpy
done
lipo -create -output "$APP/Contents/Resources/lumpy" "$OUT/build/lumpy-arm64" "$OUT/build/lumpy-amd64"

swiftc -O -target arm64-apple-macos12 -o "$OUT/build/Lumpy-arm64" macos/main.swift
swiftc -O -target x86_64-apple-macos12 -o "$OUT/build/Lumpy-x86_64" macos/main.swift
lipo -create -output "$APP/Contents/MacOS/Lumpy" "$OUT/build/Lumpy-arm64" "$OUT/build/Lumpy-x86_64"

sed "s/VERSION/$VERSION/g" macos/Info.plist > "$APP/Contents/Info.plist"

ICONSET="$OUT/build/Lumpy.iconset"
mkdir -p "$ICONSET"
for s in 16 32 128 256 512; do
  sips -z $s $s macos/icon-1024.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
  sips -z $((s*2)) $((s*2)) macos/icon-1024.png --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/Lumpy.icns"

# Ad-hoc signature: enough to run on this Mac. Downloaded copies need the
# quarantine flag removed (see README) until the app is notarized.
codesign --force --sign - "$APP/Contents/Resources/lumpy"
codesign --force --sign - "$APP"
rm -rf "$OUT/build"
echo "Built $APP ($VERSION)"
