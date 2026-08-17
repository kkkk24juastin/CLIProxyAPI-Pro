import { apiClient } from '@/services/api/client';
import type { DataManagementSettings } from './dataManagementSettings';

export type DataSensitivity = 'internal' | 'sensitive' | 'secret';

export interface DataOperation {
  id: number;
  kind: string;
  status: 'running' | 'success' | 'failed';
  target: string;
  fileName: string;
  startedAtMs: number;
  finishedAtMs: number;
  sizeBytes: number;
  affectedRecords: number;
  secretClasses: string[];
  message: string;
  metadata: Record<string, unknown>;
}

export interface DataDomainInventory {
  id: string;
  owner: string;
  schemaVersion: number;
  records: number;
  updatedAtMs: number;
  backupIncluded: boolean;
  restoreMode: string;
  cleanupSupported: boolean;
  sensitivity: DataSensitivity;
  secretClasses: string[];
  available: boolean;
  error?: string;
}

export interface DataManagementOverview {
  service: string;
  dbPath: string;
  dbSizeBytes: number;
  walSizeBytes: number;
  events: number;
  deadLetters: number;
  latestId: number;
  latestTimestampMs: number;
  generation: number;
  resetAtMs: number;
  webdavEnabled: boolean;
  webdavConfigured: boolean;
  lastBackup?: DataOperation;
  domains: DataDomainInventory[];
  secretClasses: string[];
  updatedAtMs: number;
}

export interface WebDAVBackup {
  fileName: string;
  sizeBytes: number;
  lastModified?: string;
  lastModifiedMs?: number;
}

export interface DataBackupHistory {
  backups: WebDAVBackup[];
  operations: DataOperation[];
}

export interface DataRestoreDomainPreview {
  id: string;
  owner: string;
  currentRecords: number;
  backupRecords: number;
  action: string;
  sensitivity: DataSensitivity;
  secretClasses: string[];
  available: boolean;
  error?: string;
}

export interface PolicyBackupPreview {
  hasPolicies: boolean;
  replacePolicies: number;
  preservePolicies: number;
  replaceProfiles: number;
  preserveProfiles: number;
  targetPolicies: number;
  targetProfiles: number;
  associatedPolicies: number;
  orphanedPolicies: number;
  currentTakeoverEnabled: boolean;
  targetTakeoverEnabled: boolean;
}

export interface DataRestorePreview {
  legacyBackup: boolean;
  integrityProtected: boolean;
  encrypted: boolean;
  restoresAPIKeys: false;
  domains: DataRestoreDomainPreview[];
  secretClasses: string[];
  policyBackup?: PolicyBackupPreview;
}

export interface DataCleanupRequest {
  domains: string[];
  retentionDays: number;
  beforeMs?: number;
  expectedRecords?: Record<string, number>;
}

export interface DataCleanupPreview {
  domains: Array<{ id: string; records: number; cutoffMs: number; reclaimable: boolean }>;
  totalRecords: number;
  cutoffMs: number;
}

export interface WebDAVConnectionTestResult {
  connected: boolean;
  writable: boolean;
  deletable: boolean;
  latencyMs: number;
}

export interface DataStatisticsResetResult {
  deletedEvents: number;
  deletedAuthRuntimeStats: number;
  generation: number;
  resetAtMs: number;
}

export const dataManagementApi = {
  overview(): Promise<DataManagementOverview> {
    return apiClient.get<DataManagementOverview>('/data/overview');
  },
  backups(): Promise<DataBackupHistory> {
    return apiClient.get<DataBackupHistory>('/data/backups');
  },
  operations(): Promise<{ operations: DataOperation[] }> {
    return apiClient.get<{ operations: DataOperation[] }>('/data/operations');
  },
  settings(): Promise<{ settings: DataManagementSettings }> {
    return apiClient.get<{ settings: DataManagementSettings }>('/data/settings');
  },
  saveSettings(
    settings: DataManagementSettings,
    expectedSettings: DataManagementSettings,
    sections: Array<'retention' | 'webdav'>
  ): Promise<{ settings: DataManagementSettings }> {
    return apiClient.put<{ settings: DataManagementSettings }>('/data/settings', { settings, expectedSettings, sections });
  },
  backupNow(): Promise<{ backup: WebDAVBackup }> {
    return apiClient.post<{ backup: WebDAVBackup }>('/data/backups/now', {});
  },
  testWebDAV(): Promise<WebDAVConnectionTestResult> {
    return apiClient.post<WebDAVConnectionTestResult>('/data/backups/test', {});
  },
  previewRestore(data: ArrayBuffer, passphrase: string, allowLegacy: boolean): Promise<DataRestorePreview> {
    return apiClient.post<DataRestorePreview>('/data/backups/preview', data, {
      headers: {
        'Content-Type': 'application/octet-stream',
        ...(passphrase ? { 'X-CLIProxy-Backup-Passphrase': passphrase } : {}),
      },
      params: allowLegacy ? { allow_legacy: 1 } : undefined,
    });
  },
  restore(data: ArrayBuffer, passphrase: string, allowLegacy: boolean): Promise<Record<string, unknown>> {
    return apiClient.post<Record<string, unknown>>('/data/backups/restore', data, {
      headers: {
        'Content-Type': 'application/octet-stream',
        ...(passphrase ? { 'X-CLIProxy-Backup-Passphrase': passphrase } : {}),
      },
      params: allowLegacy ? { allow_legacy: 1 } : undefined,
    });
  },
  previewCleanup(request: DataCleanupRequest): Promise<DataCleanupPreview> {
    return apiClient.post<DataCleanupPreview>('/data/maintenance/preview', request);
  },
  executeCleanup(request: DataCleanupRequest): Promise<DataCleanupPreview> {
    return apiClient.post<DataCleanupPreview>('/data/maintenance/execute', request);
  },
  resetStatistics(): Promise<DataStatisticsResetResult> {
    return apiClient.post<DataStatisticsResetResult>('/data/statistics/reset', { confirm: true });
  },
};
