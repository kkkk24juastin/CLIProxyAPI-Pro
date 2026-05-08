import { apiClient } from './client';
import type {
  AccountInspectionConfigurableSettings,
  AccountInspectionLogLevel,
  AccountInspectionResultItem,
  AccountInspectionSummary,
} from '@/features/monitoring/accountInspection';

export type AccountInspectionSchedule = {
  enabled: boolean;
  intervalMinutes: number;
  nextRunAt: number;
  settings: AccountInspectionConfigurableSettings;
};

export type AccountInspectionBackendLog = {
  time: number;
  level: AccountInspectionLogLevel;
  message: string;
};

export type AccountInspectionBackendStatus = {
  running: boolean;
  lastStartedAt: number;
  lastFinishedAt: number;
  lastError: string;
  summary: AccountInspectionSummary & {
    executedDeleteCount?: number;
    executedDisableCount?: number;
    executedEnableCount?: number;
  };
  logs: AccountInspectionBackendLog[];
  results: Array<
    Omit<AccountInspectionResultItem, 'displayAccount' | 'accountId' | 'status' | 'state' | 'raw'> & {
      displayName: string;
      executed?: boolean;
      executeError?: string;
    }
  >;
};

export type AccountInspectionScheduleResponse = {
  schedule: AccountInspectionSchedule;
  status: AccountInspectionBackendStatus;
};

export const accountInspectionApi = {
  getSchedule: () => apiClient.get<AccountInspectionScheduleResponse>('/account-inspection/schedule'),
  updateSchedule: (schedule: AccountInspectionSchedule) =>
    apiClient.put<AccountInspectionScheduleResponse>('/account-inspection/schedule', schedule),
  runNow: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/run', {}),
};
