import { useCallback, useDeferredValue, useEffect, useId, useMemo, useRef, useState, type ChangeEvent, type CSSProperties, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Input } from '@/components/ui/Input';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { Modal } from '@/components/ui/Modal';
import { Select } from '@/components/ui/Select';
import {
  IconChevronDown,
  IconChevronUp,
  IconRefreshCw,
  IconSearch,
  IconSlidersHorizontal,
  IconTimer,
} from '@/components/ui/icons';
import {
  buildAccountRows,
  buildMonitoringSummary,
  useMonitoringData,
  type MonitoringAccountRow,
  type MonitoringEventRow,
  type MonitoringStatusTone,
  type MonitoringTimeRange,
} from '@/features/monitoring/hooks/useMonitoringData';
import { useUsageData } from '@/features/monitoring/hooks/useUsageData';
import { useHeaderRefresh } from '@/hooks/useHeaderRefresh';
import { useInterval } from '@/hooks/useInterval';
import { apiClient } from '@/services/api/client';
import { useAuthStore, useConfigStore, useNotificationStore, useQuotaStore } from '@/stores';
import type { AuthFileItem } from '@/types';
import { maskSensitiveText } from '@/utils/format';
import { getStatusFromError } from '@/utils/quota';
import { formatCompactNumber, formatDurationMs, formatUsd, normalizeAuthIndex, type ModelPrice } from '@/utils/usage';
import {
  ANTIGRAVITY_CONFIG,
  CLAUDE_CONFIG,
  CODEX_CONFIG,
  GEMINI_CLI_CONFIG,
  KIMI_CONFIG,
  type QuotaConfig,
  type QuotaStore,
} from '@/components/quota/quotaConfigs';
import { QuotaProgressBar, type QuotaRenderHelpers, type QuotaStatusState } from '@/components/quota/QuotaCard';
import quotaStyles from '@/pages/QuotaPage.module.scss';
import styles from './MonitoringCenterPage.module.scss';

const TIME_RANGE_OPTIONS: Array<{ value: MonitoringTimeRange; labelKey: string }> = [
  { value: 'today', labelKey: 'monitoring.range_today' },
  { value: '7d', labelKey: 'monitoring.range_7d' },
  { value: '14d', labelKey: 'monitoring.range_14d' },
  { value: '30d', labelKey: 'monitoring.range_30d' },
  { value: 'all', labelKey: 'monitoring.range_all' },
];

const AUTO_REFRESH_OPTIONS = [
  { value: '0', labelKey: 'monitoring.auto_refresh_off' },
  { value: '5000', labelKey: 'monitoring.auto_refresh_5s' },
  { value: '10000', labelKey: 'monitoring.auto_refresh_10s' },
  { value: '30000', labelKey: 'monitoring.auto_refresh_30s' },
  { value: '60000', labelKey: 'monitoring.auto_refresh_60s' },
  { value: '300000', labelKey: 'monitoring.auto_refresh_5m' },
];

type StatusFilter = 'all' | 'success' | 'failed';

type PanelProps = {
  title: string;
  subtitle?: string;
  extra?: ReactNode;
  children: ReactNode;
  className?: string;
};

type UsageMetricCard = {
  key: string;
  title: string;
  label: string;
  value: ReactNode;
  accent: 'blue' | 'purple' | 'green' | 'amber';
  footer: Array<{ label: string; value: ReactNode }>;
};

type RankingMetric = 'requests' | 'tokens' | 'cost';

const RANKING_METRIC_OPTIONS: Array<{ value: RankingMetric; label: string }> = [
  { value: 'requests', label: '请求' },
  { value: 'tokens', label: 'TOKEN' },
  { value: 'cost', label: '金额' },
];

const ACCOUNT_STATUS_BLOCK_COUNT = 20;
const ACCOUNT_STATUS_BLOCK_DURATION_MS = 10 * 60 * 1000;
const ACCOUNT_STATUS_COLOR_STOPS = [
  { r: 239, g: 68, b: 68 },
  { r: 250, g: 204, b: 21 },
  { r: 34, g: 197, b: 94 },
] as const;

const getAccountStatusColor = (rate: number) => {
  const value = Math.max(0, Math.min(1, rate));
  const segment = value < 0.5 ? 0 : 1;
  const localValue = segment === 0 ? value * 2 : (value - 0.5) * 2;
  const from = ACCOUNT_STATUS_COLOR_STOPS[segment];
  const to = ACCOUNT_STATUS_COLOR_STOPS[segment + 1];
  const r = Math.round(from.r + (to.r - from.r) * localValue);
  const g = Math.round(from.g + (to.g - from.g) * localValue);
  const b = Math.round(from.b + (to.b - from.b) * localValue);
  return `rgb(${r}, ${g}, ${b})`;
};

const formatStatusRate = (rate: number) => {
  const rounded = rate.toFixed(1);
  return `${rounded.endsWith('.0') ? rounded.slice(0, -2) : rounded}%`;
};

const formatStatusWindowLabel = (startTime: number, endTime: number, locale: string) => {
  const start = new Date(startTime);
  const end = new Date(endTime);
  const sameDay = start.toDateString() === end.toDateString();
  const dateOptions: Intl.DateTimeFormatOptions = { month: 'numeric', day: 'numeric' };
  const timeOptions: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' };
  const startDateLabel = start.toLocaleDateString(locale, dateOptions);
  const endDateLabel = end.toLocaleDateString(locale, dateOptions);
  const startTimeLabel = start.toLocaleTimeString(locale, timeOptions);
  const endTimeLabel = end.toLocaleTimeString(locale, timeOptions);

  return sameDay
    ? `${startDateLabel} ${startTimeLabel} - ${endTimeLabel}`
    : `${startDateLabel} ${startTimeLabel} - ${endDateLabel} ${endTimeLabel}`;
};

const buildAccountStatusBlockAriaLabel = ({
  detail,
  timeRangeLabel,
  successRateValue,
  copy,
}: {
  detail: AccountStatusBlockDetail;
  timeRangeLabel: string;
  successRateValue: string;
  copy: {
    successLabel: string;
    failureLabel: string;
    noRequestsLabel: string;
    successRateLabel: string;
  };
}) => {
  const total = detail.success + detail.failure;
  if (total === 0) {
    return `${timeRangeLabel}，${copy.noRequestsLabel}`;
  }
  return `${timeRangeLabel}，${copy.successLabel} ${detail.success}，${copy.failureLabel} ${detail.failure}，${copy.successRateLabel} ${successRateValue}`;
};

const getNextAccountStatusBlockIndex = (currentIndex: number, key: string, count: number) => {
  if (count <= 0) return null;
  if (key === 'ArrowRight' || key === 'ArrowDown') return Math.min(currentIndex + 1, count - 1);
  if (key === 'ArrowLeft' || key === 'ArrowUp') return Math.max(currentIndex - 1, 0);
  if (key === 'Home') return 0;
  if (key === 'End') return count - 1;
  return null;
};

const formatAccountOverviewScopeText = (rangeLabel: string) => `统计范围：${rangeLabel}`;

const buildAccountStatusData = (pattern: boolean[], lastSeenAt: number): AccountStatusData => {
  const normalized = pattern.slice(-ACCOUNT_STATUS_BLOCK_COUNT);
  const emptyBlockCount = Math.max(0, ACCOUNT_STATUS_BLOCK_COUNT - normalized.length);
  const blocks = [
    ...Array.from({ length: emptyBlockCount }, () => null),
    ...normalized,
  ];
  const anchorEnd = Number.isFinite(lastSeenAt) ? lastSeenAt : Date.now();
  const windowStart = anchorEnd - ACCOUNT_STATUS_BLOCK_COUNT * ACCOUNT_STATUS_BLOCK_DURATION_MS;
  let totalSuccess = 0;
  let totalFailure = 0;

  const blockDetails = blocks.map((item, index) => {
    const success = item === true ? 1 : 0;
    const failure = item === false ? 1 : 0;
    const total = success + failure;
    totalSuccess += success;
    totalFailure += failure;

    return {
      success,
      failure,
      rate: total > 0 ? success / total : -1,
      startTime: windowStart + index * ACCOUNT_STATUS_BLOCK_DURATION_MS,
      endTime: windowStart + (index + 1) * ACCOUNT_STATUS_BLOCK_DURATION_MS,
    };
  });
  const total = totalSuccess + totalFailure;

  return {
    blockDetails,
    successRate: total > 0 ? (totalSuccess / total) * 100 : 100,
    totalSuccess,
    totalFailure,
  };
};

const DONUT_COLORS = ['#2563eb', '#22c55e', '#f97316', '#8b5cf6', '#06b6d4', '#ec4899', '#14b8a6', '#eab308'];

const getDonutColor = (index: number) => DONUT_COLORS[index % DONUT_COLORS.length];

type TrendPoint = {
  key: string;
  label: string;
  requests: number;
  failures: number;
  tokens: number;
  cost: number;
};

type TokenDistributionPoint = {
  key: string;
  label: string;
  requests: number;
  totalTokens: number;
  inputTokens: number;
  outputTokens: number;
  reasoningTokens: number;
  cachedTokens: number;
};

type AccountHealthTone = 'good' | 'warn' | 'bad';

type AccountStatusBlockDetail = {
  success: number;
  failure: number;
  rate: number;
  startTime: number;
  endTime: number;
};

type AccountStatusData = {
  blockDetails: AccountStatusBlockDetail[];
  successRate: number;
  totalSuccess: number;
  totalFailure: number;
};

type PriceDraft = {
  prompt: string;
  completion: string;
  cache: string;
};

type RealtimeLogRow = MonitoringEventRow & {
  requestCount: number;
  successRate: number;
  streamKey: string;
  recentPattern: boolean[];
};

type UsageImportResult = {
  added?: number;
  skipped?: number;
  total?: number;
  failed?: number;
};


const SuccessFailureValue = ({ success, failure }: { success: number; failure: number }) => (
  <span className={styles.successFailureValue}>
    <span className={styles.goodText}>{formatCompactNumber(success)}</span>
    <span className={styles.badText}>({formatCompactNumber(failure)})</span>
  </span>
);

const getRankingMetricValue = (row: MonitoringAccountRow, metric: RankingMetric) => {
  if (metric === 'cost') return row.totalCost;
  if (metric === 'tokens') return row.totalTokens;
  return row.totalCalls;
};

const getRankingMetricLabel = (metric: RankingMetric) => {
  if (metric === 'cost') return '金额';
  if (metric === 'tokens') return 'TOKEN';
  return '请求';
};

const getRankingSummaryLabel = (metric: RankingMetric) => {
  if (metric === 'cost') return '总金额';
  if (metric === 'tokens') return '总 TOKEN';
  return '总调用';
};

const getRankingMetricTotalFromRows = (rows: MonitoringEventRow[], metric: RankingMetric) => {
  if (metric === 'cost') return rows.reduce((sum, row) => sum + row.totalCost, 0);
  if (metric === 'tokens') return rows.reduce((sum, row) => sum + row.totalTokens, 0);
  return rows.reduce((sum, row) => sum + (row.statsIncluded ? 1 : 0), 0);
};

const formatRankingMetricValue = (value: number, metric: RankingMetric, hasPrices: boolean) => {
  if (metric === 'cost') return hasPrices ? formatUsd(value) : '--';
  return formatCompactNumber(value);
};

const getAccountHealthTone = (row: MonitoringAccountRow): AccountHealthTone => {
  if (row.successRate >= 0.95) return 'good';
  if (row.successRate >= 0.85) return 'warn';
  return 'bad';
};

const RankingMetricSwitch = ({
  value,
  onChange,
  disabledCost,
}: {
  value: RankingMetric;
  onChange: (value: RankingMetric) => void;
  disabledCost: boolean;
}) => (
  <div className={styles.rankingMetricSwitch} role="group" aria-label="排名分析维度">
    {RANKING_METRIC_OPTIONS.map((option) => (
      <button
        key={option.value}
        type="button"
        className={`${styles.rankingMetricButton} ${value === option.value ? styles.rankingMetricButtonActive : ''}`}
        onClick={() => onChange(option.value)}
        disabled={option.value === 'cost' && disabledCost}
      >
        {option.label}
      </button>
    ))}
  </div>
);

const buildAccountCardFileName = (row: MonitoringAccountRow, quotaEntries: AccountQuotaEntry[] = []) => {
  const quotaFileNames = Array.from(new Set(quotaEntries.map((entry) => entry.fileName).filter(Boolean)));
  if (quotaFileNames.length > 0) return joinShort(quotaFileNames, 1);

  const fileName = row.authLabels.find((label) => label && label !== '-' && label.endsWith('.json'));
  return fileName || row.authLabels.find((label) => label && label !== '-') || row.accountMasked || row.account;
};

const buildAccountCardProviderText = (row: MonitoringAccountRow) => {
  const providers = row.providers.filter((provider) => provider && provider !== '-');
  return providers.length > 0 ? joinShort(providers, 2) : '-';
};

const sortAccountOverviewCardMetrics = (metrics: AccountSummaryMetric[]) => {
  const labels: Record<string, string> = {
    'total-tokens': '总计',
    'input-tokens': '输入',
    'output-tokens': '输出',
    'cached-tokens': '缓存',
  };
  const order = ['total-tokens', 'input-tokens', 'output-tokens', 'cached-tokens'];
  return order
    .map((key) => {
      const metric = metrics.find((item) => item.key === key);
      return metric ? { ...metric, label: labels[key] } : undefined;
    })
    .filter(Boolean) as AccountSummaryMetric[];
};

