import { IconSidebarAPIKeyPolicy } from '@/pro/icons';
import type { ProModuleManifest } from '@/pro/manifest';
import { APIKeyPolicyPage } from './APIKeyPolicyPage';

export const apiKeyPolicyModule: ProModuleManifest = {
  id: 'api-key-policy',
  route: { path: '/api-key-policy', element: <APIKeyPolicyPage /> },
  navigation: {
    groupId: 'pro',
    groupLabelKey: 'nav_groups.pro',
    path: '/api-key-policy',
    labelKey: 'nav.api_key_policy',
    metaKey: 'nav_meta.api_key_policy',
    icon: <IconSidebarAPIKeyPolicy size={18} />,
  },
};
