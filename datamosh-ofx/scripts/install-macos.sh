#!/bin/bash
# Installs EriksTV.ofx.bundle where DaVinci Resolve looks for OFX plugins.
# Run from the folder that holds EriksTV.ofx.bundle (or its .zip):
#   bash install-macos.sh
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -d EriksTV.ofx.bundle ] && [ -f EriksTV.ofx.bundle.zip ]; then
  ditto -x -k EriksTV.ofx.bundle.zip .
fi
[ -d EriksTV.ofx.bundle ] || { echo "EriksTV.ofx.bundle not found next to this script"; exit 1; }

DEST=/Library/OFX/Plugins
echo "Installing to $DEST (asks for your password)"
sudo mkdir -p "$DEST"
# Remove this version and the older one named DataMosh (same plugin).
sudo rm -rf "$DEST/EriksTV.ofx.bundle" "$DEST/DataMosh.ofx.bundle"
sudo ditto EriksTV.ofx.bundle "$DEST/EriksTV.ofx.bundle"
# Downloaded files are quarantined by macOS; Resolve would refuse to load it.
sudo xattr -dr com.apple.quarantine "$DEST/EriksTV.ofx.bundle" 2>/dev/null || true
echo "Done. Restart DaVinci Resolve; the effect is under OpenFX > Lumpy > EriksTV."
