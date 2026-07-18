import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { accountInspectionApi } from '@/services/api/accountInspection';
import type {
  AccountInspectionBackendResponse,
  AccountInspectionBackendRunState,
} from '@/features/monitoring/accountInspection';
import { useNotificationStore } from '@/stores';
import { quotaPersistenceMiddleware } from './persistenceMiddleware';

const POLL_INTERVAL_MS = 1_500;
const ACTIVE_STATES = new Set<AccountInspectionBackendRunState>([
  'running',
  'paused',
  'stopping',
]);

interface BackendQuotaRefreshState {
  active: boolean;
  syncing: boolean;
  completed: number;
  total: number;
}

const EMPTY_STATE: BackendQuotaRefreshState = {
  active: false,
  syncing: false,
  completed: 0,
  total: 0,
};

const isMatchingJob = (response: AccountInspectionBackendResponse, provider: string) =>
  response.status.runKind === 'quota-refresh' && response.status.targetProvider === provider;

export function useBackendQuotaRefresh(provider: string) {
  const { t } = useTranslation();
  const showNotification = useNotificationStore((state) => state.showNotification);
  const [state, setState] = useState<BackendQuotaRefreshState>(EMPTY_STATE);
  const [starting, setStarting] = useState(false);
  const trackedRunRef = useRef(0);
  const finishingRunRef = useRef(0);

  const finishJob = useCallback(
    async (response: AccountInspectionBackendResponse, notify: boolean) => {
      const runID = response.status.lastStartedAt;
      if (!runID || finishingRunRef.current === runID) return;
      finishingRunRef.current = runID;
      const progress = response.status.progress;
      setState({
        active: false,
        syncing: true,
        completed: progress?.completed ?? 0,
        total: progress?.total ?? 0,
      });

      quotaPersistenceMiddleware.markStale();
      await quotaPersistenceMiddleware.ensureFresh();

      const failed = response.status.summary?.errorCount ?? 0;
      if (notify && response.status.state === 'completed') {
        showNotification(
          t('quota_management.refresh_job_completed', {
            completed: progress?.completed ?? 0,
            total: progress?.total ?? 0,
            failed,
          }),
          failed > 0 ? 'warning' : 'success'
        );
      } else if (notify) {
        showNotification(
          t('quota_management.refresh_job_incomplete', {
            completed: progress?.completed ?? 0,
            total: progress?.total ?? 0,
            message: response.status.lastError || response.status.state,
          }),
          'warning'
        );
      }
      trackedRunRef.current = 0;
      setState((current) => ({ ...current, syncing: false }));
    },
    [showNotification, t]
  );

  const applyResponse = useCallback(
    (response: AccountInspectionBackendResponse, attach: boolean, notifyTerminal: boolean) => {
      if (!isMatchingJob(response, provider)) return false;
      const progress = response.status.progress;
      const active = ACTIVE_STATES.has(response.status.state);
      if (active && attach) trackedRunRef.current = response.status.lastStartedAt;
      setState({
        active,
        syncing: false,
        completed: progress?.completed ?? 0,
        total: progress?.total ?? 0,
      });
      if (!active && trackedRunRef.current === response.status.lastStartedAt) {
        void finishJob(response, notifyTerminal);
      } else if (!active && attach && response.status.lastStartedAt) {
        void finishJob(response, false);
      }
      return true;
    },
    [finishJob, provider]
  );

  const poll = useCallback(async (notifyTerminal: boolean) => {
    try {
      const response = await accountInspectionApi.getStatus(false);
      applyResponse(response, true, notifyTerminal);
    } catch {
      // A transient status failure must not detach the UI from a backend job.
    }
  }, [applyResponse]);

  useEffect(() => {
    void poll(false);
  }, [poll]);

  useEffect(() => {
    if (!state.active) return;
    const timer = window.setInterval(() => void poll(true), POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [poll, state.active]);

  const start = useCallback(async () => {
    if (starting || state.active || state.syncing) return;
    setStarting(true);
    try {
      const response = await accountInspectionApi.runQuotaRefresh(provider);
      if (!isMatchingJob(response, provider)) {
        throw new Error(t('quota_management.refresh_job_conflict'));
      }
      trackedRunRef.current = response.status.lastStartedAt;
      applyResponse(response, true, true);
      showNotification(t('quota_management.refresh_job_started'), 'info');
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : t('common.unknown_error');
      showNotification(t('quota_management.refresh_job_failed', { message }), 'error');
    } finally {
      setStarting(false);
    }
  }, [applyResponse, provider, showNotification, starting, state.active, state.syncing, t]);

  return {
    start,
    isRefreshing: starting || state.active || state.syncing,
    completed: state.completed,
    total: state.total,
  };
}
