import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { Modal } from '@/components/ui/Modal';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import {
  IconChevronDown,
  IconChevronUp,
  IconExternalLink,
  IconShield,
} from '@/components/ui/icons';
import {
  applyAccountInspectionExecutionResult,
  buildAccountInspectionError,
  buildExecutionFailureMessage,
  clearAccountInspectionConfigurableSettings,
  createAccountInspectionSession,
  DEFAULT_ACCOUNT_INSPECTION_SETTINGS,
  executeAccountInspectionActions,
  formatAccountInspectionIdentity,
  getAutoExecutableAccountInspectionItems,
  hasAccountInspectionAutoExecutePolicies,
  isAccountInspectionStoppedError,
  isSuggestedAction,
  loadAccountInspectionConfigurableSettings,
  saveAccountInspectionConfigurableSettings,
  type AccountInspectionAction,
  type AccountInspectionAutoErrorAction,
  type AccountInspectionConfigurableSettings,
  type AccountInspectionLogLevel,
  type AccountInspectionProgressSnapshot,
  type AccountInspectionResultItem,
  type AccountInspectionRunResult,
  type AccountInspectionSession,
} from '@/features/monitoring/accountInspection';
import { accountInspectionApi, type AccountInspectionScheduleResponse } from '@/services/api';
import { sqliteQuotaCache } from '@/extensions/quota/sqliteQuotaCache';
import { useAuthStore, useConfigStore, useNotificationStore, useQuotaStore } from '@/stores';
import styles from './AccountInspectionPage.module.scss';

type RunStatus = 'idle' | 'running' | 'paused' | 'success' | 'error';

type InspectionLogEntry = {
  id: string;
  level: AccountInspectionLogLevel;
  message: string;
  timestamp: number;
};

type ExecutionTriggerSource = 'manual' | 'auto';

type SummaryCard = {
  key: string;
  label: string;
  value: string;
  tone?: 'neutral' | 'good' | 'warn' | 'bad';
};

type InspectionSettingsDraft = {
  targetType: string;
  workers: string;
  deleteWorkers: string;
  timeout: string;
  retries: string;
  usedPercentThreshold: string;
  sampleSize: string;
  autoExecuteQuotaLimitDisable: boolean;
  autoExecuteQuotaRecoveryEnable: boolean;
  autoExecuteAccountErrorAction: AccountInspectionAutoErrorAction;
};

type InspectionSettingsDraftField = Exclude<
  keyof InspectionSettingsDraft,
  'autoExecuteQuotaLimitDisable' | 'autoExecuteQuotaRecoveryEnable' | 'autoExecuteAccountErrorAction'
>;

type ScheduleDraft = {
  enabled: boolean;
  intervalMinutes: string;
};

const actionToneClass: Record<AccountInspectionAction, string> = {
  keep: styles.actionKeep,
  delete: styles.actionDelete,
  disable: styles.actionDisable,
  enable: styles.actionEnable,
};

const levelClassMap: Record<AccountInspectionLogLevel, string> = {
  info: styles.logInfo,
  success: styles.logSuccess,
  warning: styles.logWarning,
  error: styles.logError,
};

const summaryToneClass: Record<NonNullable<SummaryCard['tone']>, string> = {
  neutral: '',
  good: styles.summaryGood,
  warn: styles.summaryWarn,
  bad: styles.summaryBad,
};

const INSPECTION_TARGET_OPTIONS = [
  { value: 'all', label: 'All' },
  { value: 'antigravity', label: 'Antigravity' },
  { value: 'claude', label: 'Claude' },
  { value: 'codex', label: 'Codex' },
  { value: 'gemini-cli', label: 'Gemini CLI' },
  { value: 'kimi', label: 'Kimi' },
] as const;

const AUTO_ERROR_ACTION_OPTIONS: Array<{ value: AccountInspectionAutoErrorAction; labelKey: string }> = [
  { value: 'none', labelKey: 'monitoring.account_inspection_settings_account_error_action_none' },
  { value: 'disable', labelKey: 'monitoring.account_inspection_settings_account_error_action_disable' },
  { value: 'delete', labelKey: 'monitoring.account_inspection_settings_account_error_action_delete' },
];

const formatTimestamp = (value: number, locale: string) => new Date(value).toLocaleString(locale);

const formatPercent = (value: number | null) => (value === null ? '--' : `${value.toFixed(1)}%`);

const toSettingsDraft = (settings: AccountInspectionConfigurableSettings): InspectionSettingsDraft => ({
  targetType: settings.targetType,
  workers: String(settings.workers),
  deleteWorkers: String(settings.deleteWorkers),
  timeout: String(settings.timeout),
  retries: String(settings.retries),
  usedPercentThreshold: String(settings.usedPercentThreshold),
  sampleSize: String(settings.sampleSize),
  autoExecuteQuotaLimitDisable: settings.autoExecuteQuotaLimitDisable,
  autoExecuteQuotaRecoveryEnable: settings.autoExecuteQuotaRecoveryEnable,
  autoExecuteAccountErrorAction: settings.autoExecuteAccountErrorAction,
});

const formatActionLabel = (action: AccountInspectionAction, t: ReturnType<typeof useTranslation>['t']) => {
  switch (action) {
    case 'delete':
      return t('monitoring.account_inspection_action_delete');
    case 'disable':
      return t('monitoring.account_inspection_action_disable');
    case 'enable':
      return t('monitoring.account_inspection_action_enable');
    case 'keep':
    default:
      return t('monitoring.account_inspection_action_keep');
  }
};

const formatCurrentStateLabel = (item: AccountInspectionResultItem, t: ReturnType<typeof useTranslation>['t']) => {
  if (item.disabled) return t('monitoring.account_inspection_state_disabled');
  return t('monitoring.account_inspection_state_enabled');
};

const countActions = (items: AccountInspectionResultItem[]) => {
  const summary = {
    delete: 0,
    disable: 0,
    enable: 0,
  };

  items.forEach((item) => {
    if (item.action === 'delete') summary.delete += 1;
    if (item.action === 'disable') summary.disable += 1;
    if (item.action === 'enable') summary.enable += 1;
  });

  return summary;
};

