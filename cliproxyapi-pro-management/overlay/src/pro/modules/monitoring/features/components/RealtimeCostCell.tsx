import {
  useCallback,
  useId,
  useRef,
  useState,
  type CSSProperties,
} from 'react';
import { createPortal } from 'react-dom';
import type { TFunction } from 'i18next';
import { IconInfo } from '@/components/ui/icons';
import type { RealtimeLogRow } from '../realtimeLogPresentation';
import { resolvePricingMode } from '../modelPricePresentation';
import { formatCompactNumber, formatUsdPrecise } from '@/pro/modules/monitoring/features/usage';
import styles from '../monitoring.module.scss';

type RealtimeCostTooltipPosition = {
  top: number;
  left: number;
  arrowTop: number;
  placement: 'left' | 'right';
};

const REALTIME_COST_TOOLTIP_WIDTH = 336;
const REALTIME_COST_TOOLTIP_MARGIN = 12;

const formatCostTierLabel = (value: string) => value
  .trim()
  .replace(/[-_]+/g, ' ')
  .replace(/\b\w/g, (character) => character.toUpperCase());

const calculateMillionTokenRate = (cost: number, tokens: number): number | null => (
  tokens > 0 ? (cost / tokens) * 1_000_000 : null
);

const formatMillionTokenRate = (rate: number | null) => rate === null
  ? '--'
  : `$${rate.toLocaleString(undefined, { minimumFractionDigits: 4, maximumFractionDigits: 4 })} / 1M Token`;

