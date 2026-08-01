import { QuotaPersistenceBootstrap } from '@/extensions/quota/QuotaPersistenceBootstrap';

// ProBootstrap is the single host insertion point for static module startup
// effects that must follow the authenticated Management lifecycle.
export function ProBootstrap() {
  return <QuotaPersistenceBootstrap />;
}