const createIdleProgressSnapshot = (): AccountInspectionProgressSnapshot => ({
  total: 0,
  completed: 0,
  inFlight: 0,
  pending: 0,
  percent: 0,
  status: 'idle',
  summary: {
    totalFiles: 0,
    probeSetCount: 0,
    sampledCount: 0,
    deleteCount: 0,
    disableCount: 0,
    enableCount: 0,
    keepCount: 0,
  },
  startedAt: Date.now(),
  updatedAt: Date.now(),
});

const backendResultToFrontendItem = (
  item: NonNullable<AccountInspectionScheduleResponse['status']['results']>[number]
): AccountInspectionResultItem => ({
  key: item.key,
  fileName: item.fileName,
  displayAccount: item.displayName,
  authIndex: item.authIndex || null,
  accountId: null,
  provider: item.provider,
  disabled: item.disabled,
  status: '',
  state: '',
  raw: {
    name: item.fileName,
    type: item.provider,
    provider: item.provider,
    authIndex: item.authIndex,
    disabled: item.disabled,
  },
  action: item.action,
  actionReason: item.actionReason,
  statusCode: item.statusCode ?? null,
  usedPercent: item.usedPercent ?? null,
  isQuota: item.isQuota,
  error: item.executeError || item.error || '',
});

const refreshQuotaStoreFromBackendResults = async (results: AccountInspectionResultItem[]) => {
  const targets = new Map<string, Set<string>>();
  results.forEach((item) => {
    if (!item.fileName || !item.provider) return;
    if (!targets.has(item.provider)) {
      targets.set(item.provider, new Set());
    }
    targets.get(item.provider)?.add(item.fileName);
  });

  await Promise.all(
    Array.from(targets.entries()).map(async ([provider, fileNameSet]) => {
      const fileNames = Array.from(fileNameSet);
      const cached = await sqliteQuotaCache.batchGet(provider, fileNames);
      if (cached.size === 0) return;

      const quotaStore = useQuotaStore.getState();
      const applyCached = (setter: unknown) => {
        if (typeof setter !== 'function') return;
        (setter as (updater: (previous: Record<string, unknown>) => Record<string, unknown>) => void)((previous) => {
          let changed = false;
          const next = { ...previous };
          cached.forEach((data, fileName) => {
            if (previous[fileName] === data) return;
            next[fileName] = data;
            changed = true;
          });
          return changed ? next : previous;
        });
      };

      if (provider === 'antigravity') applyCached(quotaStore.setAntigravityQuota);
      if (provider === 'claude') applyCached(quotaStore.setClaudeQuota);
      if (provider === 'codex') applyCached(quotaStore.setCodexQuota);
      if (provider === 'gemini-cli') applyCached(quotaStore.setGeminiCliQuota);
      if (provider === 'kimi') applyCached(quotaStore.setKimiQuota);
    })
  );
};

const applyBackendInspectionResponse = (
  response: AccountInspectionScheduleResponse,
  setters: {
    setInspectionSettings: (settings: AccountInspectionConfigurableSettings) => void;
    setSettingsDraft: (draft: InspectionSettingsDraft) => void;
    setScheduleDraft: (draft: ScheduleDraft) => void;
    setScheduleResponse: (response: AccountInspectionScheduleResponse) => void;
    setBackendRunning: (running: boolean) => void;
    setLogs: (logs: InspectionLogEntry[]) => void;
    setResult: (result: AccountInspectionRunResult | null) => void;
    setProgress: (progress: AccountInspectionProgressSnapshot) => void;
  }
) => {
  const settings = response.schedule.settings;
  setters.setInspectionSettings(settings);
  setters.setSettingsDraft(toSettingsDraft(settings));
  setters.setScheduleDraft({
    enabled: response.schedule.enabled,
    intervalMinutes: String(response.schedule.intervalMinutes),
  });
  setters.setScheduleResponse(response);
  setters.setBackendRunning(Boolean(response.status.running));
  setters.setLogs(
    (response.status.logs ?? []).map((entry, index) => ({
      id: `backend-${entry.time}-${index}`,
      level: entry.level,
      message: entry.message,
      timestamp: entry.time,
    }))
  );

  const startedAt = response.status.lastStartedAt || Date.now();
  const finishedAt = response.status.lastFinishedAt || startedAt;
  const backendResults = (response.status.results ?? []).map(backendResultToFrontendItem);
  if (backendResults.length > 0 || response.status.lastFinishedAt > 0) {
    setters.setResult({
      settings: {
        baseUrl: '',
        token: '',
        targetType: settings.targetType,
        workers: settings.workers,
        deleteWorkers: settings.deleteWorkers,
        timeout: settings.timeout,
        retries: settings.retries,
        usedPercentThreshold: settings.usedPercentThreshold,
        sampleSize: settings.sampleSize,
      },
      files: [],
      results: backendResults,
      summary: {
        ...response.status.summary,
        usedPercentThreshold: settings.usedPercentThreshold,
        sampled: settings.sampleSize > 0,
        plannedActionPreview: backendResults
          .filter((item) => item.action !== 'keep')
          .slice(0, 10)
          .map((item) => `${formatAccountInspectionIdentity(item)} -> ${item.action}`),
      },
      startedAt,
      finishedAt,
    });
  }
  setters.setProgress({
    total: response.status.summary.sampledCount ?? backendResults.length,
    completed: response.status.running ? 0 : (response.status.summary.sampledCount ?? backendResults.length),
    inFlight: response.status.running ? 1 : 0,
    pending: 0,
    percent: response.status.running ? 0 : 100,
    status: response.status.running ? 'running' : response.status.lastFinishedAt > 0 ? 'completed' : 'idle',
    summary: response.status.summary,
    startedAt,
    updatedAt: Date.now(),
  });
};

