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

export type AccountInspectionBackendRunState = 'idle' | 'running' | 'paused' | 'stopping' | 'stopped' | 'completed' | 'partial' | 'failed';

export type AccountInspectionBackendProgress = {
  total: number;
  completed: number;
  inFlight: number;
  pending: number;
  status: AccountInspectionBackendRunState;
};

export type AccountInspectionBackendStatus = {
  running: boolean;
  paused: boolean;
  stopping: boolean;
  state: AccountInspectionBackendRunState;
  lastStartedAt: number;
  lastFinishedAt: number;
  lastError: string;
  progress?: AccountInspectionBackendProgress;
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

export type AccountInspectionLogStreamMessage = {
  type: 'snapshot' | 'log' | 'status';
  schedule: AccountInspectionSchedule;
  status: AccountInspectionBackendStatus;
  log?: AccountInspectionBackendLog;
  logs?: AccountInspectionBackendLog[];
};

export type AccountInspectionActionOutcome = {
  action: 'delete' | 'disable' | 'enable';
  fileName: string;
  displayName: string;
  provider: string;
  authIndex: string;
  success: boolean;
  error: string;
};

export type AccountInspectionActionsResponse = {
  outcomes: AccountInspectionActionOutcome[];
  summary: { total: number; success: number; failed: number };
  status: AccountInspectionBackendStatus;
};

export type AccountInspectionScheduleResponse = {
  schedule: AccountInspectionSchedule;
  status: AccountInspectionBackendStatus;
};

export const accountInspectionApi = {
  getSchedule: () => apiClient.get<AccountInspectionScheduleResponse>('/account-inspection/schedule'),
  getStatus: () => apiClient.get<AccountInspectionScheduleResponse>('/account-inspection/status'),
  updateSchedule: (schedule: AccountInspectionSchedule) =>
    apiClient.put<AccountInspectionScheduleResponse>('/account-inspection/schedule', schedule),
  runNow: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/run', {}),
  pause: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/pause', {}),
  resume: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/resume', {}),
  stop: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/stop', {}),
  executeActions: (items: AccountInspectionResultItem[]) =>
    apiClient.post<AccountInspectionActionsResponse>('/account-inspection/actions', { items }),
};
