import type { RankingMetric } from './monitoringAnalytics';

export const RANKING_METRIC_OPTIONS: Array<{ value: RankingMetric; labelKey: string }> = [
  { value: 'requests', labelKey: 'monitoring.ranking_metric_requests' },
  { value: 'tokens', labelKey: 'monitoring.ranking_metric_tokens' },
  { value: 'cost', labelKey: 'monitoring.ranking_metric_cost' },
];
