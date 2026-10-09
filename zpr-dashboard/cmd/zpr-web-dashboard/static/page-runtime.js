(() => {
  async function readJSON(response) {
    try {
      return await response.json();
    } catch (error) {
      if (!(error instanceof SyntaxError)) throw error;
      const failure = new Error(`HTTP ${response.status}: invalid JSON response`, { cause: error });
      failure.status = response.status;
      throw failure;
    }
  }

  async function requestJSON(fetcher, url, options = {}, { acceptError = () => false } = {}) {
    const response = await fetcher(url, { cache: "no-store", ...options });
    const result = await readJSON(response);
    if (!response.ok && !acceptError(result)) {
      const error = new Error(result?.error || result?.diagnostics || `HTTP ${response.status}`);
      error.line = result?.line || 0;
      error.details = result;
      error.status = response.status;
      throw error;
    }
    return result;
  }

  function createPoller({ run, interval = null, onError, onPending = () => {} }) {
    if (typeof run !== "function" || typeof onError !== "function") {
      throw new TypeError("Page pollers require run and onError callbacks.");
    }
    let active = false;
    let paused = false;
    let disposed = false;
    let generation = 0;
    let timer;
    let operation;

    function clearTimer() {
      window.clearTimeout(timer);
      timer = undefined;
    }

    function schedule() {
      clearTimer();
      if (!active || paused || disposed || operation) return;
      const delay = typeof interval === "function" ? interval() : interval;
      if (delay != null) {
        timer = window.setTimeout(() => { timer = undefined; void refresh(); }, delay);
      }
    }

    function refresh() {
      if (!active || disposed) return Promise.resolve();
      if (operation) return operation.promise;
      clearTimer();
      const controller = new AbortController();
      const current = { controller, generation, promise: null };
      const isCurrent = () => active && !disposed && operation === current &&
        generation === current.generation && !controller.signal.aborted;
      operation = current;
      onPending(true);
      current.promise = Promise.resolve()
        .then(() => { if (isCurrent()) return run({ signal: controller.signal, isCurrent }); })
        .catch(error => { if (isCurrent()) onError(error); })
        .finally(() => {
          if (operation !== current) return;
          operation = null;
          onPending(false);
          schedule();
        });
      return current.promise;
    }

    function cancel() {
      generation++;
      clearTimer();
      if (operation) {
        operation.controller.abort();
        operation = null;
        onPending(false);
      }
    }

    function start({ immediate = true } = {}) {
      if (active || disposed) return;
      active = true;
      if (immediate && !paused) void refresh();
      else schedule();
    }

    function stop() {
      active = false;
      cancel();
    }

    function setPaused(value) {
      if (paused === value) return;
      paused = value;
      if (paused) cancel();
      else schedule();
    }

    function dispose() {
      stop();
      disposed = true;
      window.removeEventListener("pagehide", stop);
    }

    window.addEventListener("pagehide", stop);
    return Object.freeze({ start, stop, refresh, setPaused, reschedule: schedule, dispose });
  }

  window.ZPRPageRuntime = Object.freeze({ readJSON, requestJSON, createPoller });
})();
