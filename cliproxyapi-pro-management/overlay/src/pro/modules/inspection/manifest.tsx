import { IconSidebarAccountInspection } from '@/pro/icons';
import type { ProModuleManifest } from '@/pro/manifest';
import { AccountInspectionPage } from './AccountInspectionPage';

export const inspectionModule: ProModuleManifest = {
  id: 'inspection',
  route: { path: '/account-inspection', element: <AccountInspectionPage /> },
  navigation: {
    groupId: 'pro',
    groupLabelKey: 'nav_groups.pro',
    path: '/account-inspection',
    labelKey: 'nav.account_inspection',
    metaKey: 'nav_meta.account_inspection',
    icon: <IconSidebarAccountInspection size={18} />,
  },
};
