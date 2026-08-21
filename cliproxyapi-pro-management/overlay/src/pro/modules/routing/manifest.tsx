import { IconSidebarRouting } from '@/pro/icons';
import type { ProModuleManifest } from '@/pro/manifest';
import { RoutingPolicyPage } from './RoutingPolicyPage';

export const routingModule: ProModuleManifest = {
  id: 'routing',
  route: { path: '/routing', element: <RoutingPolicyPage /> },
  navigation: {
    groupId: 'pro',
    groupLabelKey: 'nav_groups.pro',
    path: '/routing',
    labelKey: 'nav.routing_policy',
    metaKey: 'nav_meta.routing_policy',
    icon: <IconSidebarRouting size={18} />,
  },
};