const buildAccountSummaryMetrics = (
  row: MonitoringAccountRow,
  hasPrices: boolean,
  locale: string,
  t: TFunction
): AccountSummaryMetric[] => [
  {
    key: 'total-calls',
    label: t('monitoring.total_calls'),
    value: formatCompactNumber(row.totalCalls),
  },
  {
    key: 'success-calls',
    label: t('monitoring.success_calls'),
    value: <SuccessFailureValue success={row.successCalls} failure={row.failureCalls} />,
  },
  {
    key: 'success-rate',
    label: t('monitoring.call_success_rate'),
    value: formatPercent(row.successRate),
    valueClassName:
      row.successRate >= 0.95
        ? styles.goodText
        : row.successRate >= 0.85
          ? styles.warnText
          : styles.badText,
  },
  {
    key: 'total-tokens',
    label: t('monitoring.total_tokens'),
    value: formatCompactNumber(row.totalTokens),
  },
  {
    key: 'input-tokens',
    label: t('monitoring.input_tokens'),
    value: formatCompactNumber(row.inputTokens),
  },
  {
    key: 'output-tokens',
    label: t('monitoring.output_tokens'),
    value: formatCompactNumber(row.outputTokens),
  },
  {
    key: 'cached-tokens',
    label: t('monitoring.cached_tokens'),
    value: formatCompactNumber(row.cachedTokens),
  },
  {
    key: 'estimated-cost',
    label: t('monitoring.estimated_cost'),
    value: hasPrices ? formatUsd(row.totalCost) : '--',
  },
  {
    key: 'latest-request-time',
    label: t('monitoring.latest_request_time'),
    value: new Date(row.lastSeenAt).toLocaleString(locale),
  },
];

type AnyQuotaConfig = QuotaConfig<any, any>;

type AccountQuotaTarget = {
  key: string;
  authIndex: string;
  authLabel: string;
  fileName: string;
  file: AuthFileItem;
  config: AnyQuotaConfig;
};

type AccountQuotaEntry = {
  key: string;
  authLabel: string;
  fileName: string;
  providerLabel: string;
  quota?: QuotaStatusState;
  config: AnyQuotaConfig;
};

type AccountQuotaState = {
  status: 'idle' | 'loading' | 'success' | 'error';
  targetKey: string;
  error?: string;
  lastRefreshedAt?: number;
};

type AccountSummaryMetric = {
  key: string;
  label: string;
  value: ReactNode;
  valueClassName?: string;
};

const formatPercent = (value: number) => `${(value * 100).toFixed(1)}%`;

const getRangeStartMs = (range: MonitoringTimeRange, nowMs: number) => {
  const start = new Date(nowMs);
  start.setHours(0, 0, 0, 0);

  switch (range) {
    case 'today':
      return start.getTime();
    case '7d':
      start.setDate(start.getDate() - 6);
      return start.getTime();
    case '14d':
      start.setDate(start.getDate() - 13);
      return start.getTime();
    case '30d':
      start.setDate(start.getDate() - 29);
      return start.getTime();
    case 'all':
    default:
      return Number.NEGATIVE_INFINITY;
  }
};

const filterRowsByRange = (rows: MonitoringEventRow[], range: MonitoringTimeRange) => {
  const nowMs = Date.now();
  const startMs = getRangeStartMs(range, nowMs);
  return rows.filter((row) => row.timestampMs >= startMs && row.timestampMs <= nowMs);
};

const getProgressWidth = (value: number) => {
  if (value <= 0) return '0%';
  return `${Math.max(value * 100, 1.5)}%`;
};

const getChartRatio = (value: number, max: number) => {
  if (max <= 0 || value <= 0) return 0;
  return Math.max(Math.min(value / max, 1), 0.02);
};

const getChartAxisLabels = <T extends { key: string; label: string }>(points: T[]) => {
  if (points.length <= 10) {
    return points.map((point, index) => ({ key: point.key, label: point.label, index }));
  }

  const step = Math.ceil((points.length - 1) / 8);
  const labels = points
    .map((point, index) => ({ key: point.key, label: point.label, index }))
    .filter((_, index) => index % step === 0);
  const last = points[points.length - 1];
  if (!labels.some((item) => item.key === last.key)) {
    labels.push({ key: last.key, label: last.label, index: points.length - 1 });
  }
  return labels;
};

const formatLocalDayKey = (date: Date) => {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
};

const formatHourLabel = (date: Date) => `${String(date.getHours()).padStart(2, '0')}:00`;

const formatDayLabel = (date: Date) => `${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')}`;

