export interface MonitoringUsageTarget {
  apiKeyHash: string;
  apiKeyLabel: string;
  profileId?: string;
  profileName?: string;
}

export interface MonitoringUsageLocationState {
  monitoringUsage: MonitoringUsageTarget;
}

export const buildMonitoringUsageLocationState = (
  target: MonitoringUsageTarget,
): MonitoringUsageLocationState => ({
  monitoringUsage: {
    apiKeyHash: target.apiKeyHash.trim(),
    apiKeyLabel: target.apiKeyLabel.trim(),
    ...(target.profileId?.trim() ? { profileId: target.profileId.trim() } : {}),
    ...(target.profileName?.trim() ? { profileName: target.profileName.trim() } : {}),
  },
});

export const readMonitoringUsageLocationState = (
  value: unknown,
): MonitoringUsageTarget | null => {
  if (!value || typeof value !== 'object') return null;
  const state = value as { monitoringUsage?: unknown };
  if (!state.monitoringUsage || typeof state.monitoringUsage !== 'object') return null;
  const target = state.monitoringUsage as Record<string, unknown>;
  const apiKeyHash = typeof target.apiKeyHash === 'string' ? target.apiKeyHash.trim() : '';
  const apiKeyLabel = typeof target.apiKeyLabel === 'string' ? target.apiKeyLabel.trim() : '';
  if (!apiKeyHash) return null;
  const profileId = typeof target.profileId === 'string' ? target.profileId.trim() : '';
  const profileName = typeof target.profileName === 'string' ? target.profileName.trim() : '';
  return {
    apiKeyHash,
    apiKeyLabel,
    ...(profileId ? { profileId } : {}),
    ...(profileName ? { profileName } : {}),
  };
};
