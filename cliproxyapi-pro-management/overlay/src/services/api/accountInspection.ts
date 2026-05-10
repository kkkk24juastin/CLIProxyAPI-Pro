import { apiClient } from './client';
import { MANAGEMENT_API_PREFIX } from '@/utils/constants';
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
};

export type AccountInspectionBackendStatus = {
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
  logs: AccountInspectionBackendLog[] | null;
  results: Array<
    Omit<AccountInspectionResultItem, 'displayAccount' | 'accountId' | 'status' | 'state' | 'raw'> & {
      displayName: string;
      email?: string;
      name?: string;
      executed?: boolean;
      executeError?: string;
    }
  > | null;
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
  email?: string;
  name?: string;
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

export const buildAccountInspectionLogsWebSocketUrl = (apiBase: string) => {
  const base = apiBase.replace(/\/?v0\/management\/?$/i, '').replace(/\/+$/i, '');
  const url = new URL(`${base}${MANAGEMENT_API_PREFIX}/account-inspection/logs`);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  return url.toString();
};

export const accountInspectionWebSocketProtocol = (managementKey: string) =>
  `cpa-management.${encodeURIComponent(managementKey)}`;

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
