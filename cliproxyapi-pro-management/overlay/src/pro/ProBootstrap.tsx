import { Fragment } from 'react';
import { proBootstraps } from '@/pro/registry';
import '@/pro/registerLocales';
import '@/pro/global.scss';

// ProBootstrap is the single host insertion point for static module startup
// effects that must follow the authenticated Management lifecycle.
export function ProBootstrap() {
  return (
    <>
      {proBootstraps.map((bootstrap) => (
        <Fragment key={bootstrap.id}>{bootstrap.element}</Fragment>
      ))}
    </>
  );
}
