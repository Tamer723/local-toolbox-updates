#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHUNKS="$ROOT/sources/0.4.0/source.b64.*"
ZIP="${TMPDIR:-/tmp}/localtoolbox-0.4.0-source.zip"
TARGET="$ROOT/product"
EXPECTED_SHA="9bfdcacfc99d6f8a17d9d4cd1b1a35b53450b81ea4e48a725c0155d850606aae"

if compgen -G "$CHUNKS" > /dev/null; then
  :
else
  echo "Missing source chunks: sources/0.4.0/source.b64.*" >&2
  exit 1
fi

if [[ -d "$TARGET" ]] && find "$TARGET" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
  echo "product/ already exists and is not empty; refusing to overwrite." >&2
  exit 2
fi

cat $CHUNKS | base64 --decode > "$ZIP"
echo "$EXPECTED_SHA  $ZIP" | sha256sum -c -

rm -rf "$TARGET"
mkdir -p "$TARGET"
unzip -q "$ZIP" -d "$TARGET"

test -f "$TARGET/helper-src/main.go"
test -f "$TARGET/updater-src/main.go"
test -f "$TARGET/extension/manifest.json"

python - "$TARGET/extension/manifest.json" <<'PY'
import json, sys
p=sys.argv[1]
with open(p, encoding='utf-8') as f:
    m=json.load(f)
assert m.get('version') == '0.4.0', m.get('version')
print('Bootstrapped Local Toolbox', m['version'], 'into product/')
PY
