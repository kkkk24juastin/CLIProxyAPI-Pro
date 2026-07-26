import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/Button';
import { Modal } from '@/components/ui/Modal';
import { IconRefreshCw, IconScrollText } from '@/components/ui/icons';
import type { AuthFileItem } from '@/types';
import { normalizeAuthIndex } from '@/utils/usage';
import {
  formatCompactNumber,
  formatDurationMs,
  formatUsd,
} from '@/utils/usage';
import {
  buildAccountUsageLogPath,
  maskAccountUsageAPIKeyHash,
  ratio,
} from '../accountUsage';
import { useAccountUsage } from '../hooks/useAccountUsage';
import type {
  AccountUsageDayStat,
  AccountUsageRangeDays,
} from '@/services/api/accountUsage';
import styles from './AccountUsageModal.module.scss';

type AccountUsageTab = 'overview' | 'detail' | 'quality';

type AccountUsageModalProps = {
  file: AuthFileItem | null;
  onClose: () => void;
};

const RANGE_OPTIONS: AccountUsageRangeDays[] = [7, 30, 90, 0];
const TAB_OPTIONS: AccountUsageTab[] = ['overview', 'detail', 'quality'];

function formatPercent(value: number): string {
  return `${(Math.min(Math.max(value, 0), 1) * 100).toFixed(1)}%`;
}

