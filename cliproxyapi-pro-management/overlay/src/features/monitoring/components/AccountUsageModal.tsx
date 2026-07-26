import { useEffect, useMemo, useState, type CSSProperties } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import {
  IconChartColumnIncreasing,
  IconInfo,
  IconKey,
  IconModelCluster,
  IconRefreshCw,
  IconScrollText,
  IconSidebarMonitor,
  IconTimer,
} from '@/components/ui/icons';
import type {
  AccountUsageAPIKeyStat,
  AccountUsageDayStat,
  AccountUsageModelStat,
  AccountUsageRangeDays,
} from '@/services/api/accountUsage';
import { useConfigStore } from '@/stores';
import type { AuthFileItem } from '@/types';
import {
  formatCompactNumber,
  formatDurationMs,
  formatUsd,
  normalizeAuthIndex,
} from '@/utils/usage';
import { buildAccountUsageLogPath, ratio } from '../accountUsage';
import {
  buildConfiguredApiKeyMap,
  resolveConfiguredApiKeyLabel,
} from '../apiKeyIdentity';
import { useAccountUsage } from '../hooks/useAccountUsage';
import styles from './AccountUsageModal.module.scss';

type AccountUsageTab = 'overview' | 'detail' | 'quality';
type DistributionMetric = 'requests' | 'tokens' | 'cost';
type DistributionItem = AccountUsageModelStat | AccountUsageAPIKeyStat;

type AccountUsageModalProps = {
  file: AuthFileItem | null;
  onClose: () => void;
};

const RANGE_OPTIONS: AccountUsageRangeDays[] = [7, 30, 90, 0];
const TAB_OPTIONS: AccountUsageTab[] = ['overview', 'detail', 'quality'];
const DISTRIBUTION_OPTIONS: DistributionMetric[] = ['requests', 'tokens', 'cost'];
const MODEL_COLORS = ['#0f8a7c', '#2563eb', '#f59e0b', '#8b5cf6', '#dc2626'];

function formatPercent(value: number): string {
  return `${(Math.min(Math.max(value, 0), 1) * 100).toFixed(1)}%`;
}

function readText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function resolveAccountLabel(file: AuthFileItem | null, authIndex: string | null): string {
  const idToken = file?.id_token && typeof file.id_token === 'object'
    ? file.id_token as Record<string, unknown>
    : null;
  const authSuffix = authIndex?.includes(':') ? authIndex.split(':').slice(1).join(':') : authIndex;
  const candidates = [
    readText(file?.email),
    readText(idToken?.email),
    readText(file?.account),
    readText(idToken?.preferred_username),
    readText(file?.label),
    readText(authSuffix),
  ].filter(Boolean);
  if (candidates.length > 0) return candidates[0];

  const fileName = readText(file?.name).replace(/\.json$/i, '').replace(/_oauth_creds$/i, '');
  return fileName || authIndex || '-';
}

function distributionValue(item: DistributionItem, metric: DistributionMetric): number {
  if (metric === 'tokens') return item.tokens;
  if (metric === 'cost') return item.estimatedCost;
  return item.requests;
}

function formatDistributionValue(value: number, metric: DistributionMetric): string {
  if (metric === 'cost') return formatUsd(value);
  return formatCompactNumber(value);
}

function buildModelDonutGradient(models: AccountUsageModelStat[]): string {
  const total = models.reduce((sum, model) => sum + model.requests, 0);
  if (total <= 0) return 'var(--bg-tertiary)';

  let cursor = 0;
  const segments = models.slice(0, MODEL_COLORS.length).map((model, index) => {
    const start = cursor;
    cursor += (model.requests / total) * 100;
    return `${MODEL_COLORS[index]} ${start}% ${cursor}%`;
  });
  if (cursor < 100) segments.push(`var(--border-color) ${cursor}% 100%`);
  return `conic-gradient(${segments.join(', ')})`;
}

