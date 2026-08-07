import type { ReactNode } from 'react';

export interface ProModuleRoute {
  path: string;
  element: ReactNode;
}

export interface ProModuleNavigation {
  groupId: string;
  groupLabelKey: string;
  path: string;
  labelKey: string;
  metaKey: string;
  icon: ReactNode;
}

export interface ProModuleManifest {
  id: string;
  route?: ProModuleRoute;
  routes?: ProModuleRoute[];
  navigation?: ProModuleNavigation;
  bootstrap?: ReactNode;
}
