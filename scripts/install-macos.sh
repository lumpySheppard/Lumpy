#!/bin/bash
# Installs the OFX plugin(s) in this folder where DaVinci Resolve looks for
# them. Run from the unzipped download:
#   bash install-macos.sh
set -euo pipefail
cd "$(dirname "$0")"

for z in *.ofx.bundle.zip; do
  [ -e "$z" ] || continue
  [ -d "${z%.zip}" ] || ditto -x -k "$z" .
done
shopt -s nullglob
bundles=(*.ofx.bundle)
[ ${#bundles[@]} -gt 0 ] || { echo "No .ofx.bundle found next to this script"; exit 1; }

DEST=/Library/OFX/Plugins
echo "Installing to $DEST (asks for your password)"
sudo mkdir -p "$DEST"
for b in "${bundles[@]}"; do
  sudo rm -rf "$DEST/$b"
  # EriksTV used to be called DataMosh; remove the old copy so Resolve
  # doesn't load the same plugin twice.
  [ "$b" = "EriksTV.ofx.bundle" ] && sudo rm -rf "$DEST/DataMosh.ofx.bundle"
  sudo ditto "$b" "$DEST/$b"
  # Downloaded files are quarantined by macOS; Resolve would refuse to load them.
  sudo xattr -dr com.apple.quarantine "$DEST/$b" 2>/dev/null || true
  echo "  installed ${b%.ofx.bundle}"
done
echo "Done. Restart DaVinci Resolve; the effects are under OpenFX > Lumpy."
