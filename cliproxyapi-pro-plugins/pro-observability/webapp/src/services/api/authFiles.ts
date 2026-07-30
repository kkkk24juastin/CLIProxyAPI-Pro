import { getCachedHostBootstrap, loadHostBootstrap } from '@/services/bridge';
import type { AuthFileItem } from '@/types';

export const authFilesApi = {
  async list(): Promise<{ files: AuthFileItem[] }> {
    const bootstrap = getCachedHostBootstrap() ? await loadHostBootstrap(true) : await loadHostBootstrap();
    return { files: bootstrap.authFiles as AuthFileItem[] };
  },
};
