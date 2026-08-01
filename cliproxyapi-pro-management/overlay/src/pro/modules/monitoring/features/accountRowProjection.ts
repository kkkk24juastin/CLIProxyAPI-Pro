import type {
  MonitoringAccountGroupBy,
  MonitoringAccountModelSpendRow,
  MonitoringAccountRow,
  MonitoringEventRow,
} from './hooks/useMonitoringData';

export type MonitoringAccountRowAccumulator = {
  authLabels: Set<string>;
  authIndices: Set<string>;
  channels: Set<string>;
  providers: Set<string>;
  totalCalls: number;
  successCalls: number;
  failureCalls: number;
  inputTokens: number;
  outputTokens: number;
  cachedTokens: number;
  totalTokens: number;
  totalCost: number;
  latencySum: number;
  latencyCount: number;
  lastSeenAt: number;
};

type MonitoringAccountRowIdentity = {
  id: string;
  group: MonitoringAccountGroupBy;
  model: string;
  apiKeyHash: string;
  apiKeyMasked: string;
  account: string;
  accountMasked: string;
};

type MonitoringAccountRowDetails = {
  recentPattern?: boolean[];
  rows?: MonitoringEventRow[];
  models?: MonitoringAccountModelSpendRow[];
};

export const projectMonitoringAccountRow = (
  item: MonitoringAccountRowAccumulator,
  identity: MonitoringAccountRowIdentity,
  details: MonitoringAccountRowDetails = {}
): MonitoringAccountRow => ({
  ...identity,
  authLabels: Array.from(item.authLabels).sort(),
  authIndices: Array.from(item.authIndices).sort(),
  channels: Array.from(item.channels).sort(),
  providers: Array.from(item.providers).sort(),
  totalCalls: item.totalCalls,
  successCalls: item.successCalls,
  failureCalls: item.failureCalls,
  successRate: item.totalCalls > 0 ? item.successCalls / item.totalCalls : 1,
  inputTokens: item.inputTokens,
  outputTokens: item.outputTokens,
  cachedTokens: item.cachedTokens,
  totalTokens: item.totalTokens,
  totalCost: item.totalCost,
  averageLatencyMs: item.latencyCount > 0 ? item.latencySum / item.latencyCount : null,
  lastSeenAt: item.lastSeenAt,
  recentPattern: details.recentPattern ?? [],
  rows: details.rows,
  models: details.models ?? [],
});
