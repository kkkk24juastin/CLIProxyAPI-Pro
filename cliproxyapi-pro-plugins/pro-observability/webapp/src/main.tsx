import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import '@/styles/global.scss';
import { MonitoringCenterPage } from '@/pages/MonitoringCenterPage';
import { AccountInspectionPage } from '@/pages/AccountInspectionPage';
import { OAuthModelPolicyPage } from '@/pages/OAuthModelPolicyPage';
import { ProxyPoolPage } from '@/pages/ProxyPoolPage';
import { loadHostBootstrap } from '@/services/bridge';
import { initializeStores } from '@/stores';
import i18n, { setPluginLanguage } from '@/i18n';

declare const __PLUGIN_MANAGEMENT_PAGE__: string;

async function start() {
  const bootstrap = await loadHostBootstrap();
  const legacyColumns = bootstrap.preferences?.realtimeLogColumns;
  const legacyFollow = bootstrap.preferences?.realtimeLogFollow;
  if (typeof legacyColumns === 'string' && !window.localStorage.getItem('cli-proxy-realtime-log-columns-v2')) {
    window.localStorage.setItem('cli-proxy-realtime-log-columns-v2', legacyColumns);
  }
  if (typeof legacyFollow === 'string' && !window.localStorage.getItem('cli-proxy-realtime-log-follow-v1')) {
    window.localStorage.setItem('cli-proxy-realtime-log-follow-v1', legacyFollow);
  }
  initializeStores(bootstrap);
  await setPluginLanguage(bootstrap.locale);
  document.documentElement.dataset.theme = bootstrap.theme;
  document.title = __PLUGIN_MANAGEMENT_PAGE__ === 'proxy-pool'
    ? i18n.t('proxy_pool.title')
    : __PLUGIN_MANAGEMENT_PAGE__ === 'oauth-model-policy'
      ? i18n.t('oauth_model_policy.title')
      : i18n.t('nav.monitoring_center');
  const route = `${bootstrap.route?.search || ''}${bootstrap.route?.hash || ''}` || '/';
  const Page = __PLUGIN_MANAGEMENT_PAGE__ === 'proxy-pool'
    ? ProxyPoolPage
    : __PLUGIN_MANAGEMENT_PAGE__ === 'oauth-model-policy'
      ? OAuthModelPolicyPage
      : window.location.pathname.includes('/account-inspection')
        ? AccountInspectionPage
        : MonitoringCenterPage;
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <MemoryRouter initialEntries={[route]}>
        <Page />
      </MemoryRouter>
    </StrictMode>
  );
}

void start().catch((error) => {
  const root = document.getElementById('root');
  if (root) root.textContent = error instanceof Error ? error.message : String(error);
});
