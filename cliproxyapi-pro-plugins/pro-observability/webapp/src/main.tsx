import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import '@/styles/global.scss';
import { MonitoringCenterPage } from '@/pages/MonitoringCenterPage';
import { loadHostBootstrap } from '@/services/bridge';
import { initializeStores } from '@/stores';
import { setPluginLanguage } from '@/i18n';

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
  const route = `${bootstrap.route?.search || ''}${bootstrap.route?.hash || ''}` || '/';
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <MemoryRouter initialEntries={[route]}>
        <MonitoringCenterPage />
      </MemoryRouter>
    </StrictMode>
  );
}

void start().catch((error) => {
  const root = document.getElementById('root');
  if (root) root.textContent = error instanceof Error ? error.message : String(error);
});