const formatShortDateTime = (date: Date) =>
  `${date.getMonth() + 1}/${date.getDate()} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;

const buildUsageTrendRangeLabel = (range: MonitoringTimeRange) => {
  if (range === 'all') return '全部保留日志';

  const now = new Date();
  const start = new Date(getRangeStartMs(range, now.getTime()));
  return `${formatShortDateTime(start)} - ${formatShortDateTime(now)}`;
};

const getEmptyTrendPoint = (key: string, label: string): TrendPoint => ({
  key,
  label,
  requests: 0,
  failures: 0,
  tokens: 0,
  cost: 0,
});

const buildFilledTrendBuckets = (range: MonitoringTimeRange, nowMs: number) => {
  if (range === 'all') return [];

  const startMs = getRangeStartMs(range, nowMs);
  const buckets: TrendPoint[] = [];
  const cursor = new Date(startMs);

  if (range === 'today') {
    const now = new Date(nowMs);
    cursor.setMinutes(0, 0, 0);
    while (cursor.getTime() <= now.getTime()) {
      const dayKey = formatLocalDayKey(cursor);
      const label = formatHourLabel(cursor);
      buckets.push(getEmptyTrendPoint(`${dayKey} ${label}`, label));
      cursor.setHours(cursor.getHours() + 1);
    }
    return buckets;
  }

  cursor.setHours(0, 0, 0, 0);
  const end = new Date(nowMs);
  end.setHours(0, 0, 0, 0);
  while (cursor.getTime() <= end.getTime()) {
    const key = formatLocalDayKey(cursor);
    buckets.push(getEmptyTrendPoint(key, formatDayLabel(cursor)));
    cursor.setDate(cursor.getDate() + 1);
  }
  return buckets;
};

const buildTimeBucketMeta = (rows: MonitoringEventRow[]) => {
  const useHourly = new Set(rows.map((row) => row.dayKey)).size <= 1;
  return {
    useHourly,
    getKey: (row: MonitoringEventRow) => (useHourly ? `${row.dayKey} ${row.hourLabel}` : row.dayKey),
    getLabel: (row: MonitoringEventRow) => (useHourly ? row.hourLabel : row.dayKey.slice(5).replace('-', '/')),
  };
};

const roundCurrency = (value: number) => Math.round(value * 100) / 100;

const formatDeltaPercent = (current: number, previous: number) => {
  const roundedCurrent = roundCurrency(current);
  const roundedPrevious = roundCurrency(previous);
  if (roundedPrevious <= 0) return roundedCurrent > 0 ? '+100.0%' : '0.0%';
  const delta = (roundedCurrent - roundedPrevious) / roundedPrevious;
  return `${delta >= 0 ? '+' : ''}${(delta * 100).toFixed(1)}%`;
};

const joinShort = (values: string[], limit = 2) => {
  if (values.length <= limit) {
    return values.join(', ');
  }
  return `${values.slice(0, limit).join(', ')} +${values.length - limit}`;
};

const createPriceDraft = (price?: ModelPrice): PriceDraft => ({
  prompt: price ? String(price.prompt) : '',
  completion: price ? String(price.completion) : '',
  cache: price ? String(price.cache) : '',
});

const parsePriceValue = (value: string) => {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0;
};

const formatPriceUnit = (value: number) => `$${value.toFixed(4)}/1M`;

const buildTrendPoints = (rows: MonitoringEventRow[], range: MonitoringTimeRange = 'all'): TrendPoint[] => {
  const nowMs = Date.now();
  const prefilled = buildFilledTrendBuckets(range, nowMs);
  const grouped = new Map<string, TrendPoint>(prefilled.map((point) => [point.key, point]));
  const { getKey, getLabel } = buildTimeBucketMeta(rows);

  rows.forEach((row) => {
    const key = getKey(row);
    const label = getLabel(row);
    const existing = grouped.get(key) ?? {
      key,
      label,
      requests: 0,
      failures: 0,
      tokens: 0,
      cost: 0,
    };

    existing.requests += row.statsIncluded ? 1 : 0;
    existing.failures += row.failed ? 1 : 0;
    existing.tokens += row.totalTokens;
    existing.cost += row.totalCost;
    grouped.set(key, existing);
  });

  return Array.from(grouped.values()).sort((left, right) => left.key.localeCompare(right.key)).slice(-24);
};

const buildTokenDistributionPoints = (rows: MonitoringEventRow[]): TokenDistributionPoint[] => {
  const grouped = new Map<string, TokenDistributionPoint>();
  const { getKey, getLabel } = buildTimeBucketMeta(rows);

  rows.forEach((row) => {
    const key = getKey(row);
    const label = getLabel(row);
    const existing = grouped.get(key) ?? {
      key,
      label,
      requests: 0,
      totalTokens: 0,
      inputTokens: 0,
      outputTokens: 0,
      reasoningTokens: 0,
      cachedTokens: 0,
    };

    existing.requests += row.statsIncluded ? 1 : 0;
    existing.totalTokens += row.totalTokens;
    existing.inputTokens += row.inputTokens;
    existing.outputTokens += row.outputTokens;
    existing.reasoningTokens += row.reasoningTokens;
    existing.cachedTokens += row.cachedTokens;
    grouped.set(key, existing);
  });

  return Array.from(grouped.values()).sort((left, right) => left.key.localeCompare(right.key)).slice(-24);
};

const buildRealtimeMetaText = (row: MonitoringEventRow) => {
  const text = `${row.endpointMethod} ${row.endpointPath}`.trim();
  return maskSensitiveText(text || '-');
};

const QUOTA_CONFIGS: AnyQuotaConfig[] = [
  ANTIGRAVITY_CONFIG,
  CLAUDE_CONFIG,
  CODEX_CONFIG,
  GEMINI_CLI_CONFIG,
  KIMI_CONFIG,
] as AnyQuotaConfig[];

const QUOTA_RENDER_HELPERS: QuotaRenderHelpers = {
  styles: quotaStyles,
  QuotaProgressBar,
};

const getQuotaProviderLabel = (config: AnyQuotaConfig, t: TFunction) => {
  const titleKey = `${config.i18nPrefix}.title`;
  const translated = t(titleKey);
  if (translated !== titleKey) return translated;
  return config.type;
};

const resolveQuotaErrorMessage = (t: TFunction, quota?: QuotaStatusState): string => {
  if (!quota) return t('common.unknown_error');
  if (quota.errorStatus === 404) return t('common.quota_update_required');
  if (quota.errorStatus === 403) return t('common.quota_check_credential');
  return quota.error || t('common.unknown_error');
};

const hasUsableQuotaContent = (quota?: QuotaStatusState) => {
  if (!quota || quota.status !== 'success') return false;
  const record = quota as unknown as Record<string, unknown>;
  return ['groups', 'windows', 'buckets', 'rows'].some((key) => {
    const value = record[key];
    return Array.isArray(value) && value.length > 0;
  }) || Boolean(record.planType || record.tierLabel || record.creditBalance !== undefined);
};

const getQuotaForTarget = (store: QuotaStore, target: AccountQuotaTarget): QuotaStatusState | undefined => {
  return target.config.storeSelector(store)[target.fileName] as QuotaStatusState | undefined;
};

const requestAccountQuota = async (
  target: AccountQuotaTarget,
  t: TFunction
): Promise<QuotaStatusState> => {
  try {
    const data = await target.config.fetchQuota(target.file, t);
    return target.config.buildSuccessState(data) as QuotaStatusState;
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : t('common.unknown_error');
    return target.config.buildErrorState(message, getStatusFromError(err)) as QuotaStatusState;
  }
};

const buildAccountQuotaTargetsByAccount = (
  rows: MonitoringEventRow[],
  authFilesByAuthIndex: Map<string, AuthFileItem>
) => {
  const grouped = new Map<string, Map<string, AccountQuotaTarget>>();

  rows.forEach((row) => {
    const authIndex = normalizeAuthIndex(row.authIndex);
    if (!authIndex || !row.account) return;

    const file = authFilesByAuthIndex.get(authIndex);
    if (!file) return;

    const quotaConfig = QUOTA_CONFIGS.find((item) => item.filterFn(file));
    if (!quotaConfig) return;

    const dedupeKey = `${quotaConfig.type}::${authIndex}::${file.name}`;
    const bucket = grouped.get(row.account) ?? new Map<string, AccountQuotaTarget>();
    if (!bucket.has(dedupeKey)) {
      bucket.set(dedupeKey, {
        key: dedupeKey,
        authIndex,
        authLabel: row.authLabel || file.name || authIndex,
        fileName: file.name,
        file,
        config: quotaConfig,
      });
    }
    grouped.set(row.account, bucket);
  });

  return new Map(
    Array.from(grouped.entries()).map(([account, bucket]) => [
      account,
      Array.from(bucket.values()).sort((left, right) => left.authLabel.localeCompare(right.authLabel)),
    ])
  );
};

const buildAccountQuotaEntriesByAccount = (
  targetsByAccount: Map<string, AccountQuotaTarget[]>,
  quotaStore: QuotaStore,
  t: TFunction
) => new Map(
  Array.from(targetsByAccount.entries()).map(([account, targets]) => [
    account,
    targets.map((target) => ({
      key: target.key,
      authLabel: target.authLabel,
      fileName: target.fileName,
      providerLabel: getQuotaProviderLabel(target.config, t),
      quota: getQuotaForTarget(quotaStore, target),
      config: target.config,
    } satisfies AccountQuotaEntry)),
  ])
);

const buildRealtimeLogRows = (rows: MonitoringEventRow[]): RealtimeLogRow[] => {
  const sortedAsc = [...rows].sort(
    (left, right) => left.timestampMs - right.timestampMs || left.id.localeCompare(right.id)
  );
  const metricsByStream = new Map<string, { total: number; success: number; pattern: boolean[] }>();

  const enriched = sortedAsc.map((row) => {
    const streamKey = [row.account, row.provider, row.model, row.channel].join('::');
    const previous = metricsByStream.get(streamKey) ?? { total: 0, success: 0, pattern: [] };
    const nextPattern = [...previous.pattern, !row.failed].slice(-10);
    const next = {
      total: previous.total + (row.statsIncluded ? 1 : 0),
      success: previous.success + (row.statsIncluded && !row.failed ? 1 : 0),
      pattern: nextPattern,
    };
    metricsByStream.set(streamKey, next);

    return {
      ...row,
      streamKey,
      requestCount: next.total,
      successRate: next.total > 0 ? next.success / next.total : 1,
      recentPattern: nextPattern,
    } satisfies RealtimeLogRow;
  });

  return enriched.sort(
    (left, right) =>
      right.timestampMs - left.timestampMs ||
      right.requestCount - left.requestCount ||
      right.id.localeCompare(left.id)
  );
};

function UsageTrendHeader({
  range,
  totalCalls,
  onRangeChange,
  onHide,
  t,
}: {
  range: MonitoringTimeRange;
  totalCalls: number;
  onRangeChange: (range: MonitoringTimeRange) => void;
  onHide: () => void;
  t: TFunction;
}) {
  return (
    <div className={styles.usageTrendHeader}>
      <div className={styles.usageTrendCopy}>
        <h2>使用趋势</h2>
        <p>{`基于选定时间范围内 ${formatCompactNumber(totalCalls)} 条请求日志自动聚合。`}</p>
      </div>
      <div className={styles.usageTrendActions}>
        <div className={`${styles.rankingMetricSwitch} ${styles.usageTrendRangeControl}`}>
          {TIME_RANGE_OPTIONS.map((option) => (
            <button
              key={option.value}
              type="button"
              className={`${styles.rankingMetricButton} ${styles.usageTrendRangeButton} ${range === option.value ? styles.rankingMetricButtonActive : ''}`}
              onClick={() => onRangeChange(option.value)}
            >
              {t(option.labelKey)}
            </button>
          ))}
        </div>
        <button type="button" className={`${styles.rankingMetricButton} ${styles.usageTrendHideButton}`} onClick={onHide}>
          隐藏分析
        </button>
      </div>
    </div>
  );
}

function TopUsageStats({ cards }: { cards: UsageMetricCard[] }) {
  return (
    <section className={styles.usageStatsGrid} aria-label="Usage statistics">
      {cards.map((card) => (
        <Card key={card.key} className={`${styles.usageStatsCard} ${card.key === 'tokens' ? styles.usageStatsCardTokens : ''}`}>
          <div className={styles.usageStatsCardHeader}>
            <span className={`${styles.usageStatsIcon} ${styles[`usageStatsIcon${card.accent}`]}`} aria-hidden="true" />
            <strong>{card.title}</strong>
          </div>
          <div className={styles.usageStatsBody}>
            <span>{card.label}</span>
            <strong>{card.value}</strong>
          </div>
          <div className={styles.usageStatsFooter}>
            {card.footer.map((item) => (
              <div key={item.label}>
                <span>{item.label}</span>
                <strong>{item.value}</strong>
              </div>
            ))}
          </div>
        </Card>
      ))}
    </section>
  );
}

function UsageTrendPanel({
  points,
  hasPrices,
  emptyText,
}: {
  points: TrendPoint[];
  hasPrices: boolean;
  emptyText: string;
}) {
  const chartPoints = points.slice(-24);
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
  const plot = { left: 46, top: 22, right: 666, bottom: 278 };
  const tokenMax = Math.max(...chartPoints.map((point) => point.tokens), 1);
  const requestMax = Math.max(...chartPoints.map((point) => point.requests), 1);
  const costMax = Math.max(...chartPoints.map((point) => point.cost), 1);
  const series = [
    {
      key: 'tokens',
      label: 'Token',
      color: '#7c3aed',
      getValue: (point: TrendPoint) => point.tokens,
      max: tokenMax,
      format: (value: number) => formatCompactNumber(value),
    },
    {
      key: 'requests',
      label: '请求',
      color: '#2563eb',
      getValue: (point: TrendPoint) => point.requests,
      max: requestMax,
      format: (value: number) => formatCompactNumber(value),
    },
    {
      key: 'cost',
      label: '费用',
      color: '#059669',
      getValue: (point: TrendPoint) => point.cost,
      max: costMax,
      format: (value: number) => (hasPrices ? formatUsd(value) : '--'),
    },
  ];
  const visibleSeries = hasPrices ? series : series.filter((item) => item.key !== 'cost');
  const totals = chartPoints.reduce(
    (sum, point) => ({
      requests: sum.requests + point.requests,
      failures: sum.failures + point.failures,
      tokens: sum.tokens + point.tokens,
      cost: sum.cost + point.cost,
    }),
    { requests: 0, failures: 0, tokens: 0, cost: 0 }
  );
  const peakTokenPoint = chartPoints.reduce<TrendPoint | null>(
    (peak, point) => (!peak || point.tokens > peak.tokens ? point : peak),
    null
  );
  const summaryItems = [
    { key: 'requests', label: '总请求', value: formatCompactNumber(totals.requests), color: '#2563eb' },
    { key: 'tokens', label: '总 Token', value: formatCompactNumber(totals.tokens), color: '#7c3aed' },
    ...(hasPrices ? [{ key: 'cost', label: '总费用', value: formatUsd(totals.cost), color: '#059669' }] : []),
    { key: 'peak', label: '峰值时段', value: peakTokenPoint?.label ?? '--', color: '#f97316' },
  ];
  const trendMinutes = Math.max(chartPoints.length * 60, 1);
  const headerStats = [
    { key: 'rpm', label: 'RPM', value: (totals.requests / trendMinutes).toFixed(2) },
    { key: 'tpm', label: 'TPM', value: formatCompactNumber(totals.tokens / trendMinutes) },
    { key: 'errorRate', label: '错误率', value: formatPercent(totals.requests > 0 ? totals.failures / totals.requests : 0) },
  ];
  const getX = (index: number) => chartPoints.length <= 1
    ? (plot.left + plot.right) / 2
    : plot.left + (index / (chartPoints.length - 1)) * (plot.right - plot.left);
  const getY = (value: number, max: number) => plot.bottom - getChartRatio(value, max) * (plot.bottom - plot.top);
  const buildPath = (item: typeof series[number]) => {
    const coords = chartPoints.map((point, index) => ({
      x: getX(index),
      y: getY(item.getValue(point), item.max),
    }));
    if (coords.length === 0) return '';
    if (coords.length === 1) return `M ${coords[0].x} ${coords[0].y}`;
    return coords.slice(1).reduce((path, point, index) => {
      const previous = coords[index];
      const midX = (previous.x + point.x) / 2;
      return `${path} C ${midX} ${previous.y}, ${midX} ${point.y}, ${point.x} ${point.y}`;
    }, `M ${coords[0].x} ${coords[0].y}`);
  };
  const labels = getChartAxisLabels(chartPoints);
  const hoveredPoint = hoveredIndex === null ? null : chartPoints[hoveredIndex];
  const hoveredX = hoveredIndex === null ? 0 : getX(hoveredIndex);
  const tooltipX = Math.min(Math.max(hoveredX - 84, plot.left + 8), plot.right - 168);

  return (
    <Card className={`${styles.usageTrendChartCard} ${styles.usageTrendLineCard}`}>
      <div className={styles.trendCardHeader}>
        <div>
          <h3>用量趋势</h3>
          <p>基于选定范围的相对峰值变化，悬停查看实际数值。</p>
        </div>
        <div className={styles.trendHeaderStats}>
          {headerStats.map((item) => (
            <div key={item.key}>
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </div>
          ))}
        </div>
      </div>
      {chartPoints.length > 0 ? (
        <div className={styles.professionalChartShell}>
          <div className={styles.trendSummaryStrip}>
            {summaryItems.map((item) => (
              <div key={item.key} className={styles.trendSummaryItem} style={{ '--series-color': item.color } as CSSProperties}>
                <span>{item.label}</span>
                <strong>{item.value}</strong>
              </div>
            ))}
          </div>
          <svg className={styles.usageTrendSvg} viewBox="0 0 700 300" role="img" aria-label="用量趋势图">
            <defs>
              <linearGradient id="usageTrendFill" x1="0" x2="0" y1="0" y2="1">
                <stop offset="0%" stopColor="#7c3aed" stopOpacity="0.14" />
                <stop offset="100%" stopColor="#7c3aed" stopOpacity="0" />
              </linearGradient>
            </defs>
            {[0, 0.25, 0.5, 0.75, 1].map((tick) => {
              const y = plot.bottom - tick * (plot.bottom - plot.top);
              return (
                <g key={tick}>
                  <line className={styles.chartGridLine} x1={plot.left} x2={plot.right} y1={y} y2={y} />
                  <text className={styles.chartAxisLabel} x="36" y={y + 4}>{`${Math.round(tick * 100)}%`}</text>
                </g>
              );
            })}
            <line className={styles.chartAxisBase} x1={plot.left} x2={plot.right} y1={plot.bottom} y2={plot.bottom} />
            {visibleSeries.map((item, index) => {
              const path = buildPath(item);
              const area = index === 0 && path
                ? `${path} L ${getX(chartPoints.length - 1)} ${plot.bottom} L ${getX(0)} ${plot.bottom} Z`
                : '';
              return (
                <g key={item.key}>
                  {area ? <path className={styles.trendAreaFill} d={area} /> : null}
                  <path className={styles.trendSeriesLine} d={path} stroke={item.color} />
                </g>
              );
            })}
            {labels.map((item) => (
              <text key={item.key} className={styles.chartXAxisLabel} x={getX(item.index)} y="286">{item.label}</text>
            ))}
            {chartPoints.map((point, index) => {
              const x = getX(index);
              const isHovered = hoveredIndex === index;
              return (
                <g
                  key={point.key}
                  className={styles.trendHoverTarget}
                  onMouseEnter={() => setHoveredIndex(index)}
                  onMouseLeave={() => setHoveredIndex(null)}
                  onFocus={() => setHoveredIndex(index)}
                  onBlur={() => setHoveredIndex(null)}
                  tabIndex={0}
                >
                  <rect x={Math.max(plot.left, x - 16)} y={plot.top - 10} width="32" height={plot.bottom - plot.top + 24} fill="transparent" />
                  {isHovered ? <line className={styles.trendHoverGuide} x1={x} x2={x} y1={plot.top} y2={plot.bottom} /> : null}
                  {visibleSeries.map((item) => (
                    <circle
                      key={item.key}
                      className={styles.trendSeriesDot}
                      cx={x}
                      cy={getY(item.getValue(point), item.max)}
                      r={isHovered ? 4.5 : 2.6}
                      stroke={item.color}
                    />
                  ))}
                </g>
              );
            })}
            {hoveredPoint ? (
              <g className={styles.trendTooltipLayer}>
                <rect x={tooltipX} y="86" width="168" height={hasPrices ? 118 : 92} rx="12" />
                <text className={styles.trendTooltipTitle} x={tooltipX + 16} y="112">{hoveredPoint.label}</text>
                {visibleSeries.map((item, index) => (
                  <text key={item.key} className={styles.trendTooltipMetric} x={tooltipX + 16} y={140 + index * 25} fill={item.color}>
                    {`${item.label}：${item.format(item.getValue(hoveredPoint))}`}
                  </text>
                ))}
              </g>
            ) : null}
          </svg>
          <div className={styles.trendLegend}>
            {visibleSeries.map((item) => (
              <span key={item.key} style={{ '--series-color': item.color } as CSSProperties}>{item.label}</span>
            ))}
          </div>
        </div>
      ) : (
        <div className={styles.emptyBlockSmall}>{emptyText}</div>
      )}
    </Card>
  );
}

function TokenDistributionPanel({
  points,
  emptyText,
}: {
  points: TokenDistributionPoint[];
  emptyText: string;
}) {
  const totals = points.reduce(
    (sum, point) => ({
      requests: sum.requests + point.requests,
      totalTokens: sum.totalTokens + point.totalTokens,
      inputTokens: sum.inputTokens + point.inputTokens,
      outputTokens: sum.outputTokens + point.outputTokens,
      reasoningTokens: sum.reasoningTokens + point.reasoningTokens,
      cachedTokens: sum.cachedTokens + point.cachedTokens,
    }),
    { requests: 0, totalTokens: 0, inputTokens: 0, outputTokens: 0, reasoningTokens: 0, cachedTokens: 0 }
  );
  const tokenMinutes = Math.max(points.length * 60, 1);
  const rpm = totals.requests / tokenMinutes;
  const tpm = totals.totalTokens / tokenMinutes;
  const rows = [
    { key: 'rpm', label: 'RPM', value: rpm, displayValue: rpm.toFixed(2), base: 0, accent: 'Purple', showShare: false },
    { key: 'tpm', label: 'TPM', value: tpm, displayValue: formatCompactNumber(tpm), base: 0, accent: 'Blue', showShare: false },
    { key: 'requests', label: '请求数', value: totals.requests, displayValue: formatCompactNumber(totals.requests), base: 0, accent: 'Cyan', showShare: false },
    { key: 'total', label: '总 TOKEN', value: totals.totalTokens, displayValue: formatCompactNumber(totals.totalTokens), base: 0, accent: 'Green', showShare: false },
    { key: 'input', label: '输入', value: totals.inputTokens, displayValue: formatCompactNumber(totals.inputTokens), base: totals.totalTokens, accent: 'Amber', showShare: true },
    { key: 'output', label: '输出', value: totals.outputTokens, displayValue: formatCompactNumber(totals.outputTokens), base: totals.totalTokens, accent: 'Rose', showShare: true },
    { key: 'reasoning', label: '推理', value: totals.reasoningTokens, displayValue: formatCompactNumber(totals.reasoningTokens), base: totals.totalTokens, accent: 'Indigo', showShare: true },
    { key: 'cached', label: '缓存', value: totals.cachedTokens, displayValue: formatCompactNumber(totals.cachedTokens), base: totals.totalTokens, accent: 'Slate', showShare: true },
  ];
  const hasData = rows.some((row) => row.value > 0);

  return (
    <Card className={`${styles.usageTrendChartCard} ${styles.tokenDistributionCard}`}>
      <div className={styles.trendCardHeader}>
        <div>
          <h3>Token 统计</h3>
          <p>按请求规模和消耗类型查看 Token 分布。</p>
        </div>
      </div>
      {hasData ? (
        <div className={styles.tokenStatCardList}>
          {rows.map((row) => {
            const share = row.base > 0 ? row.value / row.base : 0;
            return (
              <div key={row.key} className={`${styles.tokenStatCard} ${styles[`tokenStatCard${row.accent}`]}`}>
                <div className={styles.tokenStatCardHeader}>
                  <span>{row.label}</span>
                  {row.showShare ? <strong>{formatPercent(share)}</strong> : null}
                </div>
                <div className={styles.tokenStatCardValue}>{row.displayValue}</div>
                {row.showShare ? (
                  <div className={styles.tokenStatProgressTrack} aria-hidden="true">
                    <span style={{ '--token-stat-width': `${Math.min(Math.max(share * 100, row.value > 0 ? 1 : 0), 100)}%` } as CSSProperties} />
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      ) : (
        <div className={styles.emptyBlockSmall}>{emptyText}</div>
      )}
    </Card>
  );
}

function ModelStatsPanel({
  title,
  subtitle,
  rows,
  metric,
  metricTotal,
  onMetricChange,
  emptyText,
  hasPrices,
}: {
  title: string;
  subtitle: string;
  rows: MonitoringAccountRow[];
  metric: RankingMetric;
  metricTotal: number;
  onMetricChange: (metric: RankingMetric) => void;
  emptyText: string;
  hasPrices: boolean;
}) {
  const shareBase = metricTotal > 0 ? metricTotal : rows.reduce((sum, row) => sum + getRankingMetricValue(row, metric), 0);
  const shareModeLabel = `按${getRankingMetricLabel(metric)}`;
  const totalShareValue = formatRankingMetricValue(shareBase, metric, hasPrices);
  const legendRows = rows.slice(0, 5);
  const donutTooltipRows = rows
    .map((row) => {
      const value = getRankingMetricValue(row, metric);
      return {
        row,
        value,
        share: shareBase > 0 ? value / shareBase : 0,
      };
    })
    .filter((item) => item.value > 0);
  const donutStops = legendRows.reduce(
    (state, row, index) => {
      const value = getRankingMetricValue(row, metric);
      const share = shareBase > 0 ? (value / shareBase) * 100 : 0;
      const end = state.offset + share;
      state.parts.push(`${getDonutColor(index)} ${state.offset}% ${end}%`);
      state.offset = end;
      return state;
    },
    { offset: 0, parts: [] as string[] }
  );
  const donutBackground = donutStops.parts.length > 0
    ? `conic-gradient(${donutStops.parts.join(', ')}, color-mix(in srgb, var(--monitor-line) 58%, transparent) ${donutStops.offset}% 100%)`
    : 'color-mix(in srgb, var(--monitor-line) 58%, transparent)';

  return (
    <Card className={styles.modelStatsCard}>
      <div className={styles.rankingHeader}>
        <div>
          <h3>{title}</h3>
          <p>{subtitle}</p>
        </div>
        <RankingMetricSwitch value={metric} onChange={onMetricChange} disabledCost={!hasPrices} />
      </div>
      {rows.length > 0 ? (
        <div className={styles.modelStatsLayout}>
          <div className={styles.modelStatsList}>
            {rows.map((row) => {
              const rowValue = getRankingMetricValue(row, metric);
              const value = shareBase > 0 ? rowValue / shareBase : 0;
              return (
                <div key={row.id} className={styles.modelStatsRow}>
                  <div className={styles.modelStatsMain}>
                    <div className={styles.modelStatsTitleLine}>
                      <strong>{row.account}</strong>
                      <span>{formatPercent(value)}</span>
                    </div>
                    <div className={styles.modelStatsMetaLine}>
                      <span>{`${formatCompactNumber(row.totalCalls)} 请求`}</span>
                      <span>{`${formatCompactNumber(row.totalTokens)} Token`}</span>
                      <span>{`${formatCompactNumber(row.failureCalls)} 错误`}</span>
                      <span>{hasPrices ? formatUsd(row.totalCost) : '--'}</span>
                    </div>
                    <span
                      className={styles.modelStatsBar}
                      style={{ '--ranking-width': getProgressWidth(value) } as CSSProperties}
                      aria-hidden="true"
                    />
                  </div>
                </div>
              );
            })}
          </div>
          <aside className={styles.modelSharePanel}>
            <div className={styles.modelShareHeader}>
              <strong>模型占比</strong>
              <span>{shareModeLabel}</span>
            </div>
            <div className={styles.donutChart} style={{ '--donut-bg': donutBackground } as CSSProperties}>
              <div className={styles.donutCenter}>
                <span>{getRankingSummaryLabel(metric)}</span>
                <strong>{totalShareValue}</strong>
              </div>
              <div className={styles.donutTooltip}>
                <strong>{shareModeLabel}占比</strong>
                <div>
                  {donutTooltipRows.map((item, index) => (
                    <span key={item.row.id}>
                      <i style={{ background: getDonutColor(index) }} aria-hidden="true" />
                      <em>{item.row.account}</em>
                      <b>{formatPercent(item.share)}</b>
                    </span>
                  ))}
                </div>
              </div>
            </div>
            <div className={styles.modelLegend}>
              {legendRows.map((row, index) => {
                const rowValue = getRankingMetricValue(row, metric);
                const value = shareBase > 0 ? rowValue / shareBase : 0;
                return (
                  <div key={row.id} className={styles.modelLegendItem} style={{ '--legend-color': getDonutColor(index) } as CSSProperties}>
                    <span className={styles.modelLegendDot} />
                    <span>{row.account}</span>
                    <strong>{formatPercent(value)}</strong>
                  </div>
                );
              })}
            </div>
          </aside>
        </div>
      ) : (
        <div className={styles.emptyBlockSmall}>{emptyText}</div>
      )}
    </Card>
  );
}

function ApiKeyRankingPanel({
  title,
  subtitle,
  rows,
  metric,
  metricTotal,
  onMetricChange,
  emptyText,
  hasPrices,
}: {
  title: string;
  subtitle: string;
  rows: MonitoringAccountRow[];
  metric: RankingMetric;
  metricTotal: number;
  onMetricChange: (metric: RankingMetric) => void;
  emptyText: string;
  hasPrices: boolean;
}) {
  const shareBase = metricTotal > 0 ? metricTotal : rows.reduce((sum, row) => sum + getRankingMetricValue(row, metric), 0);
  const summaryLabel = getRankingSummaryLabel(metric);

  return (
    <Card className={styles.apiKeyRankingCard}>
      <div className={styles.rankingHeader}>
        <div>
          <h3>{title}</h3>
          <p>{subtitle}</p>
        </div>
        <RankingMetricSwitch value={metric} onChange={onMetricChange} disabledCost={!hasPrices} />
      </div>
      <div className={styles.apiKeyRankingList}>
        {rows.length > 0 ? (
          <div className={styles.apiKeyRankingSummary}>
            <span>{summaryLabel}</span>
            <strong>{formatRankingMetricValue(shareBase, metric, hasPrices)}</strong>
            <small>{`${rows.length} 个 API 密钥`}</small>
          </div>
        ) : null}
        {rows.length > 0 ? (
          <div className={styles.apiKeyRankingScroll}>
            {rows.map((row, index) => {
              const rowValue = getRankingMetricValue(row, metric);
              const share = shareBase > 0 ? rowValue / shareBase : 0;
              return (
                <div key={row.id} className={styles.apiKeyRankingRow}>
                  <div className={styles.apiKeyRankingTopLine}>
                    <div className={styles.apiKeyRankingName}>
                      <span className={styles.rankingIndex}>{index + 1}</span>
                      <strong>{row.account}</strong>
                    </div>
                    <span>{formatPercent(share)}</span>
                  </div>
                  <div className={styles.apiKeyRankingMetaLine}>
                    <span>{`${formatCompactNumber(row.totalCalls)} 请求`}</span>
                    <span>{`${formatCompactNumber(row.totalTokens)} Token`}</span>
                    <span>{`${formatCompactNumber(row.failureCalls)} 错误`}</span>
                    <span>{hasPrices ? formatUsd(row.totalCost) : '--'}</span>
                  </div>
                  <span
                    className={styles.apiKeyRankingBar}
                    style={{ '--ranking-width': getProgressWidth(share) } as CSSProperties}
                    aria-hidden="true"
                  />
                </div>
              );
            })}
          </div>
        ) : (
          <div className={styles.emptyBlockSmall}>{emptyText}</div>
        )}
      </div>
    </Card>
  );
}

function MonitoringHealthStatusBar({
  statusData,
  locale,
  t,
  showRate = true,
}: {
  statusData: AccountStatusData;
  locale: string;
  t: TFunction;
  showRate?: boolean;
}) {
  const [activeTooltip, setActiveTooltip] = useState<number | null>(null);
  const [focusIndex, setFocusIndex] = useState(() => (statusData.blockDetails.length > 0 ? 0 : -1));
  const blocksRef = useRef<HTMLDivElement | null>(null);
  const blockButtonRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const tooltipIdPrefix = useId();
  const blockCount = statusData.blockDetails.length;
  const resolvedFocusIndex = blockCount === 0 ? -1 : focusIndex >= 0 && focusIndex < blockCount ? focusIndex : 0;
  const resolvedActiveTooltip = activeTooltip !== null && activeTooltip >= 0 && activeTooltip < blockCount ? activeTooltip : null;
  const hasData = statusData.totalSuccess + statusData.totalFailure > 0;
  const rateClassName = !hasData
    ? ''
    : statusData.successRate >= 90
      ? styles.monitoringStatusRateHigh
      : statusData.successRate >= 50
        ? styles.monitoringStatusRateMedium
        : styles.monitoringStatusRateLow;

  useEffect(() => {
    if (resolvedActiveTooltip === null) return;
    const handlePointerDown = (event: PointerEvent) => {
      if (blocksRef.current && !blocksRef.current.contains(event.target as Node)) {
        setActiveTooltip(null);
      }
    };
    document.addEventListener('pointerdown', handlePointerDown);
    return () => document.removeEventListener('pointerdown', handlePointerDown);
  }, [resolvedActiveTooltip]);

  const handlePointerEnter = useCallback((event: React.PointerEvent, index: number) => {
    if (event.pointerType === 'mouse') {
      setActiveTooltip(index);
    }
  }, []);

  const handlePointerLeave = useCallback((event: React.PointerEvent) => {
    if (event.pointerType === 'mouse' && (!blocksRef.current || !blocksRef.current.contains(document.activeElement))) {
      setActiveTooltip(null);
    }
  }, []);

  const handlePointerDown = useCallback((event: React.PointerEvent, index: number) => {
    if (event.pointerType === 'touch') {
      event.preventDefault();
      setFocusIndex(index);
      setActiveTooltip((previous) => (previous === index ? null : index));
    }
  }, []);

  const focusBlock = useCallback((index: number) => {
    blockButtonRefs.current[index]?.focus();
    setFocusIndex(index);
    setActiveTooltip(index);
  }, []);

  const handleFocus = useCallback((index: number) => {
    setFocusIndex(index);
    setActiveTooltip(index);
  }, []);

  const handleBlur = useCallback((event: React.FocusEvent<HTMLButtonElement>) => {
    if (blocksRef.current?.contains(event.relatedTarget as Node | null)) {
      return;
    }
    setActiveTooltip(null);
  }, []);

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLButtonElement>, index: number) => {
      if (event.key === 'Escape') {
        setActiveTooltip(null);
        return;
      }

      const nextIndex = getNextAccountStatusBlockIndex(index, event.key, blockCount);
      if (nextIndex === null) return;

      event.preventDefault();
      focusBlock(nextIndex);
    },
    [blockCount, focusBlock]
  );

  const getTooltipPositionClassName = (index: number, total: number) => {
    if (index <= 2) return styles.monitoringStatusTooltipLeft;
    if (index >= total - 3) return styles.monitoringStatusTooltipRight;
    return '';
  };

  const renderTooltip = (detail: AccountStatusBlockDetail, index: number, tooltipId: string) => {
    const total = detail.success + detail.failure;
    const timeRange = formatStatusWindowLabel(detail.startTime, detail.endTime, locale);

    return (
      <div
        id={tooltipId}
        role="tooltip"
        className={[
          styles.monitoringStatusTooltip,
          getTooltipPositionClassName(index, statusData.blockDetails.length),
        ]
          .filter(Boolean)
          .join(' ')}
      >
        <span className={styles.monitoringTooltipTime}>{timeRange}</span>
        {total > 0 ? (
          <span className={styles.monitoringTooltipStats}>
            <span className={styles.monitoringTooltipSuccess}>{t('status_bar.success_short')} {detail.success}</span>
            <span className={styles.monitoringTooltipFailure}>{t('status_bar.failure_short')} {detail.failure}</span>
            <span className={styles.monitoringTooltipRate}>({(detail.rate * 100).toFixed(1)}%)</span>
          </span>
        ) : (
          <span className={styles.monitoringTooltipStats}>{t('status_bar.no_requests')}</span>
        )}
      </div>
    );
  };

  return (
    <div className={styles.monitoringStatusBar}>
      <div
        className={styles.monitoringStatusBlocks}
        ref={blocksRef}
        role="group"
        aria-label="账号健康状态"
      >
        {statusData.blockDetails.map((detail, index) => {
          const isIdle = detail.rate === -1;
          const isActive = resolvedActiveTooltip === index;
          const timeRangeLabel = formatStatusWindowLabel(detail.startTime, detail.endTime, locale);
          const tooltipId = `${tooltipIdPrefix}-monitoring-status-tooltip-${index}`;
          const ariaLabel = buildAccountStatusBlockAriaLabel({
            detail,
            timeRangeLabel,
            successRateValue: formatStatusRate(Math.max(0, detail.rate * 100)),
            copy: {
              successLabel: t('stats.success'),
              failureLabel: t('stats.failure'),
              noRequestsLabel: t('status_bar.no_requests'),
              successRateLabel: t('monitoring.success_rate'),
            },
          });

          return (
            <div
              key={`${detail.startTime}-${detail.endTime}`}
              className={[styles.monitoringStatusBlockWrapper, isActive ? styles.monitoringStatusBlockActive : '']
                .filter(Boolean)
                .join(' ')}
            >
              <button
                ref={(node) => {
                  blockButtonRefs.current[index] = node;
                }}
                type="button"
                className={styles.monitoringStatusBlockButton}
                tabIndex={resolvedFocusIndex === index ? 0 : -1}
                aria-label={ariaLabel}
                aria-describedby={isActive ? tooltipId : undefined}
                onFocus={() => handleFocus(index)}
                onBlur={handleBlur}
                onKeyDown={(event) => handleKeyDown(event, index)}
                onPointerEnter={(event) => handlePointerEnter(event, index)}
                onPointerLeave={handlePointerLeave}
                onPointerDown={(event) => handlePointerDown(event, index)}
              >
                <div
                  aria-hidden="true"
                  className={[styles.monitoringStatusBlock, isIdle ? styles.monitoringStatusBlockIdle : '']
                    .filter(Boolean)
                    .join(' ')}
                  style={isIdle ? undefined : { backgroundColor: getAccountStatusColor(detail.rate) }}
                />
              </button>
              {isActive ? renderTooltip(detail, index, tooltipId) : null}
            </div>
          );
        })}
      </div>
      {showRate ? (
        <span
          className={[styles.monitoringStatusRate, rateClassName, !hasData ? styles.monitoringStatusRatePlaceholder : '']
            .filter(Boolean)
            .join(' ')}
        >
          {hasData ? formatStatusRate(statusData.successRate) : '--'}
        </span>
      ) : null}
    </div>
  );
}

const getSuccessRateClassName = (rate: number) =>
  rate >= 0.95 ? styles.goodText : rate >= 0.85 ? styles.warnText : styles.badText;

const getAccountStatusDotClassName = (tone: AccountHealthTone) => {
  if (tone === 'good') return styles.accountStatusDotEnabled;
  if (tone === 'warn') return styles.accountStatusDotMixed;
  return styles.accountStatusDotDisabled;
};

function AccountHealthStatusPanel({
  row,
  hasPrices,
  locale,
  t,
  statusData,
  scopeText,
}: {
  row: MonitoringAccountRow;
  hasPrices: boolean;
  locale: string;
  t: TFunction;
  statusData: AccountStatusData;
  scopeText: string;
}) {
  const healthMetrics = [
    { key: 'total-calls', label: '总调用', value: formatCompactNumber(row.totalCalls) },
    {
      key: 'success-failure',
      label: '成功/失败',
      value: <SuccessFailureValue success={row.successCalls} failure={row.failureCalls} />,
    },
    { key: 'estimated-cost', label: '预估花费', value: hasPrices ? formatUsd(row.totalCost) : '--', className: styles.primaryText },
    { key: 'success-rate', label: '成功率', value: formatPercent(row.successRate), className: getSuccessRateClassName(row.successRate) },
  ];

  return (
    <section className={styles.accountOverviewStatusSection}>
      <div className={styles.accountSectionHeader}>
        <strong>健康状态</strong>
        <span className={styles.accountSectionInfo} title="基于近期请求成功率与错误情况计算">
          i
        </span>
      </div>
      <div className={styles.healthMetricGrid}>
        {healthMetrics.map((metric) => (
          <div key={metric.key} className={styles.healthMetricItem}>
            <span>{metric.label}</span>
            <strong className={metric.className}>{metric.value}</strong>
          </div>
        ))}
      </div>
      <MonitoringHealthStatusBar statusData={statusData} locale={locale} t={t} showRate={false} />
      <div className={styles.accountScopeText}>{scopeText}</div>
    </section>
  );
}

function AccountTokenMetricGrid({ metrics }: { metrics: AccountSummaryMetric[] }) {
  const getTokenMetricToneClassName = (key: string) => {
    if (key === 'input-tokens') return styles.accountMetricIconInput;
    if (key === 'output-tokens') return styles.accountMetricIconOutput;
    if (key === 'cached-tokens') return styles.accountMetricIconCached;
    return styles.accountMetricIconTotal;
  };

  return (
    <section className={styles.accountTokenPanel}>
      <div className={styles.accountSectionHeader}>
        <strong>Token 使用</strong>
      </div>
      <div className={styles.accountOverviewMetricGrid}>
        {metrics.map((metric) => (
          <div key={metric.key} className={styles.accountOverviewMetricCard}>
            <span className={styles.accountOverviewMetricLabel}>
              <span
                className={[styles.accountMetricIcon, getTokenMetricToneClassName(metric.key)]
                  .filter(Boolean)
                  .join(' ')}
                aria-hidden="true"
              />
              {metric.label}
            </span>
            <strong className={[styles.accountOverviewMetricValue, metric.valueClassName].filter(Boolean).join(' ')}>
              {metric.value}
            </strong>
          </div>
        ))}
      </div>
    </section>
  );
}

function AccountModelUsageList({
  row,
  hasPrices,
  locale,
  t,
  limit = 1,
}: {
  row: MonitoringAccountRow;
  hasPrices: boolean;
  locale: string;
  t: TFunction;
  limit?: number;
}) {
  const [showAll, setShowAll] = useState(false);
  const [expandedModels, setExpandedModels] = useState<Record<string, boolean>>({});
  const hasExtraModels = row.models.length > limit;
  const visibleModels = showAll ? row.models : row.models.slice(0, limit);
  const toggleModel = (key: string) => setExpandedModels((previous) => ({ ...previous, [key]: !previous[key] }));

  return (
    <section className={styles.accountModelListPanel}>
      <div className={styles.accountSectionHeader}>
        <strong>Top 模型</strong>
        {hasExtraModels ? (
          <button type="button" className={styles.accountModelViewAllButton} onClick={() => setShowAll((previous) => !previous)}>
            {showAll ? '收起' : '查看全部'}
          </button>
        ) : null}
      </div>

      {visibleModels.length > 0 ? (
        <div className={styles.accountModelList}>
          {visibleModels.map((model) => {
            const modelKey = `${row.id}-${model.model}`;
            const isModelExpanded = Boolean(expandedModels[modelKey]);
            return (
              <div key={modelKey} className={styles.accountModelItem}>
                <button
                  type="button"
                  className={styles.accountModelRow}
                  onClick={() => toggleModel(modelKey)}
                  aria-expanded={isModelExpanded}
                >
                  <span className={styles.accountModelName} title={model.model}>{model.model}</span>
                  <span className={styles.accountModelMetaLine}>
                    <span className={styles.accountModelStat}><small>请求</small><strong>{formatCompactNumber(model.totalCalls)}</strong></span>
                    <span className={styles.accountModelStat}><small>成功率</small><strong className={getSuccessRateClassName(model.successRate)}>{formatPercent(model.successRate)}</strong></span>
                    <span className={styles.accountModelStat}><small>Token</small><strong>{formatCompactNumber(model.totalTokens)}</strong></span>
                    <span className={styles.accountModelStat}><small>金额</small><strong>{hasPrices ? formatUsd(model.totalCost) : '--'}</strong></span>
                    <span className={styles.accountModelChevron} aria-hidden="true">{isModelExpanded ? <IconChevronDown size={14} /> : '›'}</span>
                  </span>
                </button>
                {isModelExpanded ? (
                  <div className={styles.accountModelExpanded}>
                    <div className={styles.accountModelExpandedItem}><small>{t('monitoring.input_tokens')}</small><strong>{formatCompactNumber(model.inputTokens)}</strong></div>
                    <div className={styles.accountModelExpandedItem}><small>{t('monitoring.output_tokens')}</small><strong>{formatCompactNumber(model.outputTokens)}</strong></div>
                    <div className={styles.accountModelExpandedItem}><small>{t('monitoring.cached_tokens')}</small><strong>{formatCompactNumber(model.cachedTokens)}</strong></div>
                    <div className={styles.accountModelExpandedItem}><small>{t('monitoring.latest_request_time')}</small><strong>{new Date(model.lastSeenAt).toLocaleString(locale)}</strong></div>
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      ) : (
        <div className={styles.emptyBlockSmall}>暂无模型数据</div>
      )}
    </section>
  );
}

function AccountQuotaPanel({
  quotaState,
  quotaEntries,
  locale,
  t,
  onRefreshQuota,
}: {
  quotaState?: AccountQuotaState;
  quotaEntries: AccountQuotaEntry[];
  locale: string;
  t: TFunction;
  onRefreshQuota: () => void;
}) {
  const quotaLoading = quotaState?.status === 'loading';
  const lastQuotaSync = quotaState?.lastRefreshedAt && Number.isFinite(quotaState.lastRefreshedAt)
    ? new Date(quotaState.lastRefreshedAt).toLocaleString(locale)
    : '';
  const quotaTitle = quotaEntries.length === 1 ? quotaEntries[0].providerLabel : quotaEntries.length > 1 ? t('quota_management.title') : t('codex_quota.title');

  const renderRefreshButton = () => (
    <button type="button" className={styles.quotaRefreshButton} onClick={onRefreshQuota} disabled={quotaLoading}>
      <IconRefreshCw size={14} className={quotaLoading ? styles.refreshIconSpinning : styles.refreshIcon} />
      <span>{t('codex_quota.refresh_button')}</span>
    </button>
  );

  return (
    <section className={styles.quotaSection}>
      <div className={styles.quotaSectionHeader}>
        <div className={styles.quotaSectionTitleGroup}>
          <strong>{quotaTitle}</strong>
          {lastQuotaSync ? <span>{`${t('monitoring.last_sync')}: ${lastQuotaSync}`}</span> : null}
        </div>
        {renderRefreshButton()}
      </div>

      {quotaLoading && quotaEntries.length === 0 ? <div className={styles.quotaStateMessage}>{t('codex_quota.loading')}</div> : null}
      {!quotaLoading && quotaState?.status === 'error' && quotaEntries.length === 0 ? (
        <div className={styles.quotaStateMessage}>{t('codex_quota.load_failed', { message: quotaState.error || t('common.unknown_error') })}</div>
      ) : null}
      {!quotaLoading && quotaState?.status === 'success' && quotaEntries.length === 0 ? <div className={styles.quotaStateMessage}>{t('monitoring.account_quota_empty')}</div> : null}
      {!quotaState && quotaEntries.length === 0 ? <div className={styles.quotaStateMessage}>{t('monitoring.account_quota_empty')}</div> : null}

      {quotaEntries.length > 0 ? (
        <div className={styles.quotaEntryGrid}>
          {quotaEntries.map((entry) => (
            <div key={entry.key} className={styles.quotaEntryCard}>
              <div className={styles.quotaEntryHeader}>
                <div className={styles.quotaEntryMain}>
                  <strong>{entry.authLabel}</strong>
                </div>
              </div>

              {entry.quota?.status === 'loading' ? (
                <div className={styles.quotaStateMessage}>{t(`${entry.config.i18nPrefix}.loading`)}</div>
              ) : entry.quota?.status === 'error' ? (
                <div className={styles.quotaStateMessage}>
                  {t(`${entry.config.i18nPrefix}.load_failed`, { message: resolveQuotaErrorMessage(t, entry.quota) })}
                </div>
              ) : hasUsableQuotaContent(entry.quota) ? (
                <div className={quotaStyles.quotaSection}>
                  {entry.config.renderQuotaItems(entry.quota!, t, QUOTA_RENDER_HELPERS)}
                </div>
              ) : (
                <div className={styles.quotaStateMessage}>{t(`${entry.config.i18nPrefix}.idle`)}</div>
              )}
            </div>
          ))}
        </div>
      ) : null}
    </section>
  );
}

function AccountExpandedDetails({
  row,
  hasPrices,
  locale,
  t,
  quotaState,
  quotaEntries,
  onRefreshQuota,
}: {
  row: MonitoringAccountRow;
  hasPrices: boolean;
  locale: string;
  t: TFunction;
  quotaState?: AccountQuotaState;
  quotaEntries: AccountQuotaEntry[];
  onRefreshQuota: () => void;
}) {
  return (
    <div className={styles.accountOverviewCardBody}>
      <AccountQuotaPanel quotaState={quotaState} quotaEntries={quotaEntries} locale={locale} t={t} onRefreshQuota={onRefreshQuota} />
      <AccountModelUsageList row={row} hasPrices={hasPrices} locale={locale} t={t} />
    </div>
  );
}

function AccountOverviewCard({
  row,
  hasPrices,
  locale,
  t,
  isExpanded,
  statusData,
  scopeText,
  quotaState,
  quotaEntries,
  onToggle,
  onRefreshQuota,
}: {
  row: MonitoringAccountRow;
  hasPrices: boolean;
  locale: string;
  t: TFunction;
  isExpanded: boolean;
  statusData: AccountStatusData;
  scopeText: string;
  quotaState?: AccountQuotaState;
  quotaEntries: AccountQuotaEntry[];
  onToggle: () => void;
  onRefreshQuota: () => void;
}) {
  const summaryMetrics = buildAccountSummaryMetrics(row, hasPrices, locale, t);
  const cardMetrics = sortAccountOverviewCardMetrics(summaryMetrics);
  const tone = getAccountHealthTone(row);
  const latestRequestText = new Date(row.lastSeenAt).toLocaleString(locale);
  const accountLabel = buildAccountCardFileName(row, quotaEntries);
  const providerText = buildAccountCardProviderText(row);

  return (
    <Card
      className={[
        styles.accountOverviewCard,
        isExpanded ? styles.accountOverviewCardExpanded : '',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <div className={styles.accountOverviewCardHeader}>
        <div className={styles.accountTitleRow}>
          <button
            type="button"
            className={styles.accountButton}
            onClick={onToggle}
            aria-expanded={isExpanded}
            title={accountLabel}
          >
            <span className={styles.accountExpandGlyph} aria-hidden="true">
              {isExpanded ? <IconChevronUp size={15} /> : <IconChevronDown size={15} />}
            </span>
            <span className={styles.accountIdentityLine}>
              <span className={[styles.accountStatusDot, getAccountStatusDotClassName(tone)].filter(Boolean).join(' ')} aria-hidden="true" />
              <span className={styles.accountButtonLabel}>{accountLabel}</span>
            </span>
          </button>
          <span className={`${styles.accountHealthBadge} ${styles[`accountHealthBadge${tone}`]}`}>
            {tone === 'good' ? '健康' : tone === 'warn' ? '波动' : '异常'}
          </span>
        </div>
        <div className={styles.accountMetaRow}>
          <span className={styles.accountOverviewCardTimestamp} title={providerText}>{providerText}</span>
          <span className={styles.accountMetaSeparator}>·</span>
          <span className={styles.accountOverviewCardTimestamp}>{`最近请求时间: ${latestRequestText}`}</span>
        </div>
      </div>

      <AccountHealthStatusPanel row={row} hasPrices={hasPrices} locale={locale} t={t} statusData={statusData} scopeText={scopeText} />
      <AccountTokenMetricGrid metrics={cardMetrics} />

      {isExpanded ? (
        <AccountExpandedDetails
          row={row}
          hasPrices={hasPrices}
          locale={locale}
          t={t}
          quotaState={quotaState}
          quotaEntries={quotaEntries}
          onRefreshQuota={onRefreshQuota}
        />
      ) : null}
    </Card>
  );
}

function AccountStatsPanel({
  rows,
  metric,
  emptyText,
  hasPrices,
  locale,
  t,
  rangeLabel,
  expandedAccounts,
  accountQuotaStates,
  accountQuotaEntriesByAccount,
  onMetricChange,
  onToggleAccount,
  onRefreshQuota,
}: {
  rows: MonitoringAccountRow[];
  metric: RankingMetric;
  emptyText: string;
  hasPrices: boolean;
  locale: string;
  t: TFunction;
  rangeLabel: string;
  expandedAccounts: Record<string, boolean>;
  accountQuotaStates: Record<string, AccountQuotaState>;
  accountQuotaEntriesByAccount: Map<string, AccountQuotaEntry[]>;
  onMetricChange: (metric: RankingMetric) => void;
  onToggleAccount: (accountId: string, account: string) => void;
  onRefreshQuota: (account: string) => void;
}) {
  const ACCOUNT_CARD_MIN_WIDTH = 330;
  const ACCOUNT_CARD_GAP = 16;
  const ROWS_PER_PAGE = 2;

  const [cardPage, setCardPage] = useState(0);
  const gridRef = useRef<HTMLDivElement>(null);
  const [gridCols, setGridCols] = useState(3);

  useEffect(() => {
    const el = gridRef.current;
    if (!el) return;
    const update = () => {
      const cols = Math.max(1, Math.floor((el.clientWidth + ACCOUNT_CARD_GAP) / (ACCOUNT_CARD_MIN_WIDTH + ACCOUNT_CARD_GAP)));
      setGridCols(cols);
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const itemsPerPage = gridCols * ROWS_PER_PAGE;
  const totalPages = Math.max(1, Math.ceil(rows.length / itemsPerPage));
  const safePageIndex = Math.min(cardPage, totalPages - 1);
  const visibleRows = rows.slice(safePageIndex * itemsPerPage, (safePageIndex + 1) * itemsPerPage);

  useEffect(() => {
    setCardPage(0);
  }, [rows.length]);

  return (
    <Card className={styles.accountStatsCard}>
      <div className={styles.rankingHeader}>
        <div>
          <h3>账号统计</h3>
          <p>按账号查看健康状态、Token 使用与近期请求活跃度。</p>
        </div>
        <RankingMetricSwitch value={metric} onChange={onMetricChange} disabledCost={!hasPrices} />
      </div>

      {rows.length > 0 ? (
        <>
          <div ref={gridRef} className={styles.accountOverviewCardGrid}>
            {visibleRows.map((row) => {
              const statusData = buildAccountStatusData(row.recentPattern, row.lastSeenAt);
              return (
                <AccountOverviewCard
                  key={row.id}
                  row={row}
                  hasPrices={hasPrices}
                  locale={locale}
                  t={t}
                  isExpanded={Boolean(expandedAccounts[row.id])}
                  statusData={statusData}
                  scopeText={formatAccountOverviewScopeText(rangeLabel)}
                  quotaState={accountQuotaStates[row.account]}
                  quotaEntries={accountQuotaEntriesByAccount.get(row.account) ?? []}
                  onToggle={() => onToggleAccount(row.id, row.account)}
                  onRefreshQuota={() => onRefreshQuota(row.account)}
                />
              );
            })}
          </div>
          {totalPages > 1 && (
            <div className={styles.accountCardPagination}>
              <button
                type="button"
                className={styles.accountCardPageButton}
                disabled={safePageIndex === 0}
                onClick={() => setCardPage((p) => Math.max(0, p - 1))}
                aria-label="上一页"
              >
                ‹
              </button>
              <span className={styles.accountCardPageInfo}>
                {safePageIndex + 1} / {totalPages}
              </span>
              <button
                type="button"
                className={styles.accountCardPageButton}
                disabled={safePageIndex >= totalPages - 1}
                onClick={() => setCardPage((p) => Math.min(totalPages - 1, p + 1))}
                aria-label="下一页"
              >
                ›
              </button>
            </div>
          )}
        </>
      ) : (
        <div className={styles.emptyBlockSmall}>{emptyText}</div>
      )}
    </Card>
  );
}

function Panel({ title, subtitle, extra, children, className }: PanelProps) {
  return (
    <Card className={[styles.panel, className].filter(Boolean).join(' ')}>
      <div className={styles.panelHeader}>
        <div>
          <h2 className={styles.panelTitle}>{title}</h2>
          {subtitle ? <p className={styles.panelSubtitle}>{subtitle}</p> : null}
        </div>
        {extra ? <div className={styles.panelExtra}>{extra}</div> : null}
      </div>
      {children}
    </Card>
  );
}

function StatusBadge({ tone, children }: { tone: MonitoringStatusTone; children: ReactNode }) {
  return <span className={`${styles.statusBadge} ${styles[`tone${tone}`]}`}>{children}</span>;
}

function RecentPattern({
  pattern,
  variant = 'default',
  label,
}: {
  pattern: boolean[];
  variant?: 'default' | 'plain';
  label?: string;
}) {
  const normalized = pattern.length > 0 ? pattern : Array.from({ length: 10 }, () => true);
  const successCount = normalized.filter(Boolean).length;
  const failureCount = normalized.length - successCount;
  const ariaLabel = label ?? `Recent ${normalized.length} requests: ${successCount} succeeded, ${failureCount} failed`;
  const containerClassName = [
    styles.patternBars,
    variant === 'plain' ? styles.patternBarsPlain : '',
  ]
    .filter(Boolean)
    .join(' ');
  const barClassName = [styles.patternBar, variant === 'plain' ? styles.patternBarPlain : '']
    .filter(Boolean)
    .join(' ');

  return (
    <div className={containerClassName} role="img" aria-label={ariaLabel}>
      {normalized.map((item, index) => (
        <span
          key={`${index}-${item ? 'success' : 'failed'}`}
          className={`${barClassName} ${item ? styles.patternSuccess : styles.patternFailed}`}
          aria-hidden="true"
        />
      ))}
    </div>
  );
}

export function MonitoringCenterPage() {
  const { t, i18n } = useTranslation();
  const config = useConfigStore((state) => state.config);
  const connectionStatus = useAuthStore((state) => state.connectionStatus);
  const showNotification = useNotificationStore((state) => state.showNotification);
  const quotaStore = useQuotaStore((state) => state);
  const [timeRange, setTimeRange] = useState<MonitoringTimeRange>('today');
  const [searchInput, setSearchInput] = useState('');
  const [autoRefreshMs, setAutoRefreshMs] = useState('5000');
  const [selectedAccount, setSelectedAccount] = useState('all');
  const [selectedProvider, setSelectedProvider] = useState('all');
  const [selectedModel, setSelectedModel] = useState('all');
  const [selectedChannel, setSelectedChannel] = useState('all');
  const [selectedStatus, setSelectedStatus] = useState<StatusFilter>('all');
  const [expandedAccounts, setExpandedAccounts] = useState<Record<string, boolean>>({});
  const [isPriceModalOpen, setIsPriceModalOpen] = useState(false);
  const [priceModel, setPriceModel] = useState('');
  const [priceDraft, setPriceDraft] = useState<PriceDraft>(() => createPriceDraft());
  const [isImportingUsage, setIsImportingUsage] = useState(false);
  const importInputRef = useRef<HTMLInputElement | null>(null);
  const [accountQuotaStates, setAccountQuotaStates] = useState<Record<string, AccountQuotaState>>({});
  const [usageTrendRange, setUsageTrendRange] = useState<MonitoringTimeRange>('today');
  const [isUsageTrendHidden, setIsUsageTrendHidden] = useState(false);
  const [modelRankingMetric, setModelRankingMetric] = useState<RankingMetric>('requests');
  const [apiKeyRankingMetric, setApiKeyRankingMetric] = useState<RankingMetric>('requests');
  const [accountStatsMetric, setAccountStatsMetric] = useState<RankingMetric>('requests');
  const accountQuotaStatesRef = useRef<Record<string, AccountQuotaState>>({});
  const accountQuotaRequestIdsRef = useRef<Record<string, number>>({});
  const deferredSearch = useDeferredValue(searchInput);

  const {
    usage,
    loading: usageLoading,
    error: usageError,
    lastRefreshedAt,
    modelPrices,
    setModelPrices,
    loadUsage,
  } = useUsageData();

  const {
    loading: monitoringLoading,
    error: monitoringError,
    authFiles,
    allRows,
    filteredRows,
    refreshMeta,
  } = useMonitoringData({
    usage,
    config,
    modelPrices,
    timeRange,
    searchQuery: deferredSearch,
  });

  const refreshAll = useCallback(async () => {
    await Promise.all([loadUsage(), refreshMeta(false)]);
  }, [loadUsage, refreshMeta]);

  const refreshUsageOnly = useCallback(async () => {
    await loadUsage();
  }, [loadUsage]);

  const handleExportUsage = useCallback(async () => {
    if (connectionStatus !== 'connected') {
      showNotification(t('notification.connection_required'), 'warning');
      return;
    }

    try {
      const response = await apiClient.getRaw('/usage/export', { responseType: 'blob' });
      const blob = response.data instanceof Blob ? response.data : new Blob([response.data], { type: 'application/x-ndjson' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
      link.href = url;
      link.download = `usage-export-${timestamp}.jsonl`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      showNotification(t('usage_stats.export_success'), 'success');
    } catch (error) {
      showNotification(error instanceof Error ? error.message : String(error || t('common.unknown_error')), 'error');
    }
  }, [connectionStatus, showNotification, t]);

  const handleImportUsageClick = useCallback(() => {
    if (connectionStatus !== 'connected') {
      showNotification(t('notification.connection_required'), 'warning');
      return;
    }
    importInputRef.current?.click();
  }, [connectionStatus, showNotification, t]);

  const handleImportUsageFile = useCallback(
    async (event: ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0];
      event.target.value = '';
      if (!file) return;

      setIsImportingUsage(true);
      try {
        const content = await file.text();
        if (!content.trim()) {
          showNotification(t('usage_stats.import_invalid'), 'error');
          return;
        }

        const result = await apiClient.post<UsageImportResult>('/usage/import', content, {
          headers: { 'Content-Type': 'application/x-ndjson' },
        });
        showNotification(
          t('usage_stats.import_success', {
            added: result.added ?? 0,
            skipped: result.skipped ?? 0,
            total: result.total ?? 0,
            failed: result.failed ?? 0,
          }),
          (result.failed ?? 0) > 0 ? 'warning' : 'success'
        );
        await refreshAll();
      } catch (error) {
        showNotification(error instanceof Error ? error.message : String(error || t('common.unknown_error')), 'error');
      } finally {
        setIsImportingUsage(false);
      }
    },
    [refreshAll, showNotification, t]
  );

  useHeaderRefresh(refreshAll);
  useInterval(
    () => {
      void refreshUsageOnly().catch(() => {});
    },
    connectionStatus === 'connected' && Number(autoRefreshMs) > 0 ? Number(autoRefreshMs) : null
  );

  const overallLoading = usageLoading || monitoringLoading;
  const combinedError = [usageError, monitoringError].filter(Boolean).join('；');
  const hasPrices = Object.keys(modelPrices).length > 0;

  useEffect(() => {
    accountQuotaStatesRef.current = accountQuotaStates;
  }, [accountQuotaStates]);

  const setQuotaForConfig = useCallback(
    (quotaConfig: AnyQuotaConfig, updater: Record<string, QuotaStatusState> | ((prev: Record<string, QuotaStatusState>) => Record<string, QuotaStatusState>)) => {
      const setter = useQuotaStore.getState()[quotaConfig.storeSetter] as (value: typeof updater) => void;
      setter(updater);
    },
    []
  );

  const providerOptions = useMemo(
    () => [
      { value: 'all', label: t('monitoring.filter_all_providers') },
      ...Array.from(new Set(filteredRows.map((row) => row.provider)))
        .filter(Boolean)
        .sort((left, right) => left.localeCompare(right))
        .map((value) => ({ value, label: value })),
    ],
    [filteredRows, t]
  );

  const accountOptions = useMemo(
    () => [
      { value: 'all', label: t('monitoring.filter_all_accounts') },
      ...Array.from(new Map(filteredRows.map((row) => [row.account, row.account])).entries())
        .sort((left, right) => left[1].localeCompare(right[1]))
        .map(([value, label]) => ({ value, label })),
    ],
    [filteredRows, t]
  );

  const modelOptions = useMemo(
    () => [
      { value: 'all', label: t('monitoring.filter_all_models') },
      ...Array.from(new Set(filteredRows.map((row) => row.model)))
        .filter(Boolean)
        .sort((left, right) => left.localeCompare(right))
        .map((value) => ({ value, label: value })),
    ],
    [filteredRows, t]
  );

  const channelOptions = useMemo(
    () => [
      { value: 'all', label: t('monitoring.filter_all_channels') },
      ...Array.from(new Set(filteredRows.map((row) => row.channel)))
        .filter(Boolean)
        .sort((left, right) => left.localeCompare(right))
        .map((value) => ({ value, label: value })),
    ],
    [filteredRows, t]
  );

  const statusOptions = useMemo(
    () => [
      { value: 'all', label: t('monitoring.filter_all_statuses') },
      { value: 'success', label: t('monitoring.filter_status_success') },
      { value: 'failed', label: t('monitoring.filter_status_failed') },
    ],
    [t]
  );

  const priceModelOptions = useMemo(
    () => [
      { value: '', label: t('usage_stats.model_price_select_placeholder') },
      ...Array.from(new Set([...filteredRows.map((row) => row.model), ...Object.keys(modelPrices)]))
        .filter(Boolean)
        .sort((left, right) => left.localeCompare(right))
        .map((value) => ({ value, label: value })),
    ],
    [filteredRows, modelPrices, t]
  );

  const authFilesByAuthIndex = useMemo(() => {
    const map = new Map<string, AuthFileItem>();
    authFiles.forEach((file) => {
      const authIndex = normalizeAuthIndex(file['auth_index'] ?? file.authIndex);
      if (!authIndex || map.has(authIndex)) return;
      map.set(authIndex, file);
    });
    return map;
  }, [authFiles]);

  const scopedRows = useMemo(
    () =>
      filteredRows.filter((row) => {
        if (selectedAccount !== 'all' && row.account !== selectedAccount) {
          return false;
        }
        if (selectedProvider !== 'all' && row.provider !== selectedProvider) {
          return false;
        }
        if (selectedModel !== 'all' && row.model !== selectedModel) {
          return false;
        }
        if (selectedChannel !== 'all' && row.channel !== selectedChannel) {
          return false;
        }
        if (selectedStatus === 'success' && row.failed) {
          return false;
        }
        if (selectedStatus === 'failed' && !row.failed) {
          return false;
        }
        return true;
      }),
    [filteredRows, selectedAccount, selectedChannel, selectedModel, selectedProvider, selectedStatus]
  );

  const topStatsRows = useMemo(() => allRows.filter((row) => row.statsIncluded), [allRows]);
  const todayStatsRows = useMemo(
    () => filterRowsByRange(topStatsRows, 'today'),
    [topStatsRows]
  );
  const trendStatsRows = useMemo(
    () => filterRowsByRange(topStatsRows, usageTrendRange),
    [topStatsRows, usageTrendRange]
  );
  const todayCost = useMemo(() => {
    const todayStart = new Date();
    todayStart.setHours(0, 0, 0, 0);
    const tomorrowStart = new Date(todayStart);
    tomorrowStart.setDate(tomorrowStart.getDate() + 1);
    return topStatsRows.reduce(
      (sum, row) => sum + (row.timestampMs >= todayStart.getTime() && row.timestampMs < tomorrowStart.getTime() ? row.totalCost : 0),
      0
    );
  }, [topStatsRows]);
  const yesterdayCost = useMemo(() => {
    const todayStart = new Date();
    todayStart.setHours(0, 0, 0, 0);
    const yesterdayStart = new Date(todayStart);
    yesterdayStart.setDate(yesterdayStart.getDate() - 1);
    return topStatsRows.reduce(
      (sum, row) => sum + (row.timestampMs >= yesterdayStart.getTime() && row.timestampMs < todayStart.getTime() ? row.totalCost : 0),
      0
    );
  }, [topStatsRows]);
  const usageTrendPoints = useMemo(() => buildTrendPoints(trendStatsRows, usageTrendRange), [trendStatsRows, usageTrendRange]);
  const tokenDistributionPoints = useMemo(() => buildTokenDistributionPoints(trendStatsRows), [trendStatsRows]);
  const trendSummary = useMemo(() => buildMonitoringSummary(trendStatsRows), [trendStatsRows]);
  const topSummary = useMemo(() => buildMonitoringSummary(topStatsRows), [topStatsRows]);
  const todaySummary = useMemo(() => buildMonitoringSummary(todayStatsRows), [todayStatsRows]);
  const modelRankingRows = useMemo(
    () => [...buildAccountRows(trendStatsRows, 'model')]
      .sort((left, right) => (
        getRankingMetricValue(right, modelRankingMetric) - getRankingMetricValue(left, modelRankingMetric)
        || right.totalTokens - left.totalTokens
        || right.totalCalls - left.totalCalls
      )),
    [modelRankingMetric, trendStatsRows]
  );
  const modelRankingMetricTotal = useMemo(
    () => getRankingMetricTotalFromRows(trendStatsRows, modelRankingMetric),
    [modelRankingMetric, trendStatsRows]
  );
  const apiKeyRankingRows = useMemo(
    () => [...buildAccountRows(trendStatsRows, 'apiKey')]
      .sort((left, right) => (
        getRankingMetricValue(right, apiKeyRankingMetric) - getRankingMetricValue(left, apiKeyRankingMetric)
        || right.totalCalls - left.totalCalls
        || right.totalCost - left.totalCost
      ))
      .slice(0, 8),
    [apiKeyRankingMetric, trendStatsRows]
  );
  const apiKeyRankingMetricTotal = useMemo(
    () => getRankingMetricTotalFromRows(trendStatsRows, apiKeyRankingMetric),
    [apiKeyRankingMetric, trendStatsRows]
  );
  const accountStatsRows = useMemo(
    () => [...buildAccountRows(trendStatsRows, 'account')]
      .sort((left, right) => (
        getRankingMetricValue(right, accountStatsMetric) - getRankingMetricValue(left, accountStatsMetric)
        || right.totalCalls - left.totalCalls
        || right.totalTokens - left.totalTokens
        || right.totalCost - left.totalCost
        || right.lastSeenAt - left.lastSeenAt
      ))
      .slice(0, 6),
    [accountStatsMetric, trendStatsRows]
  );
  const accountStatsRangeLabel = useMemo(() => buildUsageTrendRangeLabel(usageTrendRange), [usageTrendRange]);
  const realtimeLogRows = useMemo(() => buildRealtimeLogRows(scopedRows), [scopedRows]);

  const trendAccountQuotaTargetsByAccount = useMemo(
    () => buildAccountQuotaTargetsByAccount(trendStatsRows, authFilesByAuthIndex),
    [authFilesByAuthIndex, trendStatsRows]
  );
  const trendAccountQuotaEntriesByAccount = useMemo(
    () => buildAccountQuotaEntriesByAccount(trendAccountQuotaTargetsByAccount, quotaStore, t),
    [trendAccountQuotaTargetsByAccount, quotaStore, t]
  );
  const quotaTargetsByAccountForLoading = trendAccountQuotaTargetsByAccount;

  const activeScopeRows = scopedRows;
  const scopedFailureCount = activeScopeRows.filter((row) => row.failed).length;
  const savedPriceEntries = useMemo(
    () => Object.entries(modelPrices).sort((left, right) => left[0].localeCompare(right[0])),
    [modelPrices]
  );

  const selectedFiltersCount =
    [selectedAccount, selectedProvider, selectedModel, selectedChannel, selectedStatus].filter(
      (value) => value !== 'all'
    ).length + (deferredSearch.trim() ? 1 : 0);

  const usageMetricCards: UsageMetricCard[] = [
    {
      key: 'traffic',
      title: '流量',
      label: '今日请求',
      value: formatCompactNumber(todaySummary.totalCalls),
      accent: 'blue',
      footer: [
        { label: '总请求', value: formatCompactNumber(topSummary.totalCalls) },
        { label: '总成功率', value: formatPercent(topSummary.successRate) },
      ],
    },
    {
      key: 'tokens',
      title: 'Token',
      label: '今日 Token',
      value: formatCompactNumber(todaySummary.totalTokens),
      accent: 'purple',
      footer: [
        { label: '总 Token', value: formatCompactNumber(topSummary.totalTokens) },
        { label: '输入/输出/推理', value: `${formatCompactNumber(topSummary.inputTokens)} / ${formatCompactNumber(topSummary.outputTokens)} / ${formatCompactNumber(topSummary.reasoningTokens)}` },
      ],
    },
    {
      key: 'cache',
      title: '缓存',
      label: '今日缓存命中率',
      value: formatPercent(todaySummary.inputTokens > 0 ? todaySummary.cachedTokens / todaySummary.inputTokens : 0),
      accent: 'green',
      footer: [
        { label: '今日缓存 Token', value: formatCompactNumber(todaySummary.cachedTokens) },
        { label: '总缓存命中', value: `${formatCompactNumber(topSummary.cachedTokens)} / ${formatPercent(topSummary.inputTokens > 0 ? topSummary.cachedTokens / topSummary.inputTokens : 0)}` },
      ],
    },
    {
      key: 'billing',
      title: '计费',
      label: '今日花费',
      value: hasPrices ? formatUsd(todayCost) : '--',
      accent: 'amber',
      footer: [
        { label: '较昨日', value: hasPrices ? formatDeltaPercent(todayCost, yesterdayCost) : '--' },
        { label: '总计花费', value: hasPrices ? formatUsd(topSummary.totalCost) : '--' },
      ],
    },
  ];

  const clearFilters = useCallback(() => {
    setSearchInput('');
    setSelectedAccount('all');
    setSelectedProvider('all');
    setSelectedModel('all');
    setSelectedChannel('all');
    setSelectedStatus('all');
  }, []);

  const loadAccountQuota = useCallback(
    async (account: string, force: boolean = false) => {
      const currentState = accountQuotaStatesRef.current[account];
      const targets = quotaTargetsByAccountForLoading.get(account) ?? [];
      const targetKey = targets.map((target) => target.key).join('|');
      if (!force && currentState && currentState.status !== 'idle' && currentState.targetKey === targetKey) {
        return;
      }

      const requestId = (accountQuotaRequestIdsRef.current[account] ?? 0) + 1;
      accountQuotaRequestIdsRef.current[account] = requestId;

      setAccountQuotaStates((previous) => ({
        ...previous,
        [account]: {
          status: 'loading',
          targetKey,
          lastRefreshedAt: previous[account]?.lastRefreshedAt,
        },
      }));

      if (targets.length === 0) {
        if (accountQuotaRequestIdsRef.current[account] !== requestId) return;
        setAccountQuotaStates((previous) => ({
          ...previous,
          [account]: {
            status: 'success',
            targetKey,
            lastRefreshedAt: Date.now(),
          },
        }));
        return;
      }

      targets.forEach((target) => {
        setQuotaForConfig(target.config, (prev) => ({
          ...prev,
          [target.fileName]: target.config.buildLoadingState(),
        }));
      });

      const settled = await Promise.allSettled(targets.map((target) => requestAccountQuota(target, t)));
      if (accountQuotaRequestIdsRef.current[account] !== requestId) return;

      settled.forEach((result, index) => {
        const target = targets[index];
        const quota = result.status === 'fulfilled'
          ? result.value
          : target.config.buildErrorState(
              result.reason instanceof Error ? result.reason.message : String(result.reason || t('common.unknown_error')),
              getStatusFromError(result.reason)
            ) as QuotaStatusState;

        setQuotaForConfig(target.config, (prev) => ({
          ...prev,
          [target.fileName]: quota,
        }));
      });

      const currentStore = useQuotaStore.getState();
      const entries = targets.map((target) => getQuotaForTarget(currentStore, target)).filter(Boolean) as QuotaStatusState[];
      const hasSuccess = entries.some((entry) => entry.status === 'success');
      const firstError = entries.find((entry) => entry.status === 'error')?.error;
      setAccountQuotaStates((previous) => ({
        ...previous,
        [account]: {
          status: hasSuccess ? 'success' : 'error',
          targetKey,
          error: hasSuccess ? '' : firstError || t('common.unknown_error'),
          lastRefreshedAt: Date.now(),
        },
      }));
    },
    [quotaTargetsByAccountForLoading, setQuotaForConfig, t]
  );

  const toggleAccountExpanded = useCallback((accountId: string, account: string) => {
    if (account && !expandedAccounts[accountId]) {
      void loadAccountQuota(account);
    }
    setExpandedAccounts((previous) => ({
      ...previous,
      [accountId]: !previous[accountId],
    }));
  }, [expandedAccounts, loadAccountQuota]);

  const handleAccountFilterChange = useCallback((value: string) => {
    setSelectedAccount(value);
  }, []);

  const handlePriceModelChange = useCallback(
    (value: string) => {
      setPriceModel(value);
      setPriceDraft(createPriceDraft(modelPrices[value]));
    },
    [modelPrices]
  );

  const handlePriceDraftChange = useCallback((field: keyof PriceDraft, value: string) => {
    setPriceDraft((previous) => ({ ...previous, [field]: value }));
  }, []);

  const resetPriceEditor = useCallback(() => {
    setPriceModel('');
    setPriceDraft(createPriceDraft());
  }, []);

  const handleSavePrice = useCallback(() => {
    if (!priceModel) {
      return;
    }

    const prompt = parsePriceValue(priceDraft.prompt);
    const completion = parsePriceValue(priceDraft.completion);
    const cache = priceDraft.cache.trim() === '' ? prompt : parsePriceValue(priceDraft.cache);

    setModelPrices({
      ...modelPrices,
      [priceModel]: {
        prompt,
        completion,
        cache,
      },
    });
  }, [modelPrices, priceDraft.cache, priceDraft.completion, priceDraft.prompt, priceModel, setModelPrices]);

  const handleDeletePrice = useCallback(
    (model: string) => {
      const nextPrices = { ...modelPrices };
      delete nextPrices[model];
      setModelPrices(nextPrices);

      if (priceModel === model) {
        resetPriceEditor();
      }
    },
    [modelPrices, priceModel, resetPriceEditor, setModelPrices]
  );

  return (
    <div className={styles.page}>
      {overallLoading && !usage ? (
        <div className={styles.loadingOverlay} aria-busy="true">
          <div className={styles.loadingOverlayContent}>
            <LoadingSpinner size={28} />
            <span>{t('common.loading')}</span>
          </div>
        </div>
      ) : null}

      <section className={styles.masthead}>
        <div className={styles.mastheadGlow} aria-hidden="true" />

        <div className={styles.mastheadCopy}>
          <div className={styles.titleRow}>
            <h1 className={styles.title}>{t('monitoring.title')}</h1>
            <div className={styles.titleActions}>
              <button
                type="button"
                className={`${styles.quickLinkButton} ${styles.mastheadActionButton}`}
                onClick={() => void handleExportUsage()}
              >
                {t('usage_stats.export')}
              </button>
              <button
                type="button"
                className={`${styles.quickLinkButton} ${styles.mastheadActionButton}`}
                onClick={handleImportUsageClick}
                disabled={isImportingUsage}
              >
                {isImportingUsage ? t('common.loading') : t('usage_stats.import')}
              </button>
              <button
                type="button"
                className={`${styles.quickLinkButton} ${styles.mastheadActionButton}`}
                onClick={() => setIsPriceModalOpen(true)}
              >
                {t('usage_stats.model_price_settings')}
              </button>
              <input
                ref={importInputRef}
                type="file"
                accept=".jsonl,.ndjson,.json,application/x-ndjson,application/json"
                className={styles.hiddenFileInput}
                onChange={handleImportUsageFile}
              />
            </div>
          </div>
          <p className={styles.subtitle}>{t('monitoring.console_subtitle')}</p>

          <div className={styles.usageStatsHero}>
            <TopUsageStats cards={usageMetricCards} />
          </div>
        </div>
      </section>

      {!isUsageTrendHidden ? (
        <section className={styles.usageTrendSection}>
          <UsageTrendHeader
            range={usageTrendRange}
            totalCalls={trendSummary.totalCalls}
            onRangeChange={setUsageTrendRange}
            onHide={() => setIsUsageTrendHidden(true)}
            t={t}
          />
          <div className={styles.usageTrendInsightsGrid}>
            <UsageTrendPanel
              points={usageTrendPoints}
              hasPrices={hasPrices}
              emptyText={t('monitoring.no_data')}
            />
            <ApiKeyRankingPanel
              title={t('monitoring.api_key_ranking_title')}
              subtitle={t('monitoring.api_key_ranking_desc')}
              rows={apiKeyRankingRows}
              metric={apiKeyRankingMetric}
              metricTotal={apiKeyRankingMetricTotal}
              onMetricChange={setApiKeyRankingMetric}
              emptyText={t('monitoring.no_data')}
              hasPrices={hasPrices}
            />
          </div>
          <div className={styles.rankingGrid}>
            <ModelStatsPanel
              title={t('monitoring.model_stats_title')}
              subtitle={t('monitoring.model_stats_desc')}
              rows={modelRankingRows}
              metric={modelRankingMetric}
              metricTotal={modelRankingMetricTotal}
              onMetricChange={setModelRankingMetric}
              emptyText={t('monitoring.no_data')}
              hasPrices={hasPrices}
            />
            <TokenDistributionPanel
              points={tokenDistributionPoints}
              emptyText={t('monitoring.no_data')}
            />
          </div>
          <AccountStatsPanel
            rows={accountStatsRows}
            metric={accountStatsMetric}
            emptyText={t('monitoring.no_data')}
            hasPrices={hasPrices}
            locale={i18n.language}
            t={t}
            rangeLabel={accountStatsRangeLabel}
            expandedAccounts={expandedAccounts}
            accountQuotaStates={accountQuotaStates}
            accountQuotaEntriesByAccount={trendAccountQuotaEntriesByAccount}
            onMetricChange={setAccountStatsMetric}
            onToggleAccount={toggleAccountExpanded}
            onRefreshQuota={(account) => void loadAccountQuota(account, true)}
          />
        </section>
      ) : (
        <section className={styles.usageTrendCollapsed}>
          <div>
            <h2>使用趋势</h2>
            <p>分析版块已隐藏，可随时重新显示。</p>
          </div>
          <button type="button" className={styles.usageTrendHideButton} onClick={() => setIsUsageTrendHidden(false)}>
            显示分析
          </button>
        </section>
      )}

      <Panel
        title={t('monitoring.analysis_tab_logs')}
        subtitle={
          selectedFiltersCount > 0
            ? t('monitoring.active_filters_hint', { count: selectedFiltersCount, rows: activeScopeRows.length })
            : t('monitoring.realtime_table_desc')
        }
        className={styles.realtimePanel}
        extra={
          <div className={styles.toolbarHeaderActions}>
            <Input
              value={searchInput}
              onChange={(event) => setSearchInput(event.target.value)}
              placeholder={t('monitoring.search_placeholder')}
              className={styles.toolbarHeaderSearchInput}
              rightElement={<IconSearch size={16} />}
              aria-label={t('monitoring.search_placeholder')}
            />
            <button type="button" className={styles.clearButton} onClick={clearFilters}>
              <IconSlidersHorizontal size={16} />
              <span>{t('monitoring.clear_filters')}</span>
            </button>
          </div>
        }
      >
        <div className={styles.toolbarControlRow}>
          <div className={styles.segmentedControl}>
            {TIME_RANGE_OPTIONS.map((option) => (
              <button
                key={option.value}
                type="button"
                className={`${styles.segmentButton} ${timeRange === option.value ? styles.segmentButtonActive : ''}`}
                onClick={() => setTimeRange(option.value)}
              >
                {t(option.labelKey)}
              </button>
            ))}
          </div>

          <div className={styles.refreshCluster}>
            <span className={styles.syncPill}>
              {t('monitoring.last_sync')}: {lastRefreshedAt ? lastRefreshedAt.toLocaleTimeString(i18n.language) : '--'}
            </span>

            <div className={styles.refreshControls}>
              <div className={styles.autoRefreshField}>
                <span className={styles.autoRefreshLabel}>
                  <IconTimer size={16} />
                  {t('monitoring.auto_refresh')}
                </span>
                <Select
                  className={styles.autoRefreshSelect}
                  triggerClassName={styles.autoRefreshSelectTrigger}
                  value={autoRefreshMs}
                  options={AUTO_REFRESH_OPTIONS.map((option) => ({
                    value: option.value,
                    label: t(option.labelKey),
                  }))}
                  onChange={setAutoRefreshMs}
                  ariaLabel={t('monitoring.auto_refresh')}
                  fullWidth={false}
                />
              </div>

              <button
                type="button"
                className={styles.refreshButton}
                onClick={() => void refreshAll()}
                disabled={overallLoading}
              >
                <IconRefreshCw size={16} className={overallLoading ? styles.refreshIconSpinning : styles.refreshIcon} />
                <span className={styles.refreshButtonLabel}>{t('usage_stats.refresh')}</span>
              </button>
            </div>
          </div>
        </div>

        <div className={styles.filterGrid}>
          <Select
            value={selectedAccount}
            options={accountOptions}
            onChange={handleAccountFilterChange}
            ariaLabel={t('monitoring.filter_account')}
          />
          <Select
            value={selectedProvider}
            options={providerOptions}
            onChange={setSelectedProvider}
            ariaLabel={t('monitoring.filter_provider')}
          />
          <Select
            value={selectedModel}
            options={modelOptions}
            onChange={setSelectedModel}
            ariaLabel={t('monitoring.filter_model')}
          />
          <Select
            value={selectedChannel}
            options={channelOptions}
            onChange={setSelectedChannel}
            ariaLabel={t('monitoring.filter_channel')}
          />
          <Select
            value={selectedStatus}
            options={statusOptions}
            onChange={(value) => setSelectedStatus(value as StatusFilter)}
            ariaLabel={t('monitoring.filter_status')}
          />
        </div>

        {combinedError ? <div className={styles.errorBox}>{combinedError}</div> : null}

        <div className={styles.inlineMetrics}>
          <span>{`${t('monitoring.log_rows')}: ${realtimeLogRows.length}`}</span>
          <span>{`${t('monitoring.recent_failures')}: ${scopedFailureCount}`}</span>
        </div>

        <div className={`${styles.tableWrapper} ${styles.tableScrollWrapper} ${styles.realtimeTableWrapper}`}>
          <table className={`${styles.table} ${styles.realtimeTable}`}>
            <thead>
              <tr>
                <th>{t('monitoring.column_type')}</th>
                <th>{t('monitoring.column_model')}</th>
                <th>{t('monitoring.recent_status')}</th>
                <th>{t('monitoring.request_status')}</th>
                <th>{t('monitoring.column_success_rate')}</th>
                <th>{t('monitoring.total_calls')}</th>
                <th>{t('monitoring.column_latency')}</th>
                <th>{t('monitoring.column_time')}</th>
                <th>{t('monitoring.this_call_usage')}</th>
                <th>{t('monitoring.this_call_cost')}</th>
              </tr>
            </thead>
            <tbody>
              {realtimeLogRows.slice(0, 150).map((row) => (
                <tr key={row.id} className={row.failed ? styles.logRowFailed : undefined}>
                  <td>
                    <div className={styles.primaryCell}>
                      <span>{row.provider}</span>
                      <small>{row.account || row.authLabel || row.accountMasked || '-'}</small>
                    </div>
                  </td>
                  <td>
                    <div className={styles.primaryCell}>
                      <span className={styles.monoCell}>{row.model}</span>
                      <small className={styles.monoCell}>{buildRealtimeMetaText(row)}</small>
                    </div>
                  </td>
                  <td>
                    <div className={styles.recentStatusCell}>
                      <RecentPattern
                        pattern={row.recentPattern}
                        variant="plain"
                        label={t('monitoring.recent_pattern_label', {
                          total: row.recentPattern.length,
                          success: row.recentPattern.filter(Boolean).length,
                          failure: row.recentPattern.filter((item) => !item).length,
                        })}
                      />
                    </div>
                  </td>
                  <td>
                    <StatusBadge tone={row.failed ? 'bad' : 'good'}>
                      {row.failed ? t('monitoring.result_failed') : t('monitoring.result_success')}
                    </StatusBadge>
                  </td>
                  <td
                    className={
                      row.successRate >= 0.95
                        ? styles.goodText
                        : row.successRate >= 0.85
                          ? styles.warnText
                          : styles.badText
                    }
                  >
                    {formatPercent(row.successRate)}
                  </td>
                  <td>{formatCompactNumber(row.requestCount)}</td>
                  <td>
                    <span
                      className={
                        row.latencyMs !== null && row.latencyMs >= 30000
                          ? styles.badText
                          : row.latencyMs !== null && row.latencyMs >= 15000
                            ? styles.warnText
                            : undefined
                      }
                    >
                      {formatDurationMs(row.latencyMs, { locale: i18n.language })}
                    </span>
                  </td>
                  <td>{new Date(row.timestampMs).toLocaleString(i18n.language)}</td>
                  <td>
                    <div className={styles.primaryCell}>
                      <span>{formatCompactNumber(row.totalTokens)}</span>
                      <small>{`I ${formatCompactNumber(row.inputTokens)} · O ${formatCompactNumber(row.outputTokens)} · C ${formatCompactNumber(row.cachedTokens)}`}</small>
                    </div>
                  </td>
                  <td>{hasPrices ? formatUsd(row.totalCost) : '--'}</td>
                </tr>
              ))}
              {realtimeLogRows.length === 0 ? (
                <tr>
                  <td colSpan={10}>
                    <div className={styles.emptyTable}>
                      {deferredSearch.trim() ? t('monitoring.no_filtered_data') : t('monitoring.no_data')}
                    </div>
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </Panel>

      <Modal
        open={isPriceModalOpen}
        onClose={() => setIsPriceModalOpen(false)}
        title={t('usage_stats.model_price_settings')}
        width={860}
        className={styles.monitorModal}
      >
        <div className={styles.priceEditor}>
          <div className={styles.priceGrid}>
            <div className={`${styles.priceField} ${styles.priceFieldModel}`}>
              <label>{t('usage_stats.model_name')}</label>
              <Select
                value={priceModel}
                options={priceModelOptions}
                onChange={handlePriceModelChange}
                ariaLabel={t('usage_stats.model_name')}
              />
            </div>
            <div className={`${styles.priceField} ${styles.priceFieldPrompt}`}>
              <label>{`${t('usage_stats.model_price_prompt')} ($/1M)`}</label>
              <Input
                type="number"
                value={priceDraft.prompt}
                onChange={(event) => handlePriceDraftChange('prompt', event.target.value)}
                placeholder="0.0000"
                step="0.0001"
              />
            </div>
            <div className={`${styles.priceField} ${styles.priceFieldCompletion}`}>
              <label>{`${t('usage_stats.model_price_completion')} ($/1M)`}</label>
              <Input
                type="number"
                value={priceDraft.completion}
                onChange={(event) => handlePriceDraftChange('completion', event.target.value)}
                placeholder="0.0000"
                step="0.0001"
              />
            </div>
            <div className={`${styles.priceField} ${styles.priceFieldCache}`}>
              <label>{`${t('usage_stats.model_price_cache')} ($/1M)`}</label>
              <Input
                type="number"
                value={priceDraft.cache}
                onChange={(event) => handlePriceDraftChange('cache', event.target.value)}
                placeholder="0.0000"
                step="0.0001"
              />
            </div>
          </div>

          <div className={styles.priceActionsBar}>
            <Button variant="secondary" size="sm" onClick={resetPriceEditor}>
              {t('common.cancel')}
            </Button>
            <Button variant="primary" size="sm" onClick={handleSavePrice} disabled={!priceModel}>
              {t('common.save')}
            </Button>
          </div>
        </div>

        <div className={styles.savedPricesList}>
          <div className={styles.savedPricesHeader}>{t('usage_stats.saved_prices')}</div>
          {savedPriceEntries.length > 0 ? (
            <div className={styles.savedPricesTableWrap}>
              <table className={styles.savedPricesTable}>
                <thead>
                  <tr>
                    <th>{t('usage_stats.model_name')}</th>
                    <th>{t('usage_stats.model_price_prompt')}</th>
                    <th>{t('usage_stats.model_price_completion')}</th>
                    <th>{t('usage_stats.model_price_cache')}</th>
                    <th>{t('common.action')}</th>
                  </tr>
                </thead>
                <tbody>
                  {savedPriceEntries.map(([model, price]) => (
                    <tr key={model}>
                      <td className={`${styles.monoCell} ${styles.savedPricesModelCell}`}>{model}</td>
                      <td>{formatPriceUnit(price.prompt)}</td>
                      <td>{formatPriceUnit(price.completion)}</td>
                      <td>{formatPriceUnit(price.cache)}</td>
                      <td className={styles.savedPricesActionsCell}>
                        <div className={styles.savedPricesActions}>
                          <button
                            type="button"
                            className={styles.inlineActionButton}
                            onClick={() => handlePriceModelChange(model)}
                          >
                            {t('common.edit')}
                          </button>
                          <button
                            type="button"
                            className={styles.inlineActionButton}
                            onClick={() => handleDeletePrice(model)}
                          >
                            {t('common.delete')}
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className={styles.emptyBlockSmall}>{t('usage_stats.model_price_empty')}</div>
          )}
        </div>
      </Modal>
    </div>
  );
}
