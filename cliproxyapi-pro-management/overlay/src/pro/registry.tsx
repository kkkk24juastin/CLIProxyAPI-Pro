import type { ReactNode } from 'react';
import {
  IconModelCluster,
  IconSidebarAccountInspection,
  IconSidebarMonitor,
  IconSidebarProxyPool,
  IconSidebarRouting,
} from '@/components/ui/icons';
import { MonitoringCenterPage } from '@/pages/MonitoringCenterPage';
import { AccountInspectionPage } from '@/pages/AccountInspectionPage';
import { RoutingPolicyPage } from '@/pages/RoutingPolicyPage';
import { ProxyPoolPage } from '@/pages/ProxyPoolPage';
import { OAuthModelPolicyPage } from '@/pages/OAuthModelPolicyPage';

export interface ProRouteEntry {
  path: string;
  element: ReactNode;
}

export interface ProNavigationItem {
  path: string;
  labelKey: string;
  metaKey: string;
  icon: ReactNode;
}

export interface ProNavigationGroup {
  id: string;
  labelKey: string;
  items: ProNavigationItem[];
}

// Static Pro modules contribute their routes through one stable host seam.
export const proRoutes: ProRouteEntry[] = [
  { path: '/monitoring', element: <MonitoringCenterPage /> },
  { path: '/account-inspection', element: <AccountInspectionPage /> },
  { path: '/routing', element: <RoutingPolicyPage /> },
  { path: '/proxy-pool', element: <ProxyPoolPage /> },
  { path: '/oauth-model-policy', element: <OAuthModelPolicyPage /> },
];

export const proNavigationGroups: ProNavigationGroup[] = [
  {
    id: 'pro',
    labelKey: 'nav_groups.pro',
    items: [
      {
        path: '/monitoring',
        labelKey: 'nav.monitoring_center',
        metaKey: 'nav_meta.monitoring_center',
        icon: <IconSidebarMonitor size={18} />,
      },
      {
        path: '/account-inspection',
        labelKey: 'nav.account_inspection',
        metaKey: 'nav_meta.account_inspection',
        icon: <IconSidebarAccountInspection size={18} />,
      },
      {
        path: '/routing',
        labelKey: 'nav.routing_policy',
        metaKey: 'nav_meta.routing_policy',
        icon: <IconSidebarRouting size={18} />,
      },
      {
        path: '/oauth-model-policy',
        labelKey: 'nav.oauth_model_policy',
        metaKey: 'nav_meta.oauth_model_policy',
        icon: <IconModelCluster size={18} />,
      },
      {
        path: '/proxy-pool',
        labelKey: 'nav.proxy_pool',
        metaKey: 'nav_meta.proxy_pool',
        icon: <IconSidebarProxyPool size={18} />,
      },
    ],
  },
];
