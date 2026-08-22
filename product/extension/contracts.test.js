const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const c = require('./contracts.js');
const spec = JSON.parse(fs.readFileSync(path.join(__dirname,'..','contracts','contracts-0.5.json'),'utf8'));

test('JavaScript enums match the canonical schema', () => {
  assert.equal(c.Protocol.version,spec.protocolVersion);
  assert.deepEqual(Object.values(c.JobState).sort(),spec.jobStates.slice().sort());
  assert.deepEqual(Object.values(c.DownloadStrategy).sort(),spec.downloadStrategies.slice().sort());
  assert.deepEqual(Object.values(c.NativeCommand).sort(),spec.commands.slice().sort());
  assert.deepEqual(Object.values(c.NativeEvent).sort(),spec.events.slice().sort());
  assert.deepEqual(Object.values(c.ErrorCode).sort(),spec.errorCodes.slice().sort());
  assert.deepEqual(Object.values(c.Capability).sort(),spec.capabilities.slice().sort());
});

test('progress reaches 100 only for completed jobs', () => {
  assert.equal(c.progressEvent({event:'progress',progress:100}).progress,99.5);
  assert.equal(c.progressEvent({event:'complete',state:c.JobState.COMPLETED,progress:100}).progress,100);
});

test('persistent job contract has explicit interrupted state', () => {
  assert.equal(c.job({jobId:'x',state:c.JobState.INTERRUPTED,progress:100}).progress,99.5);
});

test('cancel acknowledgement preserves the current canonical state and progress', () => {
  const downloading = c.reduceJobEvent(
    {jobId:'x',event:'progress',state:c.JobState.DOWNLOADING,progress:64},
    {jobId:'x',event:c.NativeEvent.CANCEL_REQUESTED,progress:0}
  );
  assert.equal(downloading.event,c.NativeEvent.CANCEL_REQUESTED);
  assert.equal(downloading.state,c.JobState.DOWNLOADING);
  assert.equal(downloading.progress,64);
});

test('late cancel acknowledgement cannot regress a cancelled job', () => {
  const cancelled = c.reduceJobEvent(
    {jobId:'x',event:c.NativeEvent.CANCELLED,state:c.JobState.CANCELLED,progress:42},
    {jobId:'x',event:c.NativeEvent.CANCEL_REQUESTED,progress:0}
  );
  assert.equal(cancelled.event,c.NativeEvent.CANCELLED);
  assert.equal(cancelled.state,c.JobState.CANCELLED);
  assert.equal(cancelled.progress,42);
});
