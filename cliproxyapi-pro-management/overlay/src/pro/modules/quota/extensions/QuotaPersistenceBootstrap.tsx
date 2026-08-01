import { useEffect } from 'react';
import { FEATURES } from '@/config/features';
import { quotaPersistenceMiddleware } from '@/pro/modules/quota/extensions/persistenceMiddleware';

export function QuotaPersistenceBootstrap() {
  useEffect(() => {
    if (!FEATURES.QUOTA_PERSISTENCE) return;
    quotaPersistenceMiddleware.start();
    return () => quotaPersistenceMiddleware.stop();
  }, []);

  return null;
}
