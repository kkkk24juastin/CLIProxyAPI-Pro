import { useEffect, type RefObject } from 'react';
import i18n from '@/i18n';
import { resolveAccountPlanLabel } from '@/features/monitoring/accountPlan';
import { maskConfiguredApiKey } from '@/features/monitoring/apiKeyIdentity';
import { authFilesApi } from '@/services/api/authFiles';
import { apiClient } from '@/services/api/client';
import { useAuthStore, useConfigStore, useNotificationStore, useQuotaStore } from '@/stores';
import type { AuthFileItem } from '@/types';
import { computeApiUrl } from '@/utils/connection';
import { sha256Hex } from '@/utils/hash';
import { readStringValue, resolveAuthProvider } from '@/utils/quota';

const REQUEST_SOURCE = 'cliproxy-plugin-resource';
const RESPONSE_SOURCE = 'cliproxy-plugin-host';
const BRIDGE_VERSION = 2;
const ALLOWED_METHODS = new Set(['GET', 'POST', 'PUT', 'PATCH', 'DELETE']);
const MAX_REQUEST_BODY_BYTES = 256 * 1024 * 1024;

type BridgeRequest = {
  source: typeof REQUEST_SOURCE;
  version?: number;
  id?: string;
  kind?: string;
  method?: string;
  path?: string;
  body?: unknown;
  params?: Record<string, unknown>;
  headers?: Record<string, string>;
  streamId?: string;
  filename?: string;
  force?: boolean;
  options?: {
    title?: string;
    message?: string;
    confirmText?: string;
    cancelText?: string;
    variant?: 'danger' | 'primary' | 'secondary';
  };
  message?: string;
  level?: string;
};

const isBridgeRequest = (value: unknown): value is BridgeRequest => {
  if (!value || typeof value !== 'object') return false;
  return (value as Partial<BridgeRequest>).source === REQUEST_SOURCE;
};

const isValidPath = (path: string) => (
  path.startsWith('/') &&
  !path.startsWith('//') &&
  !path.includes('://') &&
  !path.split('/').includes('..')
);

const estimateBodyBytes = (value: unknown): number => {
  if (value === undefined || value === null) return 0;
  if (typeof value === 'string') return new TextEncoder().encode(value).byteLength;
  if (value instanceof ArrayBuffer) return value.byteLength;
  try { return new TextEncoder().encode(JSON.stringify(value)).byteLength; } catch { return MAX_REQUEST_BODY_BYTES + 1; }
};

const requestManagementAPI = async (request: BridgeRequest): Promise<unknown> => {
  const method = String(request.method || '').trim().toUpperCase();
  const path = String(request.path || '').trim();
  if (!ALLOWED_METHODS.has(method)) throw new Error('Unsupported management API method');
  if (!isValidPath(path)) throw new Error('Invalid management API path');
  if (estimateBodyBytes(request.body) > MAX_REQUEST_BODY_BYTES) throw new Error('Management request body is too large');
  const config = { params: request.params, headers: request.headers };
  if (method === 'GET') return apiClient.get(path, config);
  if (method === 'POST') return apiClient.post(path, request.body, config);
  if (method === 'PUT') return apiClient.put(path, request.body, config);
  if (method === 'PATCH') return apiClient.patch(path, request.body, config);
  return apiClient.delete(path, config);
};

const sanitizeProviderItems = (items: unknown): Array<Record<string, unknown>> => {
  if (!Array.isArray(items)) return [];
  return items.map((raw) => {
    const item = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {};
    return {
      prefix: readStringValue(item.prefix),
      authIndex: readStringValue(item.authIndex ?? item['auth-index'] ?? item.auth_index),
    };
  });
};