export function RealtimeCostCell({ row, hasPrices, t }: {
  row: RealtimeLogRow;
  hasPrices: boolean;
  t: TFunction;
}) {
  const cellRef = useRef<HTMLSpanElement>(null);
  const tooltipId = useId();
  const [tooltipPosition, setTooltipPosition] = useState<RealtimeCostTooltipPosition | null>(null);
  const breakdown = row.costBreakdown;

  const showTooltip = useCallback((element: HTMLElement | null) => {
    if (!element || typeof window === 'undefined') return;
    const rect = element.getBoundingClientRect();
    const detailRowCount = breakdown
      ? 8 + [breakdown.cacheReadTokens, breakdown.cacheWriteTokens, breakdown.reasoningTokens].filter((tokens) => tokens > 0).length
        + (breakdown.matchedServiceTier ? 1 : 0)
        + (breakdown.requestedSpeed || breakdown.effectiveSpeed || breakdown.matchedSpeed ? 4 : 0)
      : 1;
    const estimatedHeight = Math.min(420, 70 + detailRowCount * 31);
    const placement = rect.left >= REALTIME_COST_TOOLTIP_WIDTH + REALTIME_COST_TOOLTIP_MARGIN * 2 ? 'left' : 'right';
    const unclampedLeft = placement === 'left'
      ? rect.left - REALTIME_COST_TOOLTIP_WIDTH - REALTIME_COST_TOOLTIP_MARGIN
      : rect.right + REALTIME_COST_TOOLTIP_MARGIN;
    const left = Math.min(
      Math.max(REALTIME_COST_TOOLTIP_MARGIN, unclampedLeft),
      Math.max(REALTIME_COST_TOOLTIP_MARGIN, window.innerWidth - REALTIME_COST_TOOLTIP_WIDTH - REALTIME_COST_TOOLTIP_MARGIN)
    );
    const centerY = rect.top + rect.height / 2;
    const top = Math.min(
      Math.max(REALTIME_COST_TOOLTIP_MARGIN, centerY - estimatedHeight / 2),
      Math.max(REALTIME_COST_TOOLTIP_MARGIN, window.innerHeight - estimatedHeight - REALTIME_COST_TOOLTIP_MARGIN)
    );
    setTooltipPosition({
      top,
      left,
      placement,
      arrowTop: Math.min(Math.max(22, centerY - top), estimatedHeight - 22),
    });
  }, [breakdown]);

  const hideTooltip = useCallback(() => setTooltipPosition(null), []);

  if (!hasPrices && !breakdown) return <span>--</span>;

  const conditionalCosts = breakdown ? [
    { key: 'cache-read', tokens: breakdown.cacheReadTokens, label: t('monitoring.cost_detail_cache_read'), cost: breakdown.cacheReadCost },
    { key: 'cache-write', tokens: breakdown.cacheWriteTokens, label: t('monitoring.cost_detail_cache_write'), cost: breakdown.cacheWriteCost },
    { key: 'reasoning', tokens: breakdown.reasoningTokens, label: t('monitoring.cost_detail_reasoning'), cost: breakdown.reasoningCost },
  ].filter((item) => item.tokens > 0 || item.cost > 0) : [];
  const requestedTier = breakdown?.requestedServiceTier || breakdown?.serviceTier || row.serviceTier;
  const effectiveTier = breakdown?.effectiveServiceTier || row.effectiveServiceTier;
  const matchedTier = breakdown?.matchedServiceTier || '';
  const requestedSpeed = breakdown?.requestedSpeed || breakdown?.speed || row.speed;
  const effectiveSpeed = breakdown?.effectiveSpeed || row.effectiveSpeed;
  const matchedSpeed = breakdown?.matchedSpeed || '';
  const requestedTierLabel = requestedTier
    ? formatCostTierLabel(requestedTier)
    : t('monitoring.cost_detail_standard');
  const effectiveTierLabel = effectiveTier
    ? formatCostTierLabel(effectiveTier)
    : t('monitoring.cost_detail_tier_unavailable');
  const resolvedPricingMode = breakdown ? resolvePricingMode(breakdown) : 'base';
  const billingMode = resolvedPricingMode === 'service_tier'
    ? t('monitoring.cost_detail_service_tier_mode')
    : resolvedPricingMode === 'speed'
      ? t('monitoring.cost_detail_speed_mode')
      : resolvedPricingMode === 'context'
      ? t('monitoring.cost_detail_context_mode', { size: formatCompactNumber(breakdown?.contextTierSize ?? 0) })
      : resolvedPricingMode === 'legacy_unknown'
        ? t('monitoring.cost_detail_legacy_unknown_mode')
        : t('monitoring.cost_detail_standard');

  return (
    <span
      ref={cellRef}
      className={styles.realtimeCostCell}
      onMouseEnter={() => showTooltip(cellRef.current)}
      onMouseLeave={hideTooltip}
    >
      <span className={styles.realtimeCostValue}>{formatUsdPrecise(row.totalCost)}</span>
      <button
        type="button"
        className={styles.realtimeCostInfoButton}
        aria-label={t('monitoring.cost_detail_open')}
        aria-describedby={tooltipPosition ? tooltipId : undefined}
        onFocus={(event) => showTooltip(event.currentTarget)}
        onBlur={hideTooltip}
      >
        <IconInfo size={16} />
      </button>
      {tooltipPosition && typeof document !== 'undefined' ? createPortal(
        <div
          id={tooltipId}
          role="tooltip"
          className={styles.realtimeCostTooltip}
          data-placement={tooltipPosition.placement}
          style={{
            top: tooltipPosition.top,
            left: tooltipPosition.left,
            '--realtime-cost-arrow-top': `${tooltipPosition.arrowTop}px`,
          } as CSSProperties}
        >
          <strong className={styles.realtimeCostTooltipTitle}>{t('monitoring.cost_detail_title')}</strong>
          {breakdown ? (
            <div className={styles.realtimeCostTooltipRows}>
              <div><span>{t('monitoring.cost_detail_input')}</span><strong>{formatUsdPrecise(breakdown.inputCost)}</strong></div>
              <div><span>{t('monitoring.cost_detail_output')}</span><strong>{formatUsdPrecise(breakdown.outputCost)}</strong></div>
              {conditionalCosts.map((item) => (
                <div key={item.key}><span>{item.label}</span><strong>{formatUsdPrecise(item.cost)}</strong></div>
              ))}
              <div className={styles.realtimeCostTooltipDivider} aria-hidden="true" />
              <div><span>{t('monitoring.cost_detail_input_rate')}</span><strong className={styles.realtimeCostRateInput}>{formatMillionTokenRate(calculateMillionTokenRate(breakdown.inputCost, breakdown.inputTokens))}</strong></div>
              <div><span>{t('monitoring.cost_detail_output_rate')}</span><strong className={styles.realtimeCostRateOutput}>{formatMillionTokenRate(calculateMillionTokenRate(breakdown.outputCost, breakdown.outputTokens))}</strong></div>
              <div><span>{t('monitoring.cost_detail_requested_tier')}</span><strong>{requestedTierLabel}</strong></div>
              <div><span>{t('monitoring.cost_detail_actual_tier')}</span><strong>{effectiveTierLabel}</strong></div>
              {matchedTier ? <div><span>{t('monitoring.cost_detail_matched_tier')}</span><strong>{formatCostTierLabel(matchedTier)}</strong></div> : null}
              <div><span>{t('monitoring.cost_detail_tier_basis')}</span><strong>{breakdown.serviceTierSource === 'response'
                ? t('monitoring.cost_detail_response_authoritative')
                : breakdown.serviceTierSource === 'codex_oauth_request'
                  ? t('monitoring.cost_detail_codex_oauth_request')
                : breakdown.serviceTierSource === 'request_fallback'
                  ? t('monitoring.cost_detail_request_fallback')
                  : breakdown.serviceTierSource === 'none'
                    ? t('monitoring.cost_detail_tier_not_applicable')
                    : t('monitoring.cost_detail_legacy_unknown_mode')}</strong></div>
              {requestedSpeed ? <div><span>{t('monitoring.cost_detail_requested_speed')}</span><strong>{formatCostTierLabel(requestedSpeed)}</strong></div> : null}
              {effectiveSpeed ? <div><span>{t('monitoring.cost_detail_actual_speed')}</span><strong>{formatCostTierLabel(effectiveSpeed)}</strong></div> : null}
              {matchedSpeed ? <div><span>{t('monitoring.cost_detail_matched_speed')}</span><strong>{formatCostTierLabel(matchedSpeed)}</strong></div> : null}
              {requestedSpeed || effectiveSpeed ? <div><span>{t('monitoring.cost_detail_speed_basis')}</span><strong>{breakdown.speedSource === 'response'
                ? t('monitoring.cost_detail_response_authoritative')
                : breakdown.speedSource === 'request_fallback'
                  ? t('monitoring.cost_detail_request_fallback')
                  : breakdown.speedSource === 'none'
                    ? t('monitoring.cost_detail_speed_not_applicable')
                    : t('monitoring.cost_detail_legacy_unknown_mode')}</strong></div> : null}
              <div><span>{t('monitoring.cost_detail_billing_mode')}</span><strong>{billingMode}</strong></div>
            </div>
          ) : (
            <p className={styles.realtimeCostTooltipEmpty}>{t('monitoring.cost_detail_unavailable')}</p>
          )}
        </div>,
        document.body
      ) : null}
    </span>
  );
}
