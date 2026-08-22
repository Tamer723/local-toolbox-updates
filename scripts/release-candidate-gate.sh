#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PRODUCT="$ROOT/product"

echo 'Checking Go formatting'
mapfile -d '' go_files < <(find "$PRODUCT/helper-src" "$PRODUCT/updater-src" -name '*.go' -print0)
unformatted="$(gofmt -l "${go_files[@]}")"
test -z "$unformatted" || { printf 'Unformatted Go files:\n%s\n' "$unformatted" >&2; exit 1; }

echo 'Running Go tests'
(cd "$PRODUCT/helper-src" && go test ./...)
(cd "$PRODUCT/updater-src" && go test ./...)

echo 'Checking extension JavaScript and tests'
while IFS= read -r -d '' file; do node --check "$file"; done < <(find "$PRODUCT/extension" -name '*.js' -print0)
node --test "$PRODUCT"/extension/*.test.js

echo 'Validating JSON and release versions'
python3 - "$PRODUCT" <<'PY'
import json, pathlib, re, sys
product = pathlib.Path(sys.argv[1])
files = sorted(product.rglob('*.json'))
for path in files:
    json.loads(path.read_text(encoding='utf-8'))
manifest = json.loads((product / 'extension/manifest.json').read_text(encoding='utf-8'))
assert manifest['version'] == '0.5.0', manifest['version']
helper = (product / 'helper-src/main.go').read_text(encoding='utf-8')
assert re.search(r'^const version = "0\.5\.0"$', helper, re.MULTILINE), 'helper version is not 0.5.0'
print(f'validated {len(files)} JSON files; extension/helper=0.5.0')
PY

echo 'Cross-building Windows amd64 binaries'
(cd "$PRODUCT/helper-src" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /tmp/local-toolbox-helper.exe .)
(cd "$PRODUCT/updater-src" && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /tmp/local-toolbox-updater-v2.exe .)
test -s /tmp/local-toolbox-helper.exe
test -s /tmp/local-toolbox-updater-v2.exe

echo 'Building and inspecting the unpublished candidate'
bash "$PRODUCT/build-candidate.sh"
python3 - "$PRODUCT/build/candidate" <<'PY'
import hashlib, json, pathlib, sys, zipfile
directory = pathlib.Path(sys.argv[1])
package = directory / 'LocalToolbox_update_0.5.0-rc.zip'
digest_file = directory / 'LocalToolbox_update_0.5.0-rc.sha256'
assert package.is_file() and package.stat().st_size > 0
digest = hashlib.sha256(package.read_bytes()).hexdigest()
assert digest == digest_file.read_text().split()[0]
with zipfile.ZipFile(package) as archive:
    names = set(archive.namelist())
    required = {
        'payload/extension/manifest.json',
        'payload/extension/contracts.js',
        'payload/local-toolbox-helper.exe',
        'payload/local-toolbox-updater-v2.exe',
    }
    assert required <= names, required - names
    assert 'payload/local-toolbox-updater.exe' not in names
    assert json.loads(archive.read('payload/extension/manifest.json'))['version'] == '0.5.0'
print(f'candidate bytes={package.stat().st_size} sha256={digest}')
PY

echo 'Protecting the production update feed'
git -C "$ROOT" diff --quiet -- latest.json releases
git -C "$ROOT" diff --cached --quiet -- latest.json releases
echo 'Release-candidate regression gate passed.'
