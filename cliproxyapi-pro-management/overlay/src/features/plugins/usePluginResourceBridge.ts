import { useEffect, type RefObject } from 'react';
import { apiClient } from '@/services/api/client';

const REQUEST_SOURCE = 'cliproxy-plugin-resource';
const RESPONSE_SOURCE = 'cliproxy-plugin-host';
const ALLOWED_METHODS = new Set(['GET', 'POST', 'PUT', 'PATCH', 'DELETE']);

interface PluginBridgeRequest {
  source: typeof REQUEST_SOURCE;
  id: string;
  method: string;
  path: string;
  body?: unknown;
}

const isBridgeRequest = (value: unknown): value is PluginBridgeRequest => {
  if (!value || typeof value !== 'object') return false;
  const request = value as Partial<PluginBridgeRequest>;
  return (
    request.source === REQUEST_SOURCE &&
    typeof request.id === 'string' &&
    request.id.length > 0 &&
    typeof request.method === 'string' &&
    typeof request.path === 'string'
  );
};

const requestManagementAPI = async (request: PluginBridgeRequest): Promise<unknown> => {
  const method = request.method.trim().toUpperCase();
  const path = request.path.trim();
  if (!ALLOWED_METHODS.has(method)) throw new Error('Unsupported management API method');
  if (!path.startsWith('/') || path.startsWith('//') || path.includes('://')) {
    throw new Error('Invalid management API path');
  }
  if (method === 'GET') return apiClient.get(path);
  if (method === 'POST') return apiClient.post(path, request.body);
  if (method === 'PUT') return apiClient.put(path, request.body);
  if (method === 'PATCH') return apiClient.patch(path, request.body);
  return apiClient.delete(path);
};

export function usePluginResourceBridge(frameRef: RefObject<HTMLIFrameElement | null>): void {
  useEffect(() => {
    const handleMessage = (event: MessageEvent<unknown>) => {
      const frameWindow = frameRef.current?.contentWindow;
      if (!frameWindow || event.source !== frameWindow || !isBridgeRequest(event.data)) return;
      const request = event.data;
      void requestManagementAPI(request)
        .then((data) => {
          frameWindow.postMessage(
            { source: RESPONSE_SOURCE, id: request.id, ok: true, data },
            event.origin
          );
        })
        .catch((error: unknown) => {
          const candidate = error as { message?: unknown; status?: unknown; details?: unknown };
          frameWindow.postMessage(
            {
              source: RESPONSE_SOURCE,
              id: request.id,
              ok: false,
              error: {
                message:
                  typeof candidate?.message === 'string'
                    ? candidate.message
                    : 'Plugin management request failed',
                status: typeof candidate?.status === 'number' ? candidate.status : undefined,
                details: candidate?.details,
              },
            },
            event.origin
          );
        });
    };

    window.addEventListener('message', handleMessage);
    return () => window.removeEventListener('message', handleMessage);
  }, [frameRef]);
}
