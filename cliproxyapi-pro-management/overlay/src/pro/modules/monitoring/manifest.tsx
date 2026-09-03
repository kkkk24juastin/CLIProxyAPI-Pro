import { IconSidebarMonitor } from '@/pro/icons';
import type { ProModuleManifest } from '@/pro/manifest';
import { MonitoringCenterPage } from './MonitoringCenterPage';

export const monitoringModule: ProModuleManifest = {
  id: 'monitoring',
  route: { path: '/monitoring', element: <MonitoringCenterPage /> },
  navigation: {
    groupId: 'pro',
    groupLabelKey: 'nav_groups.pro',
    path: '/monitoring',
    labelKey: 'nav.monitoring_center',
    metaKey: 'nav_meta.monitoring_center',
    icon: <IconSidebarMonitor size={18} />,
  },
};
