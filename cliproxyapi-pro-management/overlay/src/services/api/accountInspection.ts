import { apiClient } from './client';
import { MANAGEMENT_API_PREFIX } from '@/utils/constants';
import type {
  AccountInspectionBackendLog as BackendLog,
  AccountInspectionBackendResponse,
  AccountInspectionBackendSchedule,
  AccountInspectionBackendStatus,
  AccountInspectionExecutionAction,
  AccountInspectionResultItem,
} from '@/features/monitoring/accountInspection';

export type AccountInspectionSchedule = AccountInspectionBackendSchedule;

export type AccountInspectionBackendLog = BackendLog;

export type AccountInspectionLogStreamMessage = {
  type: 'snapshot' | 'log' | 'status';
  schedule: AccountInspectionSchedule;
  status: AccountInspectionBackendStatus;
  log?: AccountInspectionBackendLog;
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

export type AccountInspectionActionItem = Pick<
  AccountInspectionResultItem,
  'key' | 'provider' | 'fileName' | 'email' | 'name' | 'authIndex' | 'disabled'
> & {
  displayName: string;
  action: AccountInspectionExecutionAction;
};

export type AccountInspectionActionsResponse = {
  outcomes: AccountInspectionActionOutcome[];
  summary: { total: number; success: number; failed: number };
  status: AccountInspectionBackendStatus;
};

export type AccountInspectionScheduleResponse = AccountInspectionBackendResponse;

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
  executeActions: (items: AccountInspectionActionItem[]) =>
    apiClient.post<AccountInspectionActionsResponse>('/account-inspection/actions', { items }),
};
