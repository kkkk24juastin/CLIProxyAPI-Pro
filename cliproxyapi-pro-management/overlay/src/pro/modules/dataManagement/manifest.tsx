import { IconSidebarDataManagement } from '@/components/ui/icons';
import type { ProModuleManifest } from '@/pro/manifest';
import { DataManagementPage } from './DataManagementPage';

export const dataManagementModule: ProModuleManifest = {
  id: 'data-management',
  route: { path: '/data-management', element: <DataManagementPage /> },
  navigation: {
    groupId: 'pro',
    groupLabelKey: 'nav_groups.pro',
    path: '/data-management',
    labelKey: 'nav.data_management',
    metaKey: 'nav_meta.data_management',
    icon: <IconSidebarDataManagement size={18} />,
  },
};
