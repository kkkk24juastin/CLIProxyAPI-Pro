import { IconModelCluster } from '@/components/ui/icons';
import type { ProModuleManifest } from '@/pro/manifest';
import { OAuthModelPolicyPage } from './OAuthModelPolicyPage';

export const modelPolicyModule: ProModuleManifest = {
  id: 'oauth-model-policy',
  route: { path: '/oauth-model-policy', element: <OAuthModelPolicyPage /> },
  navigation: {
    groupId: 'pro',
    groupLabelKey: 'nav_groups.pro',
    path: '/oauth-model-policy',
    labelKey: 'nav.oauth_model_policy',
    metaKey: 'nav_meta.oauth_model_policy',
    icon: <IconModelCluster size={18} />,
  },
};