export function AccountInspectionPage() {
  const { t, i18n } = useTranslation();
  const config = useConfigStore((state) => state.config);
  const apiBase = useAuthStore((state) => state.apiBase);
  const managementKey = useAuthStore((state) => state.managementKey);
  const connectionStatus = useAuthStore((state) => state.connectionStatus);
  const showNotification = useNotificationStore((state) => state.showNotification);
  const showConfirmation = useNotificationStore((state) => state.showConfirmation);

  const [inspectionSettings, setInspectionSettings] = useState<AccountInspectionConfigurableSettings>(() =>
    loadAccountInspectionConfigurableSettings(config)
  );
  const [settingsDraft, setSettingsDraft] = useState<InspectionSettingsDraft>(() =>
    toSettingsDraft(loadAccountInspectionConfigurableSettings(config))
  );
  const [isSettingsModalOpen, setIsSettingsModalOpen] = useState(false);
  const [scheduleDraft, setScheduleDraft] = useState<ScheduleDraft>({ enabled: false, intervalMinutes: '360' });
  const [scheduleResponse, setScheduleResponse] = useState<AccountInspectionScheduleResponse | null>(null);
  const [scheduleLoading, setScheduleLoading] = useState(false);
  const [backendRunning, setBackendRunning] = useState(false);
  const [logs, setLogs] = useState<InspectionLogEntry[]>([]);
  const [logsCollapsed, setLogsCollapsed] = useState(false);
  const [runStatus, setRunStatus] = useState<RunStatus>('idle');
  const [progress, setProgress] = useState<AccountInspectionProgressSnapshot>(createIdleProgressSnapshot);
  const [result, setResult] = useState<AccountInspectionRunResult | null>(null);
  const [executing, setExecuting] = useState(false);
  const logCounterRef = useRef(0);
  const sessionRef = useRef<AccountInspectionSession | null>(null);
  const activeSessionIdRef = useRef<string | null>(null);
  const logListRef = useRef<HTMLDivElement | null>(null);
  const refreshedBackendFinishedAtRef = useRef(0);
  const executeItemsRef = useRef<
    ((
      items: AccountInspectionResultItem[],
      options?: { resultOverride?: AccountInspectionRunResult | null; source?: ExecutionTriggerSource }
    ) => Promise<void>) | null
  >(null);

  useEffect(() => {
    const nextSettings = loadAccountInspectionConfigurableSettings(config);
    setInspectionSettings(nextSettings);
    if (!isSettingsModalOpen) {
      setSettingsDraft(toSettingsDraft(nextSettings));
    }
  }, [config, isSettingsModalOpen]);

  const applyBackendResponse = useCallback((response: AccountInspectionScheduleResponse) => {
    applyBackendInspectionResponse(response, {
      setInspectionSettings,
      setSettingsDraft,
      setScheduleDraft,
      setScheduleResponse,
      setBackendRunning,
      setLogs,
      setResult,
      setProgress,
    });

    if (
      !response.status.running &&
      response.status.lastFinishedAt > 0 &&
      refreshedBackendFinishedAtRef.current !== response.status.lastFinishedAt
    ) {
      refreshedBackendFinishedAtRef.current = response.status.lastFinishedAt;
      const backendResults = (response.status.results ?? []).map(backendResultToFrontendItem);
      void refreshQuotaStoreFromBackendResults(backendResults);
    }
  }, []);

  const loadBackendSchedule = useCallback(async () => {
    if (connectionStatus !== 'connected') return;
    try {
      const response = await accountInspectionApi.getSchedule();
      applyBackendResponse(response);
    } catch {
      setScheduleResponse(null);
    }
  }, [applyBackendResponse, connectionStatus]);

  useEffect(() => {
    void loadBackendSchedule();
  }, [loadBackendSchedule]);

  useEffect(() => {
    if (connectionStatus !== 'connected' || !backendRunning) return;
    const timer = window.setInterval(() => {
      void loadBackendSchedule();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [backendRunning, connectionStatus, loadBackendSchedule]);

  const appendLog = useCallback((level: AccountInspectionLogLevel, message: string) => {
    logCounterRef.current += 1;
    setLogs((previous) => [
      ...previous,
      {
        id: `${Date.now()}-${logCounterRef.current}`,
        level,
        message,
        timestamp: Date.now(),
      },
    ]);
  }, []);

  useEffect(() => {
    if (logsCollapsed) return;
    const element = logListRef.current;
    if (!element) return;
    element.scrollTop = element.scrollHeight;
  }, [logs, logsCollapsed]);

  useEffect(() => {
    return () => {
      activeSessionIdRef.current = null;
      sessionRef.current?.stop();
      sessionRef.current = null;
    };
  }, []);

  const attachSessionPromise = useCallback(
    (session: AccountInspectionSession, promise: Promise<AccountInspectionRunResult>, autoExecuteSettings: AccountInspectionConfigurableSettings) => {
      const sessionId = session.id;

      void promise
        .then((nextResult) => {
          if (activeSessionIdRef.current !== sessionId) return;
          const nextActionableResults = nextResult.results.filter(isSuggestedAction);
          const nextAutoExecutableResults = getAutoExecutableAccountInspectionItems(
            nextResult.results,
            autoExecuteSettings
          );
          setResult(nextResult);
          setProgress(session.getProgress());
          setRunStatus('success');
          setLogsCollapsed(true);
          if (hasAccountInspectionAutoExecutePolicies(autoExecuteSettings)) {
            if (nextAutoExecutableResults.length > 0 && executeItemsRef.current) {
              const startedMessage = t('monitoring.account_inspection_auto_execute_started', {
                count: nextAutoExecutableResults.length,
              });
              appendLog('info', startedMessage);
              showNotification(startedMessage, 'info');
              void executeItemsRef.current(nextAutoExecutableResults, {
                resultOverride: {
                  ...nextResult,
                  results: nextResult.results.map(
                    (item) => nextAutoExecutableResults.find((nextItem) => nextItem.key === item.key) ?? item
                  ),
                },
                source: 'auto',
              });
              return;
            }

            const noActionsMessage =
              nextActionableResults.length > 0
                ? t('monitoring.account_inspection_auto_execute_no_policy_actions')
                : t('monitoring.account_inspection_auto_execute_no_actions');
            appendLog('success', noActionsMessage);
            showNotification(noActionsMessage, 'success');
            return;
          }

          showNotification(t('monitoring.account_inspection_run_success'), 'success');
        })
        .catch((error) => {
          if (activeSessionIdRef.current !== sessionId) return;
          if (isAccountInspectionStoppedError(error)) {
            setRunStatus('idle');
            setProgress(createIdleProgressSnapshot());
            return;
          }

          const message = buildAccountInspectionError(
            error instanceof Error ? error.message : String(error || t('common.unknown_error'))
          );
          appendLog('error', message);
          setRunStatus('error');
          setLogsCollapsed(false);
          showNotification(message, 'error');
        });
    },
    [appendLog, showNotification, t]
  );

  const startFreshInspection = useCallback(
    (
      preserveLogs: boolean = false,
      introMessage: string = '',
      options?: {
        autoExecuteSettings?: AccountInspectionConfigurableSettings;
      }
    ) => {
      if (connectionStatus !== 'connected') {
        const message = t('notification.connection_required');
        showNotification(message, 'warning');
        return;
      }

      const autoExecuteSettings = options?.autoExecuteSettings ?? inspectionSettings;

      if (!preserveLogs) {
        setLogs([]);
      }
      if (introMessage) {
        appendLog('info', introMessage);
      }

      setResult(null);
      setRunStatus('running');
      setLogsCollapsed(false);

      const session = createAccountInspectionSession({
        config,
        apiBase,
        managementKey,
        settings: inspectionSettings,
        onLog: (level, message) => {
          if (activeSessionIdRef.current !== session.id) return;
          appendLog(level, message);
        },
        onProgress: (snapshot) => {
          if (activeSessionIdRef.current !== session.id) return;
          setProgress(snapshot);
          if (snapshot.status === 'running') {
            setRunStatus('running');
            return;
          }
          if (snapshot.status === 'paused') {
            setRunStatus('paused');
          }
        },
      });

      sessionRef.current = session;
      activeSessionIdRef.current = session.id;
      setProgress(session.getProgress());
      attachSessionPromise(session, session.start(), autoExecuteSettings);
    },
    [
      apiBase,
      appendLog,
      attachSessionPromise,
      config,
      connectionStatus,
      inspectionSettings,
      managementKey,
      showNotification,
      t,
    ]
  );

  const handleRunInspection = useCallback(() => {
    if (runStatus === 'paused' && sessionRef.current) {
      setLogsCollapsed(false);
      sessionRef.current.resume();
      return;
    }

    startFreshInspection(false);
  }, [runStatus, startFreshInspection]);

  const handleBackendRunNow = useCallback(async () => {
    if (connectionStatus !== 'connected') {
      showNotification(t('notification.connection_required'), 'warning');
      return;
    }
    setScheduleLoading(true);
    try {
      const response = await accountInspectionApi.runNow();
      applyBackendResponse(response);
      showNotification(t('monitoring.account_inspection_backend_run_success'), 'success');
    } catch (error) {
      showNotification(error instanceof Error ? error.message : String(error || t('common.unknown_error')), 'error');
    } finally {
      setScheduleLoading(false);
    }
  }, [applyBackendResponse, connectionStatus, showNotification, t]);


  const handlePauseInspection = useCallback(() => {
    if (runStatus !== 'running') return;
    sessionRef.current?.pause();
  }, [runStatus]);

  const handleStopInspection = useCallback(() => {
    const currentSession = sessionRef.current;
    if (!currentSession) return;

    appendLog('warning', t('monitoring.account_inspection_stopped'));
    activeSessionIdRef.current = null;
    sessionRef.current = null;
    currentSession.stop();
    setRunStatus('idle');
    setProgress(createIdleProgressSnapshot());
    setResult(null);
    setLogsCollapsed(false);
  }, [appendLog, t]);

  const executeItems = useCallback(
    async (
      items: AccountInspectionResultItem[],
      options?: {
        resultOverride?: AccountInspectionRunResult | null;
        source?: ExecutionTriggerSource;
      }
    ) => {
      const currentResult = options?.resultOverride ?? result;
      const source = options?.source ?? 'manual';
      if (!currentResult) return;
      const targets = items.filter(isSuggestedAction);
      if (targets.length === 0) {
        showNotification(t('monitoring.account_inspection_no_pending_actions'), 'info');
        return;
      }

      setExecuting(true);
      setLogsCollapsed(false);
      appendLog('info', t('monitoring.account_inspection_execute_started'));

      try {
        const execution = await executeAccountInspectionActions({
          settings: currentResult.settings,
          items: targets,
          previousFiles: currentResult.files,
          onLog: appendLog,
        });

        const failed = execution.outcomes.filter((item) => !item.success);
        if (failed.length > 0) {
          showNotification(
            `${t('monitoring.account_inspection_execute_partial')}: ${failed
              .slice(0, 2)
              .map(buildExecutionFailureMessage)
              .join('；')}`,
            'warning'
          );
        } else {
          showNotification(t('monitoring.account_inspection_execute_success'), 'success');
        }
        const nextResult = applyAccountInspectionExecutionResult(currentResult, execution);
        setResult(nextResult);

        if (source === 'auto') {
          const successCount = execution.outcomes.filter((item) => item.success).length;
          const failedCount = execution.outcomes.length - successCount;
          const remainingCount = nextResult.results.filter(isSuggestedAction).length;
          const summaryMessage =
            failedCount > 0 || remainingCount > 0
              ? t('monitoring.account_inspection_auto_execute_summary_partial', {
                  total: targets.length,
                  success: successCount,
                  failed: failedCount,
                  remaining: remainingCount,
                })
              : t('monitoring.account_inspection_auto_execute_summary_success', {
                  total: targets.length,
                  success: successCount,
                });
          appendLog(failedCount > 0 || remainingCount > 0 ? 'warning' : 'success', summaryMessage);
          showNotification(summaryMessage, failedCount > 0 || remainingCount > 0 ? 'warning' : 'success');
        }
      } finally {
        setExecuting(false);
      }
    },
    [appendLog, result, showNotification, t]
  );

  useEffect(() => {
    executeItemsRef.current = executeItems;
  }, [executeItems]);

  const actionableResults = useMemo(
    () => (result ? result.results.filter(isSuggestedAction) : []),
    [result]
  );

  const handleExecutePlanned = useCallback(() => {
    if (!result) return;

    const targets = actionableResults;
    const counts = countActions(targets);
    showConfirmation({
      title: t('monitoring.account_inspection_execute_confirm_title'),
      message: t('monitoring.account_inspection_execute_confirm_body', {
        total: targets.length,
        delete: counts.delete,
        disable: counts.disable,
        enable: counts.enable,
      }),
      confirmText: t('monitoring.account_inspection_execute_now'),
      cancelText: t('common.cancel'),
      variant: 'danger',
      onConfirm: () => executeItems(targets),
    });
  }, [actionableResults, executeItems, result, showConfirmation, t]);

  const handleExecuteSingle = useCallback(
    (item: AccountInspectionResultItem) => {
      const actionLabel = formatActionLabel(item.action, t);
      showConfirmation({
        title: t('monitoring.account_inspection_execute_single_title'),
        message: t('monitoring.account_inspection_execute_single_body', {
          account: formatAccountInspectionIdentity(item),
          action: actionLabel,
        }),
        confirmText: actionLabel,
        cancelText: t('common.cancel'),
        variant: item.action === 'delete' ? 'danger' : 'primary',
        onConfirm: () => executeItems([item]),
      });
    },
    [executeItems, showConfirmation, t]
  );

  const summaryCards = useMemo<SummaryCard[]>(() => {
    const summarySource =
      result?.summary ?? (runStatus === 'running' || runStatus === 'paused' ? progress.summary : null);

    if (!summarySource) {
      return [
        { key: 'total', label: t('monitoring.account_inspection_total_accounts'), value: '--' },
        { key: 'sampled', label: t('monitoring.account_inspection_sampled_accounts'), value: '--' },
        { key: 'delete', label: t('monitoring.account_inspection_delete_count'), value: '--' },
        { key: 'disable', label: t('monitoring.account_inspection_disable_count'), value: '--' },
        { key: 'enable', label: t('monitoring.account_inspection_enable_count'), value: '--' },
      ];
    }

    return [
      {
        key: 'total',
        label: t('monitoring.account_inspection_total_accounts'),
        value: String(summarySource.probeSetCount),
      },
      {
        key: 'sampled',
        label: t('monitoring.account_inspection_sampled_accounts'),
        value: String(summarySource.sampledCount),
      },
      {
        key: 'delete',
        label: t('monitoring.account_inspection_delete_count'),
        value: String(summarySource.deleteCount),
        tone: summarySource.deleteCount > 0 ? 'bad' : 'neutral',
      },
      {
        key: 'disable',
        label: t('monitoring.account_inspection_disable_count'),
        value: String(summarySource.disableCount),
        tone: summarySource.disableCount > 0 ? 'warn' : 'neutral',
      },
      {
        key: 'enable',
        label: t('monitoring.account_inspection_enable_count'),
        value: String(summarySource.enableCount),
        tone: summarySource.enableCount > 0 ? 'good' : 'neutral',
      },
    ];
  }, [progress.summary, result, runStatus, t]);

  const pendingActionCount = actionableResults.length;
  const progressLabel =
    progress.total > 0
      ? t('monitoring.account_inspection_progress_status', {
          completed: progress.completed,
          total: progress.total,
          inFlight: progress.inFlight,
          pending: progress.pending,
          percent: progress.percent,
        })
      : t('monitoring.account_inspection_progress_idle');
  const openSettingsModal = useCallback(() => {
    setSettingsDraft(toSettingsDraft(inspectionSettings));
    setIsSettingsModalOpen(true);
  }, [inspectionSettings]);

  const handleSettingsDraftChange = useCallback(
    (field: InspectionSettingsDraftField, value: string) => {
      setSettingsDraft((previous) => ({
        ...previous,
        [field]: value,
      }));
    },
    []
  );

  const handleAutoExecuteQuotaLimitChange = useCallback((value: boolean) => {
    setSettingsDraft((previous) => ({
      ...previous,
      autoExecuteQuotaLimitDisable: value,
    }));
  }, []);

  const handleAutoExecuteQuotaRecoveryChange = useCallback((value: boolean) => {
    setSettingsDraft((previous) => ({
      ...previous,
      autoExecuteQuotaRecoveryEnable: value,
    }));
  }, []);

  const handleAutoExecuteAccountErrorActionChange = useCallback((value: string) => {
    setSettingsDraft((previous) => ({
      ...previous,
      autoExecuteAccountErrorAction:
        value === 'disable' || value === 'delete' ? value : 'none',
    }));
  }, []);

  const parseNonNegativeInteger = useCallback(
    (value: string, label: string, min: number) => {
      const parsed = Number(value.trim());
      if (!Number.isFinite(parsed) || !Number.isInteger(parsed) || parsed < min) {
        throw new Error(t('monitoring.account_inspection_settings_invalid_integer', { field: label, min }));
      }
      return parsed;
    },
    [t]
  );

  const handleSaveSettings = useCallback(async () => {
    const targetType = settingsDraft.targetType.trim().toLowerCase();
    if (!targetType) {
      showNotification(t('monitoring.account_inspection_settings_target_type_required'), 'error');
      return;
    }

    try {
      const nextSettings = saveAccountInspectionConfigurableSettings({
        targetType,
        workers: parseNonNegativeInteger(
          settingsDraft.workers,
          t('monitoring.account_inspection_settings_workers_label'),
          1
        ),
        deleteWorkers: parseNonNegativeInteger(
          settingsDraft.deleteWorkers,
          t('monitoring.account_inspection_settings_delete_workers_label'),
          1
        ),
        timeout: parseNonNegativeInteger(
          settingsDraft.timeout,
          t('monitoring.account_inspection_settings_timeout_label'),
          1
        ),
        retries: parseNonNegativeInteger(
          settingsDraft.retries,
          t('monitoring.account_inspection_settings_retries_label'),
          0
        ),
        sampleSize: parseNonNegativeInteger(
          settingsDraft.sampleSize,
          t('monitoring.account_inspection_settings_sample_size_label'),
          0
        ),
        usedPercentThreshold: (() => {
          const parsed = Number(settingsDraft.usedPercentThreshold.trim());
          if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100) {
            throw new Error(
              t('monitoring.account_inspection_settings_invalid_threshold', {
                field: t('monitoring.account_inspection_settings_used_percent_threshold_label'),
              })
            );
          }
          return parsed;
        })(),
        autoExecuteQuotaLimitDisable: settingsDraft.autoExecuteQuotaLimitDisable,
        autoExecuteQuotaRecoveryEnable: settingsDraft.autoExecuteQuotaRecoveryEnable,
        autoExecuteAccountErrorAction: settingsDraft.autoExecuteAccountErrorAction,
      });

      const intervalMinutes = parseNonNegativeInteger(
        scheduleDraft.intervalMinutes,
        t('monitoring.account_inspection_schedule_interval_label'),
        1
      );
      setScheduleLoading(true);
      const response = await accountInspectionApi.updateSchedule({
        enabled: scheduleDraft.enabled,
        intervalMinutes,
        nextRunAt: scheduleDraft.enabled
          ? (scheduleResponse?.schedule.nextRunAt ?? 0)
          : 0,
        settings: nextSettings,
      });
      applyBackendResponse(response);
      setIsSettingsModalOpen(false);
      showNotification(t('monitoring.account_inspection_settings_saved'), 'success');
    } catch (error) {
      showNotification(error instanceof Error ? error.message : String(error || t('common.unknown_error')), 'error');
    } finally {
      setScheduleLoading(false);
    }
  }, [applyBackendResponse, parseNonNegativeInteger, scheduleDraft.enabled, scheduleDraft.intervalMinutes, scheduleResponse?.schedule.nextRunAt, settingsDraft, showNotification, t]);

  const handleResetSettings = useCallback(() => {
    clearAccountInspectionConfigurableSettings();
    const nextSettings = saveAccountInspectionConfigurableSettings(DEFAULT_ACCOUNT_INSPECTION_SETTINGS);
    setInspectionSettings(nextSettings);
    setSettingsDraft(toSettingsDraft(nextSettings));
    showNotification(t('monitoring.account_inspection_settings_reset'), 'success');
  }, [showNotification, t]);

  return (
    <div className={styles.page}>
      <Card className={styles.heroCard}>
        <div className={styles.heroHeader}>
          <div className={styles.heroCopy}>
            <div className={styles.heroEyebrow}>
              <IconShield size={14} />
              <span>{t('monitoring.account_inspection_eyebrow')}</span>
            </div>
            <h1 className={styles.heroTitle}>{t('monitoring.account_inspection_title')}</h1>
          </div>

          <div className={styles.heroActions}>
            <Link to="/monitoring" className={styles.backLink}>
              <IconExternalLink size={14} />
              <span>{t('monitoring.account_inspection_back')}</span>
            </Link>
            <Button
              variant="secondary"
              onClick={openSettingsModal}
              disabled={(runStatus === 'running' || runStatus === 'paused') || executing}
            >
              {t('monitoring.account_inspection_settings_button')}
            </Button>
            <Button
              variant="secondary"
              onClick={handleBackendRunNow}
              loading={scheduleLoading || backendRunning}
              disabled={scheduleLoading || backendRunning || connectionStatus !== 'connected'}
            >
              {backendRunning
                ? t('monitoring.account_inspection_backend_running')
                : t('monitoring.account_inspection_backend_run_now')}
            </Button>
            <Button
              variant="secondary"
              onClick={handleRunInspection}
              loading={runStatus === 'running'}
              disabled={runStatus === 'running' || executing || connectionStatus !== 'connected'}
            >
              {runStatus === 'paused'
                ? t('monitoring.account_inspection_resume')
                : runStatus === 'running'
                  ? t('monitoring.account_inspection_running')
                  : t('monitoring.account_inspection_run')}
            </Button>
            <Button
              variant="secondary"
              onClick={handlePauseInspection}
              disabled={runStatus !== 'running' || executing}
            >
              {t('monitoring.account_inspection_pause')}
            </Button>
            <Button
              variant="danger"
              onClick={handleStopInspection}
              disabled={(runStatus !== 'running' && runStatus !== 'paused') || executing}
            >
              {t('monitoring.account_inspection_stop')}
            </Button>
          </div>
        </div>

        <div className={styles.metaRow}>
          <span className={styles.metaPill}>{`${t('monitoring.account_inspection_target_type')}: ${inspectionSettings.targetType}`}</span>
          <span className={styles.metaPill}>{`${t('monitoring.account_inspection_threshold')}: ${inspectionSettings.usedPercentThreshold}%`}</span>
          <span className={styles.metaPill}>{`${t('monitoring.account_inspection_workers')}: ${inspectionSettings.workers}`}</span>
          <span className={styles.metaPill}>{`${t('monitoring.account_inspection_delete_workers')}: ${inspectionSettings.deleteWorkers}`}</span>
          <span className={styles.metaPill}>{`${t('monitoring.account_inspection_sample_size')}: ${inspectionSettings.sampleSize}`}</span>
          <span className={styles.metaPill}>
            {`${t('monitoring.account_inspection_settings_auto_execute_quota_limit_disable_label')}: ${
              inspectionSettings.autoExecuteQuotaLimitDisable ? t('common.yes') : t('common.no')
            }`}
          </span>
          <span className={styles.metaPill}>
            {`${t('monitoring.account_inspection_settings_auto_execute_quota_recovery_enable_label')}: ${
              inspectionSettings.autoExecuteQuotaRecoveryEnable ? t('common.yes') : t('common.no')
            }`}
          </span>
          <span className={styles.metaPill}>
            {`${t('monitoring.account_inspection_settings_auto_execute_account_error_action_label')}: ${
              t(`monitoring.account_inspection_settings_account_error_action_${inspectionSettings.autoExecuteAccountErrorAction}`)
            }`}
          </span>
          <span className={styles.metaPill}>
            {`${t('monitoring.account_inspection_schedule_status')}: ${
              scheduleResponse?.schedule.enabled ? t('common.yes') : t('common.no')
            }`}
          </span>
          {scheduleResponse?.schedule.enabled ? (
            <span className={styles.metaPill}>
              {`${t('monitoring.account_inspection_schedule_next_run')}: ${
                scheduleResponse.schedule.nextRunAt ? formatTimestamp(scheduleResponse.schedule.nextRunAt, i18n.language) : '--'
              }`}
            </span>
          ) : null}
          <span className={styles.metaPill}>{`${t('monitoring.account_inspection_timeout')}: ${inspectionSettings.timeout}ms`}</span>
        </div>
        <div className={styles.progressSection}>
          <div className={styles.progressHeader}>
            <strong>{t('monitoring.account_inspection_progress_title')}</strong>
            <span>{`${progress.percent}%`}</span>
          </div>
          <div className={styles.progressTrack}>
            <span className={styles.progressBar} style={{ width: `${Math.max(0, Math.min(100, progress.percent))}%` }} />
          </div>
          <div className={styles.progressMeta}>
            <span>{progressLabel}</span>
            {runStatus === 'paused' ? <strong>{t('monitoring.account_inspection_paused')}</strong> : null}
          </div>
        </div>
      </Card>

      <section className={styles.summaryGrid}>
        {summaryCards.map((card) => (
          <Card
            key={card.key}
            className={[styles.summaryCard, summaryToneClass[card.tone ?? 'neutral']]
              .filter(Boolean)
              .join(' ')}
          >
            <span>{card.label}</span>
            <strong>{card.value}</strong>
          </Card>
        ))}
      </section>

      <Card className={styles.panel}>
        <div className={styles.panelHeader}>
          <div>
            <h2 className={styles.panelTitle}>{t('monitoring.account_inspection_logs_title')}</h2>
            <p className={styles.panelSubtitle}>{t('monitoring.account_inspection_logs_desc')}</p>
          </div>
          <div className={styles.panelActions}>
            <button
              type="button"
              className={styles.foldButton}
              onClick={() => setLogsCollapsed((previous) => !previous)}
              disabled={logs.length === 0}
            >
              {logsCollapsed ? <IconChevronDown size={16} /> : <IconChevronUp size={16} />}
              <span>
                {logsCollapsed
                  ? t('monitoring.account_inspection_expand_logs')
                  : t('monitoring.account_inspection_fold_logs')}
              </span>
            </button>
          </div>
        </div>

        {!logsCollapsed ? (
          <div ref={logListRef} className={styles.logList}>
            {logs.length > 0 ? (
              logs.map((entry) => (
                <div key={entry.id} className={`${styles.logRow} ${levelClassMap[entry.level]}`}>
                  <span className={styles.logTime}>{formatTimestamp(entry.timestamp, i18n.language)}</span>
                  <span className={styles.logMessage}>{entry.message}</span>
                </div>
              ))
            ) : (
              <div className={styles.emptyBlock}>{t('monitoring.account_inspection_logs_empty')}</div>
            )}
          </div>
        ) : (
          <div className={styles.logCollapsedBar}>
            <span>{t('monitoring.account_inspection_logs_collapsed', { count: logs.length })}</span>
          </div>
        )}
      </Card>

      <Card className={styles.panel}>
        <div className={styles.panelHeader}>
          <div>
            <h2 className={styles.panelTitle}>{t('monitoring.account_inspection_results_title')}</h2>
            <p className={styles.panelSubtitle}>{t('monitoring.account_inspection_results_desc')}</p>
          </div>
          <div className={styles.resultsHeaderActions}>
            {result ? (
              <div className={styles.panelMeta}>
                <span>{`${t('monitoring.last_sync')}: ${formatTimestamp(result.finishedAt, i18n.language)}`}</span>
                <span>{`${t('monitoring.account_inspection_pending_actions')}: ${pendingActionCount}`}</span>
              </div>
            ) : null}
            <Button
              variant="primary"
              size="sm"
              onClick={handleExecutePlanned}
              loading={executing}
              disabled={!result || runStatus === 'running' || executing || pendingActionCount === 0}
            >
              {executing
                ? t('monitoring.account_inspection_executing')
                : t('monitoring.account_inspection_execute_now')}
            </Button>
          </div>
        </div>

        {result ? (
          <>
            <div className={styles.tableWrap}>
              <table className={styles.table}>
                <colgroup>
                  <col className={styles.accountColumn} />
                  <col className={styles.stateColumn} />
                  <col className={styles.httpColumn} />
                  <col className={styles.usageColumn} />
                  <col className={styles.actionColumn} />
                  <col className={styles.reasonColumn} />
                  <col className={styles.errorColumn} />
                  <col className={styles.operationColumn} />
                </colgroup>
                <thead>
                  <tr>
                    <th>{t('monitoring.account_label')}</th>
                    <th>{t('monitoring.account_inspection_current_state')}</th>
                    <th>{t('monitoring.account_inspection_http_status')}</th>
                    <th>{t('monitoring.account_inspection_used_percent')}</th>
                    <th>{t('monitoring.account_inspection_next_action')}</th>
                    <th>{t('monitoring.account_inspection_reason')}</th>
                    <th>{t('monitoring.account_inspection_error')}</th>
                    <th>{t('common.action')}</th>
                  </tr>
                </thead>
                <tbody>
                  {actionableResults.length > 0 ? (
                    actionableResults.map((item) => (
                    <tr key={item.key}>
                      <td>
                        <div className={styles.primaryCell}>
                          <span>{item.displayAccount}</span>
                          <small>{`${item.provider} · ${item.fileName}`}</small>
                          <small>{item.authIndex ? `auth ${item.authIndex}` : '-'}</small>
                        </div>
                      </td>
                      <td>{formatCurrentStateLabel(item, t)}</td>
                      <td>{item.statusCode === null ? '--' : item.statusCode}</td>
                      <td>{formatPercent(item.usedPercent)}</td>
                      <td>
                        <span className={`${styles.actionBadge} ${actionToneClass[item.action]}`}>
                          {formatActionLabel(item.action, t)}
                        </span>
                      </td>
                      <td>{item.actionReason}</td>
                      <td className={item.error ? styles.errorText : styles.mutedText}>{item.error || '--'}</td>
                      <td>
                        <Button
                          size="sm"
                          variant={item.action === 'delete' ? 'danger' : 'secondary'}
                          onClick={() => handleExecuteSingle(item)}
                          disabled={runStatus === 'running' || executing}
                        >
                          {formatActionLabel(item.action, t)}
                        </Button>
                      </td>
                    </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={8}>
                        <div className={styles.emptyBlockSmall}>{t('monitoring.account_inspection_no_pending_actions')}</div>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </>
        ) : (
          <div className={styles.emptyBlock}>{t('monitoring.account_inspection_empty')}</div>
        )}
      </Card>

      <Modal
        open={isSettingsModalOpen}
        onClose={() => setIsSettingsModalOpen(false)}
        title={t('monitoring.account_inspection_settings_title')}
        width={920}
        className={styles.settingsModal}
      >
        <div className={styles.settingsIntro}>
          <strong>{t('monitoring.account_inspection_settings_title')}</strong>
          <span>{t('monitoring.account_inspection_settings_desc')}</span>
        </div>

        <section className={styles.settingsSection}>
          <div className={styles.settingsSectionHeader}>
            <div>
              <strong>{t('monitoring.account_inspection_schedule_section_title')}</strong>
              <span>{t('monitoring.account_inspection_schedule_section_desc')}</span>
            </div>
            <ToggleSwitch
              checked={scheduleDraft.enabled}
              onChange={(value) => setScheduleDraft((previous) => ({ ...previous, enabled: value }))}
              ariaLabel={t('monitoring.account_inspection_schedule_enabled_label')}
            />
          </div>
          <div className={styles.settingsGrid}>
            <div className={styles.settingsField}>
              <Input
                label={t('monitoring.account_inspection_schedule_interval_label')}
                type="number"
                value={scheduleDraft.intervalMinutes}
                onChange={(event) => setScheduleDraft((previous) => ({ ...previous, intervalMinutes: event.target.value }))}
                min={1}
                step={1}
              />
              <div className={styles.settingsHint}>{t('monitoring.account_inspection_schedule_interval_hint')}</div>
            </div>
            <div className={styles.settingsFieldWide}>
              <div className={styles.settingsHint}>
                {`${t('monitoring.account_inspection_schedule_next_run')}: ${
                  scheduleResponse?.schedule.nextRunAt ? formatTimestamp(scheduleResponse.schedule.nextRunAt, i18n.language) : '--'
                }`}
              </div>
            </div>
          </div>
        </section>

        <section className={styles.settingsSection}>
          <div className={styles.settingsSectionHeader}>
            <div>
              <strong>{t('monitoring.account_inspection_settings_basic_section_title')}</strong>
              <span>{t('monitoring.account_inspection_settings_basic_section_desc')}</span>
            </div>
          </div>
          <div className={styles.settingsGrid}>
            <div className={styles.settingsField}>
              <label className={styles.settingsLabel}>{t('monitoring.account_inspection_settings_target_type_label')}</label>
              <Select
                value={settingsDraft.targetType}
                options={INSPECTION_TARGET_OPTIONS}
                onChange={(value) => handleSettingsDraftChange('targetType', value)}
                ariaLabel={t('monitoring.account_inspection_settings_target_type_label')}
              />
            </div>
            <div className={styles.settingsField}>
              <Input
                label={t('monitoring.account_inspection_settings_workers_label')}
                type="number"
                value={settingsDraft.workers}
                onChange={(event) => handleSettingsDraftChange('workers', event.target.value)}
                min={1}
                step={1}
              />
            </div>
            <div className={styles.settingsField}>
              <Input
                label={t('monitoring.account_inspection_settings_delete_workers_label')}
                type="number"
                value={settingsDraft.deleteWorkers}
                onChange={(event) => handleSettingsDraftChange('deleteWorkers', event.target.value)}
                min={1}
                step={1}
              />
            </div>
            <div className={styles.settingsField}>
              <Input
                label={t('monitoring.account_inspection_settings_timeout_label')}
                type="number"
                value={settingsDraft.timeout}
                onChange={(event) => handleSettingsDraftChange('timeout', event.target.value)}
                min={1}
                step={100}
              />
            </div>
            <div className={styles.settingsField}>
              <Input
                label={t('monitoring.account_inspection_settings_retries_label')}
                type="number"
                value={settingsDraft.retries}
                onChange={(event) => handleSettingsDraftChange('retries', event.target.value)}
                min={0}
                step={1}
              />
            </div>
            <div className={styles.settingsField}>
              <Input
                label={t('monitoring.account_inspection_settings_used_percent_threshold_label')}
                hint={t('monitoring.account_inspection_settings_threshold_hint')}
                type="number"
                value={settingsDraft.usedPercentThreshold}
                onChange={(event) => handleSettingsDraftChange('usedPercentThreshold', event.target.value)}
                min={0}
                max={100}
                step={0.1}
              />
            </div>
            <div className={styles.settingsField}>
              <Input
                label={t('monitoring.account_inspection_settings_sample_size_label')}
                hint={t('monitoring.account_inspection_settings_sample_size_hint')}
                type="number"
                value={settingsDraft.sampleSize}
                onChange={(event) => handleSettingsDraftChange('sampleSize', event.target.value)}
                min={0}
                step={1}
              />
            </div>
          </div>
        </section>

        <section className={styles.settingsSection}>
          <div className={styles.settingsSectionHeader}>
            <div>
              <strong>{t('monitoring.account_inspection_settings_auto_section_title')}</strong>
              <span>{t('monitoring.account_inspection_settings_auto_section_desc')}</span>
            </div>
          </div>
          <div className={styles.settingsPolicyGrid}>
            <div className={styles.settingsPolicyCard}>
              <div className={styles.settingsPolicyControl}>
                <ToggleSwitch
                  checked={settingsDraft.autoExecuteQuotaLimitDisable}
                  onChange={handleAutoExecuteQuotaLimitChange}
                  label={t('monitoring.account_inspection_settings_auto_execute_quota_limit_disable_label')}
                  ariaLabel={t('monitoring.account_inspection_settings_auto_execute_quota_limit_disable_label')}
                  labelPosition="left"
                />
              </div>
              <span className={styles.settingsHint}>
                {t('monitoring.account_inspection_settings_auto_execute_quota_limit_disable_hint')}
              </span>
            </div>
            <div className={styles.settingsPolicyCard}>
              <div className={styles.settingsPolicyControl}>
                <ToggleSwitch
                  checked={settingsDraft.autoExecuteQuotaRecoveryEnable}
                  onChange={handleAutoExecuteQuotaRecoveryChange}
                  label={t('monitoring.account_inspection_settings_auto_execute_quota_recovery_enable_label')}
                  ariaLabel={t('monitoring.account_inspection_settings_auto_execute_quota_recovery_enable_label')}
                  labelPosition="left"
                />
              </div>
              <span className={styles.settingsHint}>
                {t('monitoring.account_inspection_settings_auto_execute_quota_recovery_enable_hint')}
              </span>
            </div>
            <div className={styles.settingsPolicyCard}>
              <label className={styles.settingsLabel}>
                {t('monitoring.account_inspection_settings_auto_execute_account_error_action_label')}
              </label>
              <Select
                value={settingsDraft.autoExecuteAccountErrorAction}
                options={AUTO_ERROR_ACTION_OPTIONS.map((option) => ({
                  value: option.value,
                  label: t(option.labelKey),
                }))}
                onChange={handleAutoExecuteAccountErrorActionChange}
                ariaLabel={t('monitoring.account_inspection_settings_auto_execute_account_error_action_label')}
              />
              <span className={styles.settingsHint}>
                {t('monitoring.account_inspection_settings_auto_execute_account_error_action_hint')}
              </span>
            </div>
          </div>
        </section>

        <div className={styles.settingsActionsBar}>
          <Button variant="secondary" onClick={handleResetSettings}>
            {t('monitoring.account_inspection_settings_reset_button')}
          </Button>
          <Button variant="secondary" onClick={() => setIsSettingsModalOpen(false)}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" onClick={() => void handleSaveSettings()} loading={scheduleLoading}>
            {t('common.save')}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
