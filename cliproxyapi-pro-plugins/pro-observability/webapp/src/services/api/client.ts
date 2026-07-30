import { getCachedHostBootstrap, requestManagement } from '@/services/bridge';

type RequestOptions = {
  params?: Record<string, unknown>;
  headers?: Record<string, string>;
  signal?: AbortSignal;
};

class PluginAPIClient {
  get<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
    const bootstrap = getCachedHostBootstrap();
    if (path === '/openai-compatibility' && bootstrap) {
      return Promise.resolve({ 'openai-compatibility': bootstrap.openaiCompatibility } as T);
    }
    return requestManagement<T>('GET', path, options);
  }

  async getRaw(path: string, options: RequestOptions & { responseType?: 'blob' } = {}) {
    const data = await requestManagement<unknown>('GET', path, options);
    const normalized = data instanceof Blob
      ? data
      : new Blob([typeof data === 'string' ? data : JSON.stringify(data)], { type: 'application/json' });
    return { data: normalized };
  }

  post<T = unknown>(path: string, body?: unknown, options: RequestOptions = {}): Promise<T> {
    return requestManagement<T>('POST', path, { ...options, body });
  }

  put<T = unknown>(path: string, body?: unknown, options: RequestOptions = {}): Promise<T> {
    return requestManagement<T>('PUT', path, { ...options, body });
  }

  patch<T = unknown>(path: string, body?: unknown, options: RequestOptions = {}): Promise<T> {
    return requestManagement<T>('PATCH', path, { ...options, body });
  }

  delete<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
    return requestManagement<T>('DELETE', path, options);
  }
}

export const apiClient = new PluginAPIClient();
