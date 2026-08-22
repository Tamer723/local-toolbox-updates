#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
DIST="${LOCALTOOLBOX_BUILD_DIR:-$ROOT/build/candidate}"
STAGE="$DIST/stage"
rm -rf "$DIST"
mkdir -p "$STAGE/payload/extension"

# Binary icons are generated locally from reviewed text sources so they never
# need to be committed to pull requests.
"$ROOT/extension/icons/generate-icons.sh"

(cd "$ROOT/helper-src" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$STAGE/payload/local-toolbox-helper.exe" .)
(cd "$ROOT/updater-src" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$STAGE/payload/local-toolbox-updater-v2.exe" .)
cp -a "$ROOT/extension/." "$STAGE/payload/extension/"

export TZ=UTC
find "$STAGE" -exec touch -t 202608210000.00 {} +
(cd "$STAGE" && find payload -type f -print | LC_ALL=C sort | zip -q -X "$DIST/LocalToolbox_update_0.5.0-rc.zip" -@)
(cd "$DIST" && sha256sum LocalToolbox_update_0.5.0-rc.zip > LocalToolbox_update_0.5.0-rc.sha256)
stat -c '%s bytes' "$DIST/LocalToolbox_update_0.5.0-rc.zip"
cat "$DIST/LocalToolbox_update_0.5.0-rc.sha256"
