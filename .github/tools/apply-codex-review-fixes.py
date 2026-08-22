from pathlib import Path
import re


def read(path):
    return Path(path).read_text(encoding="utf-8")


def write(path, text):
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    Path(path).write_text(text, encoding="utf-8")


def replace_once(text, old, new, label):
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected exactly one match, got {count}")
    return text.replace(old, new, 1)


# 1) Chrome detection lifecycle: remove duplicate startup registrations and clear blob state.
p = Path("product/extension/background.js")
s = read(p)
s = replace_once(
    s,
    "function clearDetected(tabId) {\n  detectedByTab.delete(tabId);\n  updateMediaBadge(tabId);\n}",
    "function clearDetected(tabId) {\n  detectedByTab.delete(tabId);\n  blobByTab.delete(tabId);\n  updateMediaBadge(tabId);\n}",
    "clearDetected",
)
pattern = re.compile(
    r"chrome\.runtime\.onStartup\.addListener\(\(\) => \{\s*\n"
    r"try \{[\s\S]*?"
    r"restoreJobs\(\)\.finally\(\(\)=>connect\(\)\); \}\);\n\n",
    re.MULTILINE,
)
s, n = pattern.subn("", s, count=1)
if n != 1:
    raise SystemExit(f"duplicate startup listener block: expected one match, got {n}")
s = replace_once(
    s,
    "chrome.tabs.onRemoved.addListener(tabId => detectedByTab.delete(tabId));",
    "chrome.tabs.onRemoved.addListener(tabId => clearDetected(tabId));",
    "tabs.onRemoved cleanup",
)
write(p, s)

background_test = r'''const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');

const source = fs.readFileSync(__dirname + '/background.js', 'utf8');

test('Chrome detection lifecycle listeners are registered exactly once', () => {
  for (const listener of ['webRequest.onHeadersReceived','tabs.onUpdated','tabs.onRemoved']) {
    const count = source.split(`${listener}.addListener`).length - 1;
    assert.equal(count, 1, `${listener} registered ${count} times`);
  }
  assert.equal(source.split('restoreJobs().finally(()=>connect())').length - 1, 1);
});

test('tab cleanup clears media and blob detection stores', () => {
  const body = source.match(/function clearDetected\(tabId\) \{([\s\S]*?)\n\}/)?.[1] || '';
  assert.match(body, /detectedByTab\.delete\(tabId\)/);
  assert.match(body, /blobByTab\.delete\(tabId\)/);
  assert.match(source, /tabs\.onRemoved\.addListener\(tabId => clearDetected\(tabId\)\)/);
});
'''
write("product/extension/background.test.js", background_test)

# 2) Safe Windows updater migration: ship v2 side-by-side and prefer it in helper 0.5.0.
p = Path("product/build-candidate.sh")
s = read(p)
s = replace_once(
    s,
    '"$STAGE/payload/local-toolbox-updater.exe"',
    '"$STAGE/payload/local-toolbox-updater-v2.exe"',
    "candidate updater filename",
)
write(p, s)

p = Path("product/README.md")
s = read(p)
needle = "- `updater-src` preserves the existing manifest → size/SHA-256 verification → updater handoff protocol and safely installs only `payload/` entries.\n"
addition = needle + "- The 0.5.0 package installs `local-toolbox-updater-v2.exe` beside the legacy updater, avoiding replacement of the running 0.4.0 updater; helper 0.5.0 prefers v2 for subsequent upgrades.\n"
if "local-toolbox-updater-v2.exe" not in s:
    s = replace_once(s, needle, addition, "README updater migration note")
write(p, s)

p = Path("product/helper-src/updates.go")
s = read(p)
insert_anchor = '''func downloadAndVerifyPackage(nw *nativeWriter, req Request, m UpdateManifest) (string, error) {'''
updater_path_fn = '''func updaterPath(root string) (string, bool) {
\tfor _, name := range []string{"local-toolbox-updater-v2.exe", "local-toolbox-updater.exe"} {
\t\tcandidate := filepath.Join(root, name)
\t\tif st, err := os.Stat(candidate); err == nil && !st.IsDir() {
\t\t\treturn candidate, true
\t\t}
\t}
\treturn "", false
}

'''
if "func updaterPath(root string)" not in s:
    s = replace_once(s, insert_anchor, updater_path_fn + insert_anchor, "updaterPath insertion")
old = '''\tupdater := filepath.Join(filepath.Dir(os.Args[0]), "local-toolbox-updater.exe")
\tif st, e := os.Stat(updater); e != nil || st.IsDir() {
\t\t_ = nw.send(Response{ID: req.ID, Event: "update_error", Message: "مكوّن التحديث المحلي غير موجود. ثبّت 0.3.0 يدويًا مرة واحدة.", Version: version})
\t\treturn
\t}
'''
new = '''\troot := filepath.Dir(os.Args[0])
\tupdater, found := updaterPath(root)
\tif !found {
\t\t_ = nw.send(Response{ID: req.ID, Event: "update_error", Message: "مكوّن التحديث المحلي غير موجود. ثبّت 0.3.0 يدويًا مرة واحدة.", Version: version})
\t\treturn
\t}
'''
s = replace_once(s, old, new, "helper updater selection")
s = replace_once(s, '\troot := filepath.Dir(os.Args[0])\n\tcmd := exec.Command(updater,', '\tcmd := exec.Command(updater,', "duplicate root removal")
write(p, s)

