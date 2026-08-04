import { IconSidebarProxyPool } from '@/components/ui/icons';
import type { ProModuleManifest } from '@/pro/manifest';
import { ProxyPoolPage } from './ProxyPoolPage';

export const proxyPoolModule: ProModuleManifest = {
  id: 'proxy-pool',
  route: { path: '/proxy-pool', element: <ProxyPoolPage /> },
  navigation: {
    groupId: 'pro',
    groupLabelKey: 'nav_groups.pro',
    path: '/proxy-pool',
    labelKey: 'nav.proxy_pool',
    metaKey: 'nav_meta.proxy_pool',
    icon: <IconSidebarProxyPool size={18} />,
  },
};
