import { useEffect } from 'react';
import { FEATURES } from '@/config/features';
import { quotaPersistenceMiddleware } from '@/extensions/quota/persistenceMiddleware';

export function QuotaPersistenceBootstrap() {
  useEffect(() => {
    if (!FEATURES.QUOTA_PERSISTENCE) return;
    quotaPersistenceMiddleware.start();
    return () => quotaPersistenceMiddleware.stop();
  }, []);

  return null;
}
