export const REQUEST_SOURCE = 'cliproxy-plugin-resource';
export const RESPONSE_SOURCE = 'cliproxy-plugin-host';
export const BRIDGE_VERSION = 2;

type BridgeError = { message?: string; status?: number; details?: unknown };
type PendingRequest = {
  resolve: (value: unknown) => void;
  reject: (reason?: unknown) => void;
  timer: number;
};
type StreamState = {
  controller: ReadableStreamDefaultController<Uint8Array>;
  signal: AbortSignal;
};

export interface HostBootstrap {
  version: 1;
  locale: string;
  theme: 'light' | 'dark';
  route?: { search?: string; hash?: string };
  config: Record<string, unknown>;
  authFiles: Array<Record<string, unknown>>;
  openaiCompatibility: Array<Record<string, unknown>>;
  preferences?: Record<string, unknown>;
}

let sequence = 0;
let bootstrapCache: HostBootstrap | null = null;
const pending = new Map<string, PendingRequest>();
const streams = new Map<string, StreamState>();
const encoder = new TextEncoder();

const nextID = (prefix: string) => `observability-${prefix}-${Date.now()}-${++sequence}`;

const post = (message: Record<string, unknown>, transfer?: Transferable[]) => {
  window.parent.postMessage(
    { source: REQUEST_SOURCE, version: BRIDGE_VERSION, ...message },
    '*',
    transfer ?? []
  );
};

const request = <T>(kind: string, payload: Record<string, unknown> = {}, timeoutMs = 30000, transfer?: Transferable[]) =>
  new Promise<T>((resolve, reject) => {
    const id = nextID(kind);
    const timer = window.setTimeout(() => {
      pending.delete(id);
      reject(new Error(`Host bridge timeout: ${kind}`));
    }, timeoutMs);
    pending.set(id, { resolve: resolve as (value: unknown) => void, reject, timer });
    post({ id, kind, ...payload }, transfer);
  });

const handleHostMessage = (event: MessageEvent<unknown>) => {
  if (event.source !== window.parent || !event.data || typeof event.data !== 'object') return;
  const message = event.data as Record<string, unknown>;
  if (message.source !== RESPONSE_SOURCE || message.version !== BRIDGE_VERSION) return;

  if (message.kind === 'stream.chunk' && typeof message.streamId === 'string') {
    const stream = streams.get(message.streamId);
    if (stream && typeof message.chunk === 'string' && !stream.signal.aborted) {
      stream.controller.enqueue(encoder.encode(message.chunk));
    }
    return;
  }
  if ((message.kind === 'stream.close' || message.kind === 'stream.error') && typeof message.streamId === 'string') {
    const stream = streams.get(message.streamId);
    if (!stream) return;
    streams.delete(message.streamId);
    if (message.kind === 'stream.error') {
      stream.controller.error(new Error(String((message.error as BridgeError | undefined)?.message || 'Usage stream failed')));
    } else {
      stream.controller.close();
    }
    return;
  }

  const id = typeof message.id === 'string' ? message.id : '';
  const task = pending.get(id);
  if (!task) return;
  pending.delete(id);
  window.clearTimeout(task.timer);
  if (message.ok) task.resolve(message.data);
  else {
    const error = (message.error || {}) as BridgeError;
    task.reject(Object.assign(new Error(error.message || 'Host bridge request failed'), error));
  }
};

if (typeof window !== 'undefined') {
  window.addEventListener('message', handleHostMessage);
}

export async function loadHostBootstrap(force = false): Promise<HostBootstrap> {
  if (bootstrapCache && !force) return bootstrapCache;
  bootstrapCache = await request<HostBootstrap>('host.bootstrap', { force });
  return bootstrapCache;
}

export const getCachedHostBootstrap = () => bootstrapCache;

export async function requestManagement<T>(
  method: string,
  path: string,
  options: { body?: unknown; params?: Record<string, unknown>; headers?: Record<string, string>; signal?: AbortSignal } = {}
): Promise<T> {
  if (options.signal?.aborted) throw new DOMException('Aborted', 'AbortError');
  const promise = request<T>('management.request', {
    method,
    path,
    body: options.body,
    params: options.params,
    headers: options.headers,
  });
  if (!options.signal) return promise;
  return Promise.race([
    promise,
    new Promise<T>((_, reject) => options.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })),
  ]);
}

export async function openManagementStream(
  path: string,
  params: Record<string, unknown>,
  signal: AbortSignal
): Promise<{ ok: boolean; status: number; body: ReadableStream<Uint8Array> }> {
  const streamId = nextID('stream');
  let controller!: ReadableStreamDefaultController<Uint8Array>;
  const body = new ReadableStream<Uint8Array>({ start(value) { controller = value; } });
  streams.set(streamId, { controller, signal });
  signal.addEventListener('abort', () => {
    const stream = streams.get(streamId);
    if (stream) {
      streams.delete(streamId);
      try { stream.controller.close(); } catch { /* already closed */ }
    }
    post({ kind: 'stream.close', streamId });
  }, { once: true });
  try {
    const response = await request<{ status: number }>('stream.open', { streamId, path, params });
    return { ok: response.status >= 200 && response.status < 300, status: response.status, body };
  } catch (error) {
    streams.delete(streamId);
    controller.error(error);
    throw error;
  }
}

export const notifyHost = (message: string, level = 'info') =>
  request<void>('ui.notify', { message, level }, 10000);

export const confirmHost = (options: {
  title?: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  variant?: string;
}) => request<boolean>('ui.confirm', { options }, 120000);

export const downloadFromHost = (path: string, filename: string) =>
  request<void>('file.download', { path, filename }, 120000);

export const uploadToHost = <T>(path: string, content: string, params?: Record<string, unknown>) => {
  const buffer = new TextEncoder().encode(content).buffer;
  return request<T>('file.upload', {
    path,
    body: buffer,
    params,
    headers: { 'Content-Type': 'application/x-ndjson' },
  }, 120000, [buffer]);
};

export const writeClipboardHost = (text: string) =>
  request<void>('clipboard.write', { body: text }, 10000);
