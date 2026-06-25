import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import {
  ANTIGRAVITY_CONFIG,
  CLAUDE_CONFIG,
  CODEX_CONFIG,
  KIMI_CONFIG,
  XAI_CONFIG,
  type QuotaConfig,
} from '@/components/quota';
import { apiClient } from '@/services/api/client';
import type { AuthFileItem } from '@/types';
import { normalizeAuthIndex } from '@/utils/authIndex';
import { resolveAuthProvider } from '@/utils/quota';

export type BatchActionResultItem = {
  name: string;
  provider: string;
  ok: boolean;
  error?: string;
  skipped?: boolean;
};

export type BatchActionSummary = {
  total: number;
  success: number;
  failed: number;
  skipped: number;
  results: BatchActionResultItem[];
};

const BATCH_CONCURRENCY = 5;

const QUOTA_CONFIG_BY_PROVIDER: Record<string, QuotaConfig<unknown, unknown> | undefined> = {
  antigravity: ANTIGRAVITY_CONFIG as unknown as QuotaConfig<unknown, unknown>,
  claude: CLAUDE_CONFIG as unknown as QuotaConfig<unknown, unknown>,
  codex: CODEX_CONFIG as unknown as QuotaConfig<unknown, unknown>,
  kimi: KIMI_CONFIG as unknown as QuotaConfig<unknown, unknown>,
  xai: XAI_CONFIG as unknown as QuotaConfig<unknown, unknown>,
};

async function runWithConcurrency<T>(
  items: T[],
  limit: number,
  worker: (item: T, index: number) => Promise<void>
): Promise<void> {
  let cursor = 0;
  const workers: Promise<void>[] = [];
  for (let i = 0; i < Math.min(limit, items.length); i++) {
    workers.push(
      (async () => {
        while (true) {
          const index = cursor;
          cursor++;
          if (index >= items.length) break;
          await worker(items[index], index);
        }
      })()
    );
  }
  await Promise.all(workers);
}

async function batchTestCredentials(
  files: AuthFileItem[],
  t: TFunction
): Promise<BatchActionSummary> {
  const results: BatchActionResultItem[] = new Array(files.length);

  await runWithConcurrency(files, BATCH_CONCURRENCY, async (file, index) => {
    const provider = resolveAuthProvider(file);
    const authIndex = normalizeAuthIndex(file['auth_index'] ?? file.authIndex);
    const item: BatchActionResultItem = {
      name: file.name,
      provider,
      ok: false,
    };

    if (!authIndex) {
      item.skipped = true;
      item.error = t('auth_files.batch_no_auth_index');
      results[index] = item;
      return;
    }

    const config = QUOTA_CONFIG_BY_PROVIDER[provider];
    if (!config?.fetchQuota) {
      item.skipped = true;
      item.error = t('auth_files.batch_unsupported_provider', { provider });
      results[index] = item;
      return;
    }

    try {
      await config.fetchQuota(file, t);
      item.ok = true;
    } catch (err) {
      item.ok = false;
      item.error = err instanceof Error ? err.message : String(err);
    }
    results[index] = item;
  });

  return summarizeResults(results);
}

async function batchClearErrorsForFiles(
  files: AuthFileItem[],
  t: TFunction
): Promise<BatchActionSummary> {
  const results: BatchActionResultItem[] = new Array(files.length);

  await runWithConcurrency(files, BATCH_CONCURRENCY, async (file, index) => {
    const provider = resolveAuthProvider(file);
    const authIndex = normalizeAuthIndex(file['auth_index'] ?? file.authIndex);
    const item: BatchActionResultItem = {
      name: file.name,
      provider,
      ok: false,
    };

    if (!authIndex) {
      item.skipped = true;
      item.error = t('auth_files.batch_no_auth_index');
      results[index] = item;
      return;
    }

    try {
      await apiClient.post('/reset-quota', { auth_index: authIndex });
      item.ok = true;
    } catch (err) {
      item.ok = false;
      const message =
        err && typeof err === 'object' && 'message' in err
          ? String((err as { message?: unknown }).message)
          : String(err);
      item.error = message || t('auth_files.batch_clear_errors_failed');
    }
    results[index] = item;
  });

  return summarizeResults(results);
}

function summarizeResults(results: BatchActionResultItem[]): BatchActionSummary {
  let success = 0;
  let failed = 0;
  let skipped = 0;
  for (const item of results) {
    if (item.skipped) {
      skipped++;
    } else if (item.ok) {
      success++;
    } else {
      failed++;
    }
  }
  return { total: results.length, success, failed, skipped, results };
}

export type UseAuthFilesBatchActionsResult = {
  batchTestRunning: boolean;
  batchClearErrorsRunning: boolean;
  batchResultType: 'test' | 'clear' | null;
  batchResult: BatchActionSummary | null;
  batchTest: (files: AuthFileItem[]) => Promise<BatchActionSummary>;
  batchClearErrors: (files: AuthFileItem[]) => Promise<BatchActionSummary>;
  clearBatchResult: () => void;
};

export function useAuthFilesBatchActions(): UseAuthFilesBatchActionsResult {
  const { t } = useTranslation();
  const [batchTestRunning, setBatchTestRunning] = useState(false);
  const [batchClearErrorsRunning, setBatchClearErrorsRunning] = useState(false);
  const [batchResult, setBatchResult] = useState<BatchActionSummary | null>(null);
  const [batchResultType, setBatchResultType] = useState<'test' | 'clear' | null>(null);

  const batchTest = useCallback(
    async (files: AuthFileItem[]): Promise<BatchActionSummary> => {
      setBatchTestRunning(true);
      setBatchResultType('test');
      try {
        const summary = await batchTestCredentials(files, t);
        setBatchResult(summary);
        return summary;
      } finally {
        setBatchTestRunning(false);
      }
    },
    [t]
  );

  const batchClearErrors = useCallback(
    async (files: AuthFileItem[]): Promise<BatchActionSummary> => {
      setBatchClearErrorsRunning(true);
      setBatchResultType('clear');
      try {
        const summary = await batchClearErrorsForFiles(files, t);
        setBatchResult(summary);
        return summary;
      } finally {
        setBatchClearErrorsRunning(false);
      }
    },
    [t]
  );

  const clearBatchResult = useCallback(() => {
    setBatchResult(null);
    setBatchResultType(null);
  }, []);

  return {
    batchTestRunning,
    batchClearErrorsRunning,
    batchResult,
    batchResultType,
    batchTest,
    batchClearErrors,
    clearBatchResult,
  };
}
