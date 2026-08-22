(() => {
  'use strict';

  const terminalStates = new Set(['completed', 'failed', 'cancelled']);

  function jobPresentation(job = {}) {
    const state = String(job.state || 'queued');
    const completed = state === 'completed';
    const failed = state === 'failed';
    const cancelled = state === 'cancelled';
    const interrupted = state === 'interrupted';
    const rawProgress = Number(job.progress) || 0;
    const progress = completed ? 100 : Math.max(0, Math.min(99.5, rawProgress));

    return Object.freeze({
      state,
      progress,
      percentLabel: failed ? 'فشل' : cancelled ? 'ملغي' : interrupted ? 'متوقف' : `${Math.floor(progress)}%`,
      terminal: terminalStates.has(state),
      failed,
      cancelled,
      interrupted,
      retryable: (failed || cancelled || interrupted) && !!job.request,
      cancellable: !terminalStates.has(state) && !interrupted
    });
  }

  const api = Object.freeze({ jobPresentation });
  globalThis.LocalToolboxUILogic = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})();
