export type MonitoringSettings = {
  retentionDays: number;
  webdav: {
    enabled: boolean;
    intervalMinutes: number;
    retentionDays: number;
    url: string;
    username: string;
    password: string;
  };
  modelPriceSync: {
    enabled: boolean;
    intervalMinutes: number;
  };
};

export type MonitoringSettingsDraft = {
  retentionDays: string;
  webdavEnabled: boolean;
  webdavIntervalMinutes: string;
  webdavRetentionDays: string;
  webdavUrl: string;
  webdavUsername: string;
  webdavPassword: string;
  modelPriceSyncEnabled: boolean;
  modelPriceSyncIntervalMinutes: string;
};

export const createMonitoringSettingsDraft = (
  settings?: MonitoringSettings
): MonitoringSettingsDraft => ({
  retentionDays: String(settings?.retentionDays ?? 0),
  webdavEnabled: settings?.webdav.enabled ?? false,
  webdavIntervalMinutes: String(settings?.webdav.intervalMinutes ?? 1440),
  webdavRetentionDays: String(settings?.webdav.retentionDays ?? 0),
  webdavUrl: settings?.webdav.url ?? '',
  webdavUsername: settings?.webdav.username ?? '',
  webdavPassword: settings?.webdav.password ?? '',
  modelPriceSyncEnabled: settings?.modelPriceSync?.enabled ?? false,
  modelPriceSyncIntervalMinutes: String(settings?.modelPriceSync?.intervalMinutes ?? 1440),
});

const parseNonNegativeInteger = (value: string) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.max(0, Math.trunc(parsed)) : 0;
};

const parseIntegerAtLeast = (value: string, min: number, fallback: number) => {
  const parsed = Number(value.trim());
  if (!value.trim() || !Number.isFinite(parsed)) return fallback;
  const integer = Math.trunc(parsed);
  return integer >= min ? integer : fallback;
};

export const buildMonitoringSettingsFromDraft = (
  draft: MonitoringSettingsDraft
): MonitoringSettings => ({
  retentionDays: parseNonNegativeInteger(draft.retentionDays),
  webdav: {
    enabled: draft.webdavEnabled,
    intervalMinutes: parseIntegerAtLeast(draft.webdavIntervalMinutes, 1, 1440),
    retentionDays: parseNonNegativeInteger(draft.webdavRetentionDays),
    url: draft.webdavUrl.trim(),
    username: draft.webdavUsername.trim(),
    password: draft.webdavPassword,
  },
  modelPriceSync: {
    enabled: draft.modelPriceSyncEnabled,
    intervalMinutes: parseIntegerAtLeast(draft.modelPriceSyncIntervalMinutes, 60, 1440),
  },
});
