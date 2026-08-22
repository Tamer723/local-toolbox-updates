const test = require('node:test');
const assert = require('node:assert/strict');

const { jobPresentation } = require('./ui-logic.js');

test('only a completed job is presented as 100 percent', () => {
  assert.equal(jobPresentation({ state: 'downloading', progress: 99.5 }).percentLabel, '99%');
  assert.equal(jobPresentation({ state: 'processing', progress: 100 }).percentLabel, '99%');
  assert.equal(jobPresentation({ state: 'completed', progress: 73 }).percentLabel, '100%');
});

test('interrupted jobs can be retried but not cancelled', () => {
  const view = jobPresentation({ state: 'interrupted', progress: 42, request: { action: 'download_video' } });
  assert.equal(view.percentLabel, 'متوقف');
  assert.equal(view.terminal, false);
  assert.equal(view.retryable, true);
  assert.equal(view.cancellable, false);
});

test('canonical terminal states determine available actions', () => {
  assert.deepEqual(
    ['completed', 'failed', 'cancelled'].map(state => jobPresentation({ state }).terminal),
    [true, true, true]
  );
  assert.equal(jobPresentation({ state: 'failed', request: {} }).retryable, true);
  assert.equal(jobPresentation({ state: 'completed', request: {} }).retryable, false);
});
