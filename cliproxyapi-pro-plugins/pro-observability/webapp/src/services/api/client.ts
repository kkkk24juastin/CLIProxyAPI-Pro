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