const sanitizeOpenAIChannel = (raw: unknown, index: number): Record<string, unknown> | null => {
  if (!raw || typeof raw !== 'object') return null;
  const item = raw as Record<string, unknown>;
  const baseURL = readStringValue(item['base-url'] ?? item.baseUrl);
  let host: string;
  try { host = new URL(baseURL).host; } catch { host = baseURL.replace(/^https?:\/\//i, '').split('/')[0] || `channel-${index + 1}`; }
  const apiKeyEntriesRaw = item['api-key-entries'] ?? item.apiKeyEntries;
  const modelsRaw = item.models;
  return {
    name: readStringValue(item.name) || `OpenAI #${index + 1}`,
    prefix: readStringValue(item.prefix),
    'base-url': host || `channel-${index + 1}`,
    disabled: Boolean(item.disabled),
    'auth-index': readStringValue(item['auth-index'] ?? item.authIndex ?? item.auth_index),
    'api-key-entries': Array.isArray(apiKeyEntriesRaw)
      ? apiKeyEntriesRaw.map((entry) => {
          const value = entry && typeof entry === 'object' ? entry as Record<string, unknown> : {};
          return { 'auth-index': readStringValue(value['auth-index'] ?? value.authIndex ?? value.auth_index) };
        })
      : [],
    models: Array.isArray(modelsRaw)
      ? modelsRaw.map((model) => {
          const value = model && typeof model === 'object' ? model as Record<string, unknown> : {};
          return { name: readStringValue(value.name), alias: readStringValue(value.alias) };
        })
      : [],
  };
};

const extractChannelItems = (payload: unknown): unknown[] => {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== 'object') return [];
  const record = payload as Record<string, unknown>;
  const candidate = record['openai-compatibility'] ?? record.items ?? record.data;
  return Array.isArray(candidate) ? candidate : [];
};

const buildHostBootstrap = async () => {
  const config = useConfigStore.getState().config;
  const quotaStore = useQuotaStore.getState();
  const [authPayload, channelPayload] = await Promise.all([
    authFilesApi.list(),
    apiClient.get('/openai-compatibility').catch(() => ({ 'openai-compatibility': [] })),
  ]);
  const authFiles = Array.isArray(authPayload.files) ? authPayload.files : [];
  const sanitizedAuthFiles = authFiles.map((file: AuthFileItem) => {
    const provider = resolveAuthProvider(file);
    const fileName = readStringValue(file.name);
    const idToken = file.id_token && typeof file.id_token === 'object'
      ? file.id_token as Record<string, unknown>
      : {};
    const hostPlanLabel = resolveAccountPlanLabel({
      authFile: file,
      fileName,
      provider,
      quotaStore,
      t: i18n.t,
      emptyLabel: '',
    });
    return {
      name: fileName,
      auth_index: readStringValue(file['auth_index'] ?? file.authIndex),
      provider,
      type: readStringValue(file.type) || provider,
      status: readStringValue(file.status),
      disabled: Boolean(file.disabled),
      unavailable: Boolean(file.unavailable),
      runtime_only: Boolean(file.runtime_only ?? file.runtimeOnly),
      email: readStringValue(file.email) || readStringValue(idToken.email),
      account: readStringValue(file.account) || readStringValue(idToken.account),
      plan_type: readStringValue(file['plan_type'] ?? idToken.plan_type),
      host_plan_label: hostPlanLabel,
      updated_at: readStringValue(file.updated_at ?? file.updatedAt ?? file.mtime),
    };
  });
  const configRecord = (config || {}) as Record<string, unknown>;
  const apiKeys = Array.isArray(config?.apiKeys)
    ? config.apiKeys.map((value) => String(value || '').trim()).filter(Boolean).map((value) => ({
        hash: sha256Hex(value),
        masked: maskConfiguredApiKey(value),
      }))
    : [];
  const openaiCompatibility = extractChannelItems(channelPayload)
    .map(sanitizeOpenAIChannel)
    .filter(Boolean);
  const prefersDark = window.matchMedia?.('(prefers-color-scheme: dark)').matches;
  const themeAttribute = document.documentElement.getAttribute('data-theme');
  return {
    version: 1,
    locale: i18n.resolvedLanguage || i18n.language || 'en',
    theme: themeAttribute === 'dark' || (!themeAttribute && prefersDark) ? 'dark' : 'light',
    route: { search: window.location.search, hash: window.location.hash },
    config: {
      apiKeys,
      geminiApiKeys: sanitizeProviderItems(configRecord.geminiApiKeys),
      claudeApiKeys: sanitizeProviderItems(configRecord.claudeApiKeys),
      codexApiKeys: sanitizeProviderItems(configRecord.codexApiKeys),
      antigravityApiKeys: sanitizeProviderItems(configRecord.antigravityApiKeys),
      vertexApiKeys: sanitizeProviderItems(configRecord.vertexApiKeys),
      openaiCompatibility,
    },
    authFiles: sanitizedAuthFiles,
    openaiCompatibility,
    preferences: {
      realtimeLogColumns: window.localStorage.getItem('cli-proxy-realtime-log-columns-v2'),
      realtimeLogFollow: window.localStorage.getItem('cli-proxy-realtime-log-follow-v1'),
    },
  };
};

const saveBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

export function usePluginResourceBridge(frameRef: RefObject<HTMLIFrameElement | null>): void {
  useEffect(() => {
    const streamControllers = new Map<string, AbortController>();
    const post = (message: Record<string, unknown>, origin = '*') => {
      frameRef.current?.contentWindow?.postMessage(
        { source: RESPONSE_SOURCE, version: BRIDGE_VERSION, ...message },
        origin === 'null' ? '*' : origin
      );
    };
    const respond = (request: BridgeRequest, origin: string, task: Promise<unknown>) => {
      const id = String(request.id || '');
      void task.then((data) => post({ id, kind: request.kind, ok: true, data }, origin)).catch((error: unknown) => {
        const candidate = error as { message?: unknown; status?: unknown; details?: unknown };
        post({
          id,
          kind: request.kind,
          ok: false,
          error: {
            message: typeof candidate?.message === 'string' ? candidate.message : 'Plugin host request failed',
            status: typeof candidate?.status === 'number' ? candidate.status : undefined,
            details: candidate?.details,
          },
        }, origin);
      });
    };

    const handleMessage = (event: MessageEvent<unknown>) => {
      const frameWindow = frameRef.current?.contentWindow;
      if (!frameWindow || event.source !== frameWindow || !isBridgeRequest(event.data)) return;
      const request = event.data;
      const kind = request.kind || 'management.request';
      if (request.version && request.version !== BRIDGE_VERSION) {
        respond(request, event.origin, Promise.reject(new Error('Unsupported plugin bridge version')));
        return;
      }
      if (kind === 'management.request') {
        respond(request, event.origin, requestManagementAPI(request));
        return;
      }
      if (kind === 'host.bootstrap') {
        respond(request, event.origin, buildHostBootstrap());
        return;
      }
      if (kind === 'ui.notify') {
        const level = ['success', 'warning', 'error', 'info'].includes(String(request.level))
          ? request.level as 'success' | 'warning' | 'error' | 'info'
          : 'info';
        useNotificationStore.getState().showNotification(String(request.message || ''), level);
        respond(request, event.origin, Promise.resolve(true));
        return;
      }
      if (kind === 'ui.confirm') {
        const options = request.options || {};
        respond(request, event.origin, new Promise<boolean>((resolve) => {
          useNotificationStore.getState().showConfirmation({
            title: options.title,
            message: options.message || '',
            confirmText: options.confirmText,
            cancelText: options.cancelText,
            variant: options.variant,
            onConfirm: () => resolve(true),
            onCancel: () => resolve(false),
          });
        }));
        return;
      }
      if (kind === 'file.download') {
        const path = String(request.path || '');
        if (path !== '/usage/export') {
          respond(request, event.origin, Promise.reject(new Error('Unsupported plugin download path')));
          return;
        }
        respond(request, event.origin, apiClient.getRaw(path, { responseType: 'blob' }).then((response) => {
          const blob = response.data instanceof Blob ? response.data : new Blob([response.data], { type: 'application/x-ndjson' });
          saveBlob(blob, String(request.filename || 'usage-export.jsonl'));
          return true;
        }));
        return;
      }
      if (kind === 'file.upload') {
        const path = String(request.path || '');
        if (path !== '/usage/import' || !(request.body instanceof ArrayBuffer)) {
          respond(request, event.origin, Promise.reject(new Error('Unsupported plugin upload request')));
          return;
        }
        if (request.body.byteLength > MAX_REQUEST_BODY_BYTES) {
          respond(request, event.origin, Promise.reject(new Error('Plugin upload is too large')));
          return;
        }
        respond(request, event.origin, apiClient.post(path, request.body, {
          params: request.params,
          headers: { 'Content-Type': 'application/x-ndjson' },
        }));
        return;
      }
      if (kind === 'clipboard.write') {
        const text = typeof request.body === 'string' ? request.body : '';
        respond(request, event.origin, navigator.clipboard?.writeText
          ? navigator.clipboard.writeText(text)
          : Promise.reject(new Error('Clipboard API is unavailable')));
        return;
      }
      if (kind === 'stream.close') {
        const streamId = String(request.streamId || '');
        streamControllers.get(streamId)?.abort();
        streamControllers.delete(streamId);
        return;
      }
      if (kind === 'stream.open') {
        const streamId = String(request.streamId || '');
        const path = String(request.path || '');
        if (!streamId || path !== '/usage/stream' || streamControllers.size > 0) {
          respond(request, event.origin, Promise.reject(new Error('Only one usage stream is allowed per plugin resource')));
          return;
        }
        const auth = useAuthStore.getState();
        const base = computeApiUrl(auth.apiBase);
        const url = new URL(`${base}${path}`);
        Object.entries(request.params || {}).forEach(([key, value]) => {
          if (value !== undefined && value !== null && value !== '') url.searchParams.set(key, String(value));
        });
        const controller = new AbortController();
        streamControllers.set(streamId, controller);
        let acknowledged = false;
        const streamTask = fetch(url, {
          headers: { Authorization: `Bearer ${auth.managementKey}` },
          signal: controller.signal,
        }).then(async (response) => {
          if (!response.ok || !response.body) throw new Error(`Usage stream failed: ${response.status}`);
          post({ id: request.id, kind, ok: true, data: { status: response.status } }, event.origin);
          acknowledged = true;
          const reader = response.body.getReader();
          const decoder = new TextDecoder();
          while (!controller.signal.aborted) {
            const { value, done } = await reader.read();
            if (done) break;
            post({ kind: 'stream.chunk', streamId, chunk: decoder.decode(value, { stream: true }) }, event.origin);
          }
          post({ kind: 'stream.close', streamId }, event.origin);
        }).catch((error: unknown) => {
          if (!controller.signal.aborted) {
            const message = error instanceof Error ? error.message : String(error);
            post(acknowledged
              ? { kind: 'stream.error', streamId, error: { message } }
              : { id: request.id, kind, ok: false, error: { message } }, event.origin);
          }
        }).finally(() => streamControllers.delete(streamId));
        void streamTask;
        return;
      }
      respond(request, event.origin, Promise.reject(new Error(`Unsupported plugin bridge request: ${kind}`)));
    };

    window.addEventListener('message', handleMessage);
    return () => {
      window.removeEventListener('message', handleMessage);
      streamControllers.forEach((controller) => controller.abort());
      streamControllers.clear();
    };
  }, [frameRef]);
}