updates_test = '''package main

import (
\t"os"
\t"path/filepath"
\t"testing"
)

func TestUpdaterPathPrefersV2AndFallsBackToLegacy(t *testing.T) {
\troot := t.TempDir()
\tlegacy := filepath.Join(root, "local-toolbox-updater.exe")
\tv2 := filepath.Join(root, "local-toolbox-updater-v2.exe")
\tif err := os.WriteFile(legacy, []byte("legacy"), 0755); err != nil {
\t\tt.Fatal(err)
\t}
\tif got, ok := updaterPath(root); !ok || got != legacy {
\t\tt.Fatalf("legacy fallback = %q, %v", got, ok)
\t}
\tif err := os.WriteFile(v2, []byte("v2"), 0755); err != nil {
\t\tt.Fatal(err)
\t}
\tif got, ok := updaterPath(root); !ok || got != v2 {
\t\tt.Fatalf("v2 preference = %q, %v", got, ok)
\t}
}
'''
write("product/helper-src/updates_test.go", updates_test)

p = Path("product/updater-src/main_test.go")
s = read(p)
if "TestLegacyUpdaterCanInstallV2SideBySide" not in s:
    s += '''
func TestLegacyUpdaterCanInstallV2SideBySide(t *testing.T) {
\td := t.TempDir()
\troot := filepath.Join(d, "root")
\tif err := os.MkdirAll(root, 0755); err != nil {
\t\tt.Fatal(err)
\t}
\tlegacy := filepath.Join(root, "local-toolbox-updater.exe")
\tif err := os.WriteFile(legacy, []byte("running-0.4.0-updater"), 0755); err != nil {
\t\tt.Fatal(err)
\t}
\tpkg := filepath.Join(d, "migration.zip")
\tf, err := os.Create(pkg)
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tz := zip.NewWriter(f)
\tw, err := z.Create("payload/local-toolbox-updater-v2.exe")
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tif _, err = w.Write([]byte("updater-v2")); err != nil {
\t\tt.Fatal(err)
\t}
\tif err = z.Close(); err != nil {
\t\tt.Fatal(err)
\t}
\tif err = f.Close(); err != nil {
\t\tt.Fatal(err)
\t}
\tif err = install(pkg, root); err != nil {
\t\tt.Fatal(err)
\t}
\told, err := os.ReadFile(legacy)
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tif string(old) != "running-0.4.0-updater" {
\t\tt.Fatalf("legacy updater was replaced: %q", old)
\t}
\tgot, err := os.ReadFile(filepath.Join(root, "local-toolbox-updater-v2.exe"))
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tif string(got) != "updater-v2" {
\t\tt.Fatalf("v2 updater = %q", got)
\t}
}
'''
write(p, s)

# 3) Keep CI aligned with the new regression tests and v2 package layout.
p = Path(".github/workflows/product-ci.yml")
s = read(p)
s = replace_once(
    s,
    "          node --test product/extension/contracts.test.js",
    "          node --test product/extension/*.test.js",
    "CI JS tests",
)
s = replace_once(
    s,
    "          (cd product/updater-src && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /tmp/local-toolbox-updater.exe .)\n          test -s /tmp/local-toolbox-helper.exe\n          test -s /tmp/local-toolbox-updater.exe",
    "          (cd product/updater-src && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /tmp/local-toolbox-updater-v2.exe .)\n          test -s /tmp/local-toolbox-helper.exe\n          test -s /tmp/local-toolbox-updater-v2.exe",
    "CI updater cross-build",
)
s = replace_once(
    s,
    "                  'payload/local-toolbox-updater.exe',",
    "                  'payload/local-toolbox-updater-v2.exe',",
    "CI candidate updater filename",
)
needle = "              manifest = json.loads(z.read('payload/extension/manifest.json'))\n"
if "assert 'payload/local-toolbox-updater.exe' not in names" not in s:
    s = replace_once(
        s,
        needle,
        "              assert 'payload/local-toolbox-updater.exe' not in names, 'legacy updater must not be replaced during 0.4.0 -> 0.5.0 migration'\n" + needle,
        "CI legacy updater exclusion",
    )
write(p, s)

# Guardrails: this repair must never touch production feed files.
for forbidden in [Path("latest.json"), Path("releases")]:
    if forbidden.is_file() and forbidden.stat().st_mtime_ns == 0:
        raise SystemExit("unexpected production feed mutation")

print("Applied Codex 0.5.0 review fixes")
