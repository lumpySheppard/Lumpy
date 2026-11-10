#!/bin/bash
# Installs DataMosh.ofx.bundle where DaVinci Resolve looks for OFX plugins.
# Run from the folder that holds DataMosh.ofx.bundle (or its .zip):
#   bash install-macos.sh
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -d DataMosh.ofx.bundle ] && [ -f DataMosh.ofx.bundle.zip ]; then
  ditto -x -k DataMosh.ofx.bundle.zip .
fi
[ -d DataMosh.ofx.bundle ] || { echo "DataMosh.ofx.bundle not found next to this script"; exit 1; }

DEST=/Library/OFX/Plugins
echo "Installing to $DEST (asks for your password)"
sudo mkdir -p "$DEST"
sudo rm -rf "$DEST/DataMosh.ofx.bundle"
sudo ditto DataMosh.ofx.bundle "$DEST/DataMosh.ofx.bundle"
# Downloaded files are quarantined by macOS; Resolve would refuse to load it.
sudo xattr -dr com.apple.quarantine "$DEST/DataMosh.ofx.bundle" 2>/dev/null || true
echo "Done. Restart DaVinci Resolve; the effect is under OpenFX > Lumpy > DataMosh."