export function AccountUsageModal({ file, onClose }: AccountUsageModalProps) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [rangeDays, setRangeDays] = useState<AccountUsageRangeDays>(30);
  const [activeTab, setActiveTab] = useState<AccountUsageTab>('overview');
  const authIndex = normalizeAuthIndex(file?.['auth_index'] ?? file?.authIndex);
  const { data, loading, error, refresh } = useAccountUsage(authIndex, rangeDays, Boolean(file));
  const detail = data?.detail ?? null;

  useEffect(() => {
    setActiveTab('overview');
  }, [authIndex]);

  const dateFormatter = useMemo(
    () => new Intl.DateTimeFormat(i18n.resolvedLanguage || i18n.language, {
      month: 'short',
      day: 'numeric',
    }),
    [i18n.language, i18n.resolvedLanguage]
  );
  const chartDays = detail?.history.slice(-30) ?? [];
  const chartMaxRequests = Math.max(...chartDays.map((item) => item.requests), 1);
  const activeDays = Math.max(detail?.activeDays ?? 0, 1);

  const openRequestLogs = () => {
    if (!detail || !authIndex) return;
    const path = buildAccountUsageLogPath(authIndex, detail.fromMs, detail.toMs);
    onClose();
    navigate(path);
  };

  const renderDayHighlight = (item: AccountUsageDayStat | undefined, metric: 'cost' | 'requests') => {
    if (!item) return '--';
    const day = dateFormatter.format(new Date(item.bucketStartMs));
    const value = metric === 'cost' ? formatUsd(item.estimatedCost) : formatCompactNumber(item.requests);
    return `${day} · ${value}`;
  };

  const title = (
    <div className={styles.titleBlock}>
      <span>{t('account_usage.title')}</span>
      {file ? <small>{file.name}</small> : null}
    </div>
  );

  return (
    <Modal
      open={Boolean(file)}
      title={title}
      onClose={onClose}
      width={980}
      className={styles.modal}
      footer={(
        <>
          <Button variant="secondary" onClick={openRequestLogs} disabled={!detail || !authIndex}>
            <IconScrollText size={16} />
            {t('account_usage.view_logs')}
          </Button>
          <Button variant="secondary" onClick={onClose}>{t('common.close')}</Button>
        </>
      )}
    >
      <div className={styles.headerRow}>
        <div className={styles.identity}>
          <span>{t('account_usage.auth_index')}</span>
          <code>{authIndex ?? t('account_usage.missing_auth_index')}</code>
        </div>
        <div className={styles.rangeActions}>
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
      </div>

      <div className={styles.tabs} role="tablist" aria-label={t('account_usage.title')}>
        {TAB_OPTIONS.map((tab) => (
          <button
            key={tab}
            type="button"
            role="tab"
            aria-selected={activeTab === tab}
            className={activeTab === tab ? styles.tabActive : undefined}
            onClick={() => setActiveTab(tab)}
          >
            {t(`account_usage.tab_${tab}`)}
          </button>
        ))}
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
              <div className={styles.metricGrid}>
                <div className={styles.metricItem}>
                  <span>{t('account_usage.estimated_cost')}</span>
                  <strong>{formatUsd(detail.estimatedCost)}</strong>
                  <small>{t('account_usage.priced_coverage', { priced: detail.pricedRequests, total: detail.totalRequests })}</small>
                </div>
                <div className={styles.metricItem}>
                  <span>{t('account_usage.requests')}</span>
                  <strong>{formatCompactNumber(detail.totalRequests)}</strong>
                  <small>{t('account_usage.success_failure', { success: detail.successCount, failure: detail.failureCount })}</small>
                </div>
                <div className={styles.metricItem}>
                  <span>{t('account_usage.tokens')}</span>
                  <strong>{formatCompactNumber(detail.totalTokens)}</strong>
                  <small>{t('account_usage.active_days', { count: detail.activeDays })}</small>
                </div>
                <div className={styles.metricItem}>
                  <span>{t('account_usage.average_response')}</span>
                  <strong>{formatDurationMs(detail.averageLatencyMs)}</strong>
                  <small>{t('account_usage.sample_coverage', { samples: detail.latencySamples, total: detail.totalRequests })}</small>
                </div>
              </div>

              <section className={styles.section}>
                <div className={styles.sectionHeader}>
                  <div>
                    <h3>{t('account_usage.daily_trend')}</h3>
                    <p>{t('account_usage.daily_trend_desc')}</p>
                  </div>
                </div>
                {chartDays.length > 0 ? (
                  <div className={styles.trendChart}>
                    {chartDays.map((item) => (
                      <div
                        key={item.bucketStartMs}
                        className={styles.trendColumn}
                        title={`${dateFormatter.format(new Date(item.bucketStartMs))}: ${item.requests} / ${formatCompactNumber(item.tokens)} / ${formatUsd(item.estimatedCost)}`}
                      >
                        <span className={styles.trendValue}>{formatCompactNumber(item.requests)}</span>
                        <div className={styles.trendTrack}>
                          <span style={{ height: `${Math.max((item.requests / chartMaxRequests) * 100, 4)}%` }} />
                        </div>
                        <small>{dateFormatter.format(new Date(item.bucketStartMs))}</small>
                      </div>
                    ))}
                  </div>
                ) : <div className={styles.empty}>{t('account_usage.no_data')}</div>}
              </section>

              <div className={styles.summaryGrid}>
                <section className={styles.section}>
                  <h3>{t('account_usage.today')}</h3>
                  <dl className={styles.definitionList}>
                    <div><dt>{t('account_usage.requests')}</dt><dd>{formatCompactNumber(detail.today.requests)}</dd></div>
                    <div><dt>{t('account_usage.tokens')}</dt><dd>{formatCompactNumber(detail.today.tokens)}</dd></div>
                    <div><dt>{t('account_usage.estimated_cost')}</dt><dd>{formatUsd(detail.today.estimatedCost)}</dd></div>
                  </dl>
                </section>
                <section className={styles.section}>
                  <h3>{t('account_usage.active_day_average')}</h3>
                  <dl className={styles.definitionList}>
                    <div><dt>{t('account_usage.requests')}</dt><dd>{formatCompactNumber(detail.totalRequests / activeDays)}</dd></div>
                    <div><dt>{t('account_usage.tokens')}</dt><dd>{formatCompactNumber(detail.totalTokens / activeDays)}</dd></div>
                    <div><dt>{t('account_usage.estimated_cost')}</dt><dd>{formatUsd(detail.estimatedCost / activeDays)}</dd></div>
                  </dl>
                </section>
                <section className={styles.section}>
                  <h3>{t('account_usage.highlights')}</h3>
                  <dl className={styles.definitionList}>
                    <div><dt>{t('account_usage.highest_cost_day')}</dt><dd>{renderDayHighlight(detail.highestCostDay, 'cost')}</dd></div>
                    <div><dt>{t('account_usage.highest_request_day')}</dt><dd>{renderDayHighlight(detail.highestRequestDay, 'requests')}</dd></div>
                  </dl>
                </section>
              </div>
            </>
          ) : null}

          {activeTab === 'detail' ? (
            <>
              <section className={styles.section}>
                <h3>{t('account_usage.token_composition')}</h3>
                <div className={styles.compositionList}>
                  {[
                    ['input', detail.inputTokens],
                    ['output', detail.outputTokens],
                    ['reasoning', detail.reasoningTokens],
                    ['cache', detail.cacheTokens],
                  ].map(([key, value]) => (
                    <div key={key} className={styles.compositionRow}>
                      <span>{t(`account_usage.${key}_tokens`)}</span>
                      <div><span style={{ width: `${ratio(Number(value), detail.totalTokens) * 100}%` }} /></div>
                      <strong>{formatCompactNumber(Number(value))}</strong>
                    </div>
                  ))}
                </div>
              </section>
              <section className={styles.section}>
                <h3>{t('account_usage.model_distribution')}</h3>
                <div className={styles.tableWrapper}>
                  <table>
                    <thead><tr><th>{t('account_usage.model')}</th><th>{t('account_usage.requests')}</th><th>{t('account_usage.tokens')}</th><th>{t('account_usage.estimated_cost')}</th></tr></thead>
                    <tbody>
                      {detail.models.map((model) => (
                        <tr key={model.model}><td><code>{model.model || '-'}</code></td><td>{formatCompactNumber(model.requests)}</td><td>{formatCompactNumber(model.tokens)}</td><td>{formatUsd(model.estimatedCost)}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                {detail.models.length === 0 ? <div className={styles.empty}>{t('account_usage.no_data')}</div> : null}
              </section>
              <section className={styles.section}>
                <h3>{t('account_usage.api_key_distribution')}</h3>
                <div className={styles.tableWrapper}>
                  <table>
                    <thead><tr><th>{t('account_usage.api_key')}</th><th>{t('account_usage.requests')}</th><th>{t('account_usage.tokens')}</th><th>{t('account_usage.estimated_cost')}</th></tr></thead>
                    <tbody>
                      {detail.apiKeys.map((item, index) => (
                        <tr key={`${item.apiKeyHash || 'unattributed'}-${index}`}><td><code>{maskAccountUsageAPIKeyHash(item.apiKeyHash, t('account_usage.unattributed'))}</code></td><td>{formatCompactNumber(item.requests)}</td><td>{formatCompactNumber(item.tokens)}</td><td>{formatUsd(item.estimatedCost)}</td></tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                {detail.apiKeys.length === 0 ? <div className={styles.empty}>{t('account_usage.no_data')}</div> : null}
              </section>
            </>
          ) : null}

          {activeTab === 'quality' ? (
            <>
              <div className={`${styles.metricGrid} ${styles.qualityMetricGrid}`}>
                <div className={styles.metricItem}><span>{t('account_usage.error_rate')}</span><strong>{formatPercent(ratio(detail.failureCount, detail.totalRequests))}</strong><small>{t('account_usage.failure_samples', { count: detail.failureCount })}</small></div>
                <div className={styles.metricItem}><span>{t('account_usage.retry_attempts')}</span><strong>{formatCompactNumber(detail.retryAttempts)}</strong><small>{t('account_usage.retry_coverage', { samples: detail.retrySamples, total: detail.totalRequests })}</small></div>
                <div className={styles.metricItem}><span>{t('account_usage.average_ttft')}</span><strong>{formatDurationMs(detail.averageTtftMs)}</strong><small>{t('account_usage.sample_coverage', { samples: detail.ttftSamples, total: detail.totalRequests })}</small></div>
                <div className={styles.metricItem}><span>{t('account_usage.p95_response')}</span><strong>{formatDurationMs(detail.p95LatencyMs)}</strong><small>{t('account_usage.sample_coverage', { samples: detail.latencySamples, total: detail.totalRequests })}</small></div>
                <div className={styles.metricItem}><span>{t('account_usage.stream_share')}</span><strong>{formatPercent(ratio(detail.streamRequests, detail.totalRequests))}</strong><small>{t('account_usage.stream_samples', { stream: detail.streamRequests, total: detail.totalRequests })}</small></div>
              </div>
              <section className={styles.section}>
                <h3>{t('account_usage.quality_notes')}</h3>
                <p className={styles.note}>{t('account_usage.retry_note')}</p>
              </section>
            </>
          ) : null}
        </div>
      ) : null}
    </Modal>
  );
}
