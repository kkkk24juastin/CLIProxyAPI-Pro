import { apiClient } from '@/services/api/client';

export interface ManagementUpdateResult {
  status: string;
  updated: boolean;
  sha256: string;
}

export const checkManagementPanelUpdate = () =>
  apiClient.post<ManagementUpdateResult>('/management-panel/check-update');
