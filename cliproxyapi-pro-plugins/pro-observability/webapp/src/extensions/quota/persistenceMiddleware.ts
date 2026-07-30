import { loadHostBootstrap } from '@/services/bridge';
import { initializeStores } from '@/stores';

export const quotaPersistenceMiddleware = {
  markStale() {},
  async ensureFresh() {
    initializeStores(await loadHostBootstrap(true));
  },
};
