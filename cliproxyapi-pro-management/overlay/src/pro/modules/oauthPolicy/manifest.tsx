import { IconModelCluster } from '@/components/ui/icons';
import type { ProModuleManifest } from '@/pro/manifest';
import { Navigate } from 'react-router-dom';
import { OAuthPolicyPage } from './OAuthPolicyPage';

export const oauthPolicyModule: ProModuleManifest = {
  id: 'oauth-policy',
  route: { path: '/oauth-policy', element: <OAuthPolicyPage /> },
  routes: [
    { path: '/oauth-model-policy', element: <Navigate to="/oauth-policy" replace /> },
  ],
  navigation: {
    groupId: 'pro',
    groupLabelKey: 'nav_groups.pro',
    path: '/oauth-policy',
    labelKey: 'nav.oauth_policy',
    metaKey: 'nav_meta.oauth_policy',
    icon: <IconModelCluster size={18} />,
  },
};
