#!/bin/sh
# Builds Lumpy for iPad: the engine as Mobile.xcframework (gomobile),
# then Lumpy.xcodeproj (XcodeGen). Needs Xcode, Go, and:
#   go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
#   brew install xcodegen
# Then open ios/Lumpy.xcodeproj, choose your team under Signing &
# Capabilities, and run on the iPad. With a free Apple ID the app has to
# be run from Xcode again every 7 days; a paid developer account lifts
# that and allows TestFlight. Pass --archive to also build an unsigned
# archive (what CI does, to prove it compiles).
set -eu
cd "$(dirname "$0")/.."
VERSION="$(sed -n 's/^var version = "\(.*\)"/\1/p' cmd/lumpy/main.go)"
export PATH="$PATH:$(go env GOPATH)/bin"
rm -rf ios/Mobile.xcframework
gomobile bind -target ios,iossimulator -iosversion 16.0 -ldflags "-X github.com/lumpysheppard/lumpy/mobile.Version=$VERSION" -o ios/Mobile.xcframework ./mobile
sed -i '' "s/MARKETING_VERSION: \".*\"/MARKETING_VERSION: \"$VERSION\"/" ios/project.yml
(cd ios && xcodegen generate)
if [ "${1:-}" = "--archive" ]; then
  xcodebuild -project ios/Lumpy.xcodeproj -scheme Lumpy -sdk iphoneos -configuration Release \
    CODE_SIGNING_ALLOWED=NO CODE_SIGNING_REQUIRED=NO -derivedDataPath ios/build build | tail -5
fi
echo "Built ios/Lumpy.xcodeproj ($VERSION)"
