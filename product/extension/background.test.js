const test = require('node:test');
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