export function AccountUsageModal({ file, onClose }: AccountUsageModalProps) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const configuredApiKeyValues = useConfigStore((state) => state.config?.apiKeys);
  const [rangeDays, setRangeDays] = useState<AccountUsageRangeDays>(30);
  const [activeTab, setActiveTab] = useState<AccountUsageTab>('overview');
  const [modelMetric, setModelMetric] = useState<DistributionMetric>('requests');
  const [apiKeyMetric, setApiKeyMetric] = useState<DistributionMetric>('requests');
  const authIndex = normalizeAuthIndex(file?.['auth_index'] ?? file?.authIndex);
  const accountLabel = resolveAccountLabel(file, authIndex);
  const { data, loading, error, refresh } = useAccountUsage(authIndex, rangeDays, Boolean(file));
  const detail = data?.detail ?? null;
  const configuredApiKeys = useMemo(
    () => buildConfiguredApiKeyMap(configuredApiKeyValues),
    [configuredApiKeyValues]
  );

  useEffect(() => {
    setActiveTab('overview');
    setModelMetric('requests');
    setApiKeyMetric('requests');
  }, [authIndex]);

  const dateFormatter = useMemo(
    () => new Intl.DateTimeFormat(i18n.resolvedLanguage || i18n.language, {
      month: '2-digit',
      day: '2-digit',
    }),
    [i18n.language, i18n.resolvedLanguage]
  );
  const chartDays = detail?.history.slice(-30) ?? [];
  const chartMaxCost = Math.max(...chartDays.map((item) => item.estimatedCost), 1);
  const activeDays = Math.max(detail?.activeDays ?? 0, 1);
  const topModel = detail?.models.reduce<AccountUsageModelStat | undefined>(
    (top, model) => (!top || model.requests > top.requests ? model : top),
    undefined
  );
  const cacheHitRate = ratio(detail?.cacheHitRequests ?? 0, detail?.totalRequests ?? 0);
  const modelTotal = detail?.models.reduce((sum, item) => sum + distributionValue(item, modelMetric), 0) ?? 0;
  const apiKeyTotal = detail?.apiKeys.reduce((sum, item) => sum + distributionValue(item, apiKeyMetric), 0) ?? 0;
  const rangeLabel = t(rangeDays === 0 ? 'account_usage.range_all' : `account_usage.range_${rangeDays}d`);
  const statusKey = file?.unavailable
    ? 'account_usage.status_unavailable'
    : file?.disabled
      ? 'account_usage.status_disabled'
      : 'account_usage.status_active';

  const openRequestLogs = () => {
    if (!detail || !authIndex) return;
    const path = buildAccountUsageLogPath(authIndex, detail.fromMs, detail.toMs);
    onClose();
    navigate(path);
  };

  const formatDay = (item: AccountUsageDayStat | undefined) => (
    item ? dateFormatter.format(new Date(item.bucketStartMs)) : '--'
  );

  const title = (
    <div className={styles.titleBlock}>
      <span>{t('account_usage.modal_title')}</span>
      <span aria-hidden="true">—</span>
      <strong>{accountLabel}</strong>
    </div>
  );

  const renderDistributionControls = (
    metric: DistributionMetric,
    onChange: (metric: DistributionMetric) => void,
    label: string
  ) => (
    <div className={`${styles.segmented} ${styles.metricSegmented}`} role="group" aria-label={label}>
      {DISTRIBUTION_OPTIONS.map((option) => (
        <button
          key={option}
          type="button"
          className={metric === option ? styles.segmentActive : undefined}
          onClick={() => onChange(option)}
        >
          {t(option === 'cost' ? 'account_usage.estimated_cost' : `account_usage.${option}`)}
        </button>
      ))}
    </div>
  );

  return (
    <Modal
      open={Boolean(file)}
      title={title}
      onClose={onClose}
      width={1180}
      className={styles.modal}
    >
      <div className={styles.accountHeader}>
        <div className={styles.accountIdentity}>
          <span className={styles.accountIcon}><IconChartColumnIncreasing size={23} /></span>
          <div>
            <strong>{accountLabel}</strong>
            <p>{t('account_usage.range_summary', { range: rangeLabel })}</p>
          </div>
        </div>
        <div className={styles.accountActions}>
          <span className={`${styles.statusBadge} ${file?.disabled || file?.unavailable ? styles.statusMuted : ''}`}>
            {t(statusKey)}
          </span>
          <Button
            variant="secondary"
            size="sm"
            className={styles.logButton}
            onClick={openRequestLogs}
            disabled={!detail || !authIndex}
          >
            <IconScrollText size={15} />
            {t('account_usage.view_logs')}
          </Button>
          <div className={styles.tabs} role="tablist" aria-label={t('account_usage.title')}>
            {TAB_OPTIONS.map((tab) => {
              const TabIcon = tab === 'overview'
                ? IconSidebarMonitor
                : tab === 'detail'
                  ? IconModelCluster
                  : IconTimer;
              return (
                <button
                  key={tab}
                  type="button"
                  role="tab"
                  aria-selected={activeTab === tab}
                  className={activeTab === tab ? styles.tabActive : undefined}
                  onClick={() => setActiveTab(tab)}
                >
                  <TabIcon size={15} />
                  {t(`account_usage.tab_${tab}`)}
                </button>
              );
            })}
          </div>
        </div>
      </div>

      <div className={styles.rangeRow}>
        <span>{t('account_usage.range_short')}</span>
        <div className={styles.segmented} role="group" aria-label={t('account_usage.range_label')}>
          {RANGE_OPTIONS.map((days) => (
            <button
              key={days}
              type="button"
              className={rangeDays === days ? styles.segmentActive : undefined}
              onClick={() => setRangeDays(days)}
            >
              {t(days === 0 ? 'account_usage.range_all' : `account_usage.range_${days}d`)}
            </button>
          ))}
        </div>
        <button
          type="button"
          className={styles.refreshButton}
          onClick={refresh}
          disabled={loading || !authIndex}
          title={t('common.refresh')}
          aria-label={t('common.refresh')}
        >
          <IconRefreshCw size={16} />
        </button>
      </div>

      {!authIndex ? <div className={styles.state}>{t('account_usage.missing_auth_index')}</div> : null}
      {authIndex && loading && !detail ? <div className={styles.state}>{t('common.loading')}</div> : null}
      {authIndex && error && !detail ? (
        <div className={`${styles.state} ${styles.errorState}`}>
          <strong>{t('account_usage.load_failed')}</strong>
          <span>{error}</span>
          <Button variant="secondary" size="sm" onClick={refresh}>{t('common.retry')}</Button>
        </div>
      ) : null}

      {detail ? (
        <div className={styles.content} aria-busy={loading}>
          {activeTab === 'overview' ? (
            <>
              <div className={styles.overviewGrid}>
                <section className={`${styles.panel} ${styles.costPanel}`}>
                  <div className={styles.costHeading}>
                    <div>
                      <span>{t('account_usage.period_total_cost', { range: rangeLabel })}</span>
                      <strong>{formatUsd(detail.estimatedCost)}</strong>
                    </div>
                    <span className={styles.coverageBadge}>
                      {t('account_usage.priced_coverage_short', { priced: detail.pricedRequests, total: detail.totalRequests })}
                    </span>
                  </div>
                  <div className={styles.primaryMetrics}>
                    <div><IconChartColumnIncreasing size={17} /><span>{t('account_usage.requests')}</span><strong>{formatCompactNumber(detail.totalRequests)}</strong></div>
                    <div><IconModelCluster size={17} /><span>{t('account_usage.tokens')}</span><strong>{formatCompactNumber(detail.totalTokens)}</strong></div>
                    <div><IconTimer size={17} /><span>{t('account_usage.average_response')}</span><strong>{formatDurationMs(detail.averageLatencyMs)}</strong></div>
                  </div>
                  <div className={styles.trendSection}>
                    <div className={styles.trendHeader}>
                      <div><strong>{t('account_usage.daily_trend')}</strong><span>{t('account_usage.daily_trend_desc')}</span></div>
                      <div><span>{t('account_usage.highest_cost_day')}</span><strong>{formatDay(detail.highestCostDay)} · {formatUsd(detail.highestCostDay?.estimatedCost ?? 0)}</strong></div>
                    </div>
                    {chartDays.length > 0 ? (
                      <div className={styles.trendChart}>
                        {chartDays.map((item) => (
                          <div
                            key={item.bucketStartMs}
                            className={styles.trendColumn}
                            title={`${dateFormatter.format(new Date(item.bucketStartMs))}: ${formatUsd(item.estimatedCost)} / ${item.requests} / ${formatCompactNumber(item.tokens)}`}
                          >
                            <span style={{ height: `${Math.max((item.estimatedCost / chartMaxCost) * 100, 4)}%` }} />
                          </div>
                        ))}
                      </div>
                    ) : <div className={styles.empty}>{t('account_usage.no_data')}</div>}
                  </div>
                </section>

                <div className={styles.overviewSide}>
                  <section className={`${styles.panel} ${styles.summaryPanel}`}>
                    <div className={styles.panelTitle}><span><IconSidebarMonitor size={18} /></span><h3>{t('account_usage.today')}</h3></div>
                    <dl className={styles.definitionList}>
                      <div><dt>{t('account_usage.requests')}</dt><dd>{formatCompactNumber(detail.today.requests)}</dd></div>
                      <div><dt>{t('account_usage.tokens')}</dt><dd>{formatCompactNumber(detail.today.tokens)}</dd></div>
                      <div><dt>{t('account_usage.estimated_cost')}</dt><dd>{formatUsd(detail.today.estimatedCost)}</dd></div>
                    </dl>
                  </section>
                  <section className={`${styles.panel} ${styles.summaryPanel}`}>
                    <div className={styles.panelTitle}><span><IconTimer size={18} /></span><h3>{t('account_usage.active_day_average')}</h3></div>
                    <dl className={styles.definitionList}>
                      <div><dt>{t('account_usage.estimated_cost')}</dt><dd>{formatUsd(detail.estimatedCost / activeDays)}</dd></div>
                      <div><dt>{t('account_usage.requests')}</dt><dd>{formatCompactNumber(detail.totalRequests / activeDays)}</dd></div>
                      <div><dt>{t('account_usage.active_days_label')}</dt><dd>{detail.activeDays} / {detail.periodDays || detail.activeDays}</dd></div>
                    </dl>
                  </section>
                </div>
              </div>

              <div className={styles.highlightGrid}>
                <section className={styles.highlightCard}>
                  <span>{t('account_usage.highest_cost_day')}</span>
                  <strong>{formatDay(detail.highestCostDay)}</strong>
                  <small>{formatUsd(detail.highestCostDay?.estimatedCost ?? 0)} · {formatCompactNumber(detail.highestCostDay?.requests ?? 0)} {t('account_usage.requests')}</small>
                </section>
                <section className={styles.highlightCard}>
                  <span>{t('account_usage.highest_request_day')}</span>
                  <strong>{formatDay(detail.highestRequestDay)}</strong>
                  <small>{formatCompactNumber(detail.highestRequestDay?.requests ?? 0)} {t('account_usage.requests')} · {formatUsd(detail.highestRequestDay?.estimatedCost ?? 0)}</small>
                </section>
                <section className={styles.highlightCard}>
                  <span>{t('account_usage.highest_model')}</span>
                  <strong title={topModel?.model}>{topModel?.model || '--'}</strong>
                  <small>{formatCompactNumber(topModel?.requests ?? 0)} {t('account_usage.requests')} · {formatCompactNumber(topModel?.tokens ?? 0)} {t('account_usage.tokens')}</small>
                </section>
              </div>
            </>
          ) : null}

          {activeTab === 'detail' ? (
            <>
              <div className={styles.detailGrid}>
                <section className={`${styles.panel} ${styles.distributionPanel}`}>
                  <div className={styles.sectionHeader}>
                    <div><h3>{t('account_usage.model_distribution')}</h3><p>{t('account_usage.current_top', { value: topModel?.model || '--' })}</p></div>
                    {renderDistributionControls(modelMetric, setModelMetric, t('account_usage.model_distribution'))}
                  </div>
                  {detail.models.length > 0 ? (
                    <div className={styles.modelDistribution}>
                      <div className={styles.donut} style={{ background: buildModelDonutGradient(detail.models) }}>
                        <div><strong>{formatCompactNumber(detail.totalRequests)}</strong><span>{t('account_usage.requests')}</span></div>
                      </div>
                      <div className={styles.distributionRows}>
                        {detail.models.map((model, index) => {
                          const value = distributionValue(model, modelMetric);
                          const share = ratio(value, modelTotal);
                          return (
                            <div
                              key={model.model}
                              className={styles.distributionRow}
                              style={{ '--row-color': MODEL_COLORS[index % MODEL_COLORS.length] } as CSSProperties}
                            >
                              <div className={styles.distributionRowHeader}><strong><i />{model.model || '-'}</strong><span>{formatPercent(share)}</span></div>
                              <div className={styles.progressTrack}><span style={{ width: `${share * 100}%` }} /></div>
                              <div className={styles.distributionRowMeta}><strong>{formatDistributionValue(value, modelMetric)}</strong><span>{formatCompactNumber(model.tokens)} {t('account_usage.tokens')} · {formatUsd(model.estimatedCost)}</span></div>
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  ) : <div className={styles.empty}>{t('account_usage.no_data')}</div>}
                </section>

                <section className={`${styles.panel} ${styles.tokenPanel}`}>
                  <div className={styles.panelTitle}><span><IconModelCluster size={18} /></span><h3>{t('account_usage.token_composition')}</h3></div>
                  <div className={styles.compositionList}>
                    {[
                      ['input', detail.inputTokens],
                      ['output', detail.outputTokens],
                      ['reasoning', detail.reasoningTokens],
                      ['cache', detail.cacheTokens],
                    ].map(([key, value]) => (
                      <div key={key} className={styles.compositionRow}>
                        <div><span>{t(`account_usage.${key}_tokens`)}</span><strong>{formatCompactNumber(Number(value))}</strong></div>
                        <div className={styles.progressTrack}><span style={{ width: `${ratio(Number(value), detail.totalTokens) * 100}%` }} /></div>
                      </div>
                    ))}
                  </div>
                </section>
              </div>

              <section className={`${styles.panel} ${styles.keyPanel}`}>
                <div className={styles.sectionHeader}>
                  <div className={styles.panelTitle}><span><IconKey size={18} /></span><div><h3>{t('account_usage.api_key_distribution')}</h3><p>{t('account_usage.api_key_count', { count: detail.apiKeys.length })}</p></div></div>
                  {renderDistributionControls(apiKeyMetric, setApiKeyMetric, t('account_usage.api_key_distribution'))}
                </div>
                {detail.apiKeys.length > 0 ? (
                  <div className={`${styles.distributionRows} ${styles.keyRows}`}>
                    {detail.apiKeys.map((item, index) => {
                      const value = distributionValue(item, apiKeyMetric);
                      const share = ratio(value, apiKeyTotal);
                      const label = resolveConfiguredApiKeyLabel(
                        item.apiKeyHash,
                        configuredApiKeys,
                        t('account_usage.unattributed'),
                        t('account_usage.api_key_unknown')
                      );
                      return (
                        <div
                          key={`${item.apiKeyHash || 'unattributed'}-${index}`}
                          className={styles.distributionRow}
                          style={{ '--row-color': MODEL_COLORS[index % MODEL_COLORS.length] } as CSSProperties}
                        >
                          <div className={styles.distributionRowHeader}><strong><i /><code title={label}>{label}</code></strong><span>{formatPercent(share)}</span></div>
                          <div className={styles.progressTrack}><span style={{ width: `${share * 100}%` }} /></div>
                          <div className={styles.distributionRowMeta}><strong>{formatDistributionValue(value, apiKeyMetric)}</strong><span>{formatCompactNumber(item.tokens)} {t('account_usage.tokens')} · {formatUsd(item.estimatedCost)}</span></div>
                        </div>
                      );
                    })}
                  </div>
                ) : <div className={styles.empty}>{t('account_usage.no_data')}</div>}
              </section>

              <div className={styles.detailStats}>
                <div><span>{t('account_usage.active_days_label')}</span><strong>{detail.activeDays} / {detail.periodDays || detail.activeDays}</strong></div>
                <div><span>{t('account_usage.daily_tokens')}</span><strong>{formatCompactNumber(detail.totalTokens / activeDays)}</strong></div>
                <div><span>{t('account_usage.cache_hit_rate')}</span><strong>{formatPercent(cacheHitRate)}</strong></div>
                <div><span>{t('account_usage.average_response')}</span><strong>{formatDurationMs(detail.averageLatencyMs)}</strong></div>
              </div>
            </>
          ) : null}

          {activeTab === 'quality' ? (
            <>
              <div className={styles.qualityHeading}>
                <h3>{t('account_usage.quality_signals')}</h3>
                <p>{t('account_usage.quality_signals_desc')}</p>
              </div>
              <div className={styles.qualityGrid}>
                <section className={`${styles.qualityCard} ${ratio(detail.failureCount, detail.totalRequests) <= 0.01 ? styles.qualitySuccess : styles.qualityDanger}`}>
                  <div><span><IconSidebarMonitor size={18} /></span><strong>{t('account_usage.error_rate')}</strong></div>
                  <b>{formatPercent(ratio(detail.failureCount, detail.totalRequests))}</b>
                  <small>{t('account_usage.failure_samples', { count: detail.failureCount })}</small>
                </section>
                <section className={`${styles.qualityCard} ${styles.qualityWarning}`}>
                  <div><span><IconRefreshCw size={18} /></span><strong>{t('account_usage.retry_attempts')}</strong></div>
                  <b>{formatCompactNumber(detail.retryAttempts)}</b>
                  <small>{t('account_usage.retry_coverage', { samples: detail.retrySamples, total: detail.totalRequests })}</small>
                </section>
                <section className={`${styles.qualityCard} ${styles.qualityDanger}`}>
                  <div><span><IconTimer size={18} /></span><strong>{t('account_usage.average_ttft')}</strong></div>
                  <b>{formatDurationMs(detail.averageTtftMs)}</b>
                  <small>{t('account_usage.sample_coverage', { samples: detail.ttftSamples, total: detail.totalRequests })}</small>
                </section>
                <section className={`${styles.qualityCard} ${styles.qualityDanger}`}>
                  <div><span><IconChartColumnIncreasing size={18} /></span><strong>{t('account_usage.p95_response')}</strong></div>
                  <b>{formatDurationMs(detail.p95LatencyMs)}</b>
                  <small>{t('account_usage.sample_coverage', { samples: detail.latencySamples, total: detail.totalRequests })}</small>
                </section>
                <section className={`${styles.qualityCard} ${styles.qualityNeutral}`}>
                  <div><span><IconScrollText size={18} /></span><strong>{t('account_usage.stream_share')}</strong></div>
                  <b>{formatPercent(ratio(detail.streamRequests, detail.totalRequests))}</b>
                  <small>{t('account_usage.stream_samples', { stream: detail.streamRequests, total: detail.totalRequests })}</small>
                </section>
              </div>
              <section className={styles.qualityNote}>
                <IconInfo size={17} />
                <div><strong>{t('account_usage.quality_notes')}</strong><p>{t('account_usage.retry_note')}</p></div>
              </section>
            </>
          ) : null}
        </div>
      ) : null}
    </Modal>
  );
}
