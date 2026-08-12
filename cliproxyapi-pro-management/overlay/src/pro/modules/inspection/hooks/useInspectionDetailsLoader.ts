import { useEffect } from 'react';

type InspectionDetailsLoaderOptions = {
  enabled: boolean;
  detailKey: string;
  retryNonce: number;
  load: (signal: AbortSignal) => Promise<unknown>;
  loadedKeyRef: { current: string };
  onError: (message: string) => void;
};

const throwIfInspectionDetailsAborted = (signal: AbortSignal) => {
  if (signal.aborted) {
    throw signal.reason ?? new DOMException('Account inspection details request aborted', 'AbortError');
  }
};

export const loadInspectionDetailsWithRetry = async (
  load: (signal: AbortSignal) => Promise<unknown>,
  delay: (milliseconds: number, signal: AbortSignal) => Promise<unknown> = (milliseconds, signal) => new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason ?? new DOMException('Account inspection details request aborted', 'AbortError'));
      return;
    }
    const timeout = window.setTimeout(resolve, milliseconds);
    signal.addEventListener('abort', () => {
      window.clearTimeout(timeout);
      reject(signal.reason ?? new DOMException('Account inspection details request aborted', 'AbortError'));
    }, { once: true });
  }),
  attempts = 3,
  signal: AbortSignal = new AbortController().signal
) => {
  let lastError: unknown;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    throwIfInspectionDetailsAborted(signal);
    try {
      const result = await load(signal);
      throwIfInspectionDetailsAborted(signal);
      return result;
    } catch (error) {
      if (signal.aborted) throw signal.reason ?? new DOMException('Account inspection details request aborted', 'AbortError');
      lastError = error;
      if (attempt + 1 < attempts) await delay(500 * 2 ** attempt, signal);
    }
  }
  throw lastError;
};

export const useInspectionDetailsLoader = ({
  enabled,
  detailKey,
  retryNonce,
  load,
  loadedKeyRef,
  onError,
}: InspectionDetailsLoaderOptions) => {
  useEffect(() => {
    if (!enabled || loadedKeyRef.current === detailKey) return;
    let cancelled = false;
    let idleHandle: number | null = null;
    let timeoutHandle: number | null = null;
    const controller = new AbortController();

    const loadWithRetry = async () => {
      try {
        await loadInspectionDetailsWithRetry(load, undefined, 3, controller.signal);
        if (cancelled) return;
        loadedKeyRef.current = detailKey;
        onError('');
      } catch (error) {
        if (!cancelled) onError(error instanceof Error ? error.message : String(error || 'Unknown error'));
      }
    };

    const windowWithIdleCallback = window as Window & {
      requestIdleCallback?: (callback: () => void, options?: { timeout?: number }) => number;
      cancelIdleCallback?: (handle: number) => void;
    };
    if (windowWithIdleCallback.requestIdleCallback) {
      idleHandle = windowWithIdleCallback.requestIdleCallback(() => void loadWithRetry(), { timeout: 1500 });
    } else {
      timeoutHandle = window.setTimeout(() => void loadWithRetry(), 120);
    }

    return () => {
      cancelled = true;
      controller.abort();
      if (idleHandle !== null) windowWithIdleCallback.cancelIdleCallback?.(idleHandle);
      if (timeoutHandle !== null) window.clearTimeout(timeoutHandle);
    };
  }, [detailKey, enabled, load, loadedKeyRef, onError, retryNonce]);
};
