import type { ProModuleManifest } from '@/pro/manifest';
import { QuotaPersistenceBootstrap } from './extensions/QuotaPersistenceBootstrap';

export const quotaModule: ProModuleManifest = {
  id: 'quota',
  bootstrap: <QuotaPersistenceBootstrap />,
};
