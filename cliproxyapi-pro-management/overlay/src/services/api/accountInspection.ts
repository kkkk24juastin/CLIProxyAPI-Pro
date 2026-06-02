import { apiClient } from './client';
import { MANAGEMENT_API_PREFIX } from '@/utils/constants';
import type {
  AccountInspectionBackendLog as BackendLog,
  AccountInspectionBackendResponse,
  AccountInspectionBackendResultItem,
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

export type AccountInspectionInspectOneItem = Pick<
  AccountInspectionResultItem,
  'key' | 'provider' | 'fileName' | 'email' | 'name' | 'authIndex' | 'disabled'
> & {
  displayName: string;
};

export type AccountInspectionActionItem = Pick<
  AccountInspectionResultItem,
  'key' | 'provider' | 'fileName' | 'email' | 'name' | 'authIndex' | 'disabled'
> & {
  displayName: string;
  action: AccountInspectionExecutionAction;
};

export type AccountInspectionActionsResponse = AccountInspectionBackendResponse & {
  outcomes: AccountInspectionActionOutcome[];
  summary: { total: number; success: number; failed: number };
};

export type AccountInspectionInspectOneResponse = AccountInspectionBackendResponse & {
  result: AccountInspectionBackendResultItem;
  error?: string;
};

export type AccountInspectionRefreshTokenItem = AccountInspectionInspectOneItem;

export type AccountInspectionRefreshTokenResponse = AccountInspectionBackendResponse & {
  result: AccountInspectionBackendResultItem;
  error?: string;
};

export type AccountInspectionScheduleResponse = AccountInspectionBackendResponse;

export type AccountInspectionDetailsOptions = {
  includeDetails?: boolean;
  resultLimit?: number;
  logLimit?: number;
};

const buildAccountInspectionDetailParams = (options: boolean | AccountInspectionDetailsOptions = false) => {
  const normalized = typeof options === 'boolean' ? { includeDetails: options } : options;
  const params: Record<string, number> = { details: normalized.includeDetails ? 1 : 0 };
  if (normalized.resultLimit !== undefined) params.result_limit = normalized.resultLimit;
  if (normalized.logLimit !== undefined) params.log_limit = normalized.logLimit;
  return params;
};

export const buildAccountInspectionLogsWebSocketUrl = (apiBase: string, includeDetails = false) => {
  const base = apiBase.replace(/\/?v0\/management\/?$/i, '').replace(/\/+$/i, '');
  const url = new URL(`${base}${MANAGEMENT_API_PREFIX}/account-inspection/logs`);
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:';
  url.searchParams.set('details', includeDetails ? '1' : '0');
  return url.toString();
};

export const accountInspectionWebSocketProtocol = (managementKey: string) =>
  `cpa-management.${encodeURIComponent(managementKey)}`;

export const accountInspectionApi = {
  getSchedule: (includeDetails = false) =>
    apiClient.get<AccountInspectionScheduleResponse>('/account-inspection/schedule', {
      params: { details: includeDetails ? 1 : 0 },
    }),
  getStatus: (options: boolean | AccountInspectionDetailsOptions = false) =>
    apiClient.get<AccountInspectionScheduleResponse>('/account-inspection/status', {
      params: buildAccountInspectionDetailParams(options),
    }),
  updateSchedule: (schedule: AccountInspectionSchedule) =>
    apiClient.put<AccountInspectionScheduleResponse>('/account-inspection/schedule', schedule, {
      params: { details: 0 },
    }),
  runNow: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/run', {}, {
    params: { details: 0 },
  }),
  inspectOne: (item: AccountInspectionInspectOneItem) =>
    apiClient.post<AccountInspectionInspectOneResponse>('/account-inspection/inspect-one', { item }, {
      params: { details: 1 },
    }),
  refreshToken: (item: AccountInspectionRefreshTokenItem) =>
    apiClient.post<AccountInspectionRefreshTokenResponse>('/account-inspection/refresh-token', { item }, {
      params: { details: 1 },
    }),
  pause: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/pause', {}, {
    params: { details: 0 },
  }),
  resume: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/resume', {}, {
    params: { details: 0 },
  }),
  stop: () => apiClient.post<AccountInspectionScheduleResponse>('/account-inspection/stop', {}, {
    params: { details: 0 },
  }),
  executeActions: (items: AccountInspectionActionItem[]) =>
    apiClient.post<AccountInspectionActionsResponse>('/account-inspection/actions', { items }, {
      params: { details: 1 },
    }),
};
