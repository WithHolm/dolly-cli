/**
 * poll runs load now and then every `ms`, one run at a time, and not while the tab is hidden.
 * It gives the function that stops it, so it fits as the result of an $effect.
 */
export function poll(load: () => Promise<void>, ms: number): () => void {
  let stopped = false;
  let running = false;

  const run = async () => {
    if (stopped || running || document.hidden) return;
    running = true;
    try {
      await load();
    } catch {
      // the server may be gone for a moment: the next run tries again
    } finally {
      running = false;
    }
  };

  void run();
  const timer = setInterval(run, ms);
  document.addEventListener("visibilitychange", run);
  return () => {
    stopped = true;
    clearInterval(timer);
    document.removeEventListener("visibilitychange", run);
  };
}
