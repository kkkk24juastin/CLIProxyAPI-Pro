export const MONITORING_PAGE_SIZE_OPTIONS = [20, 50, 100] as const;

export type MonitoringPageSize = (typeof MONITORING_PAGE_SIZE_OPTIONS)[number];

export const DEFAULT_MONITORING_PAGE_SIZE: MonitoringPageSize = 20;

export const normalizeMonitoringPageSize = (value: string | number): MonitoringPageSize => {
  const parsed = typeof value === 'number' ? value : Number.parseInt(value, 10);
  return MONITORING_PAGE_SIZE_OPTIONS.find((pageSize) => pageSize === parsed)
    ?? DEFAULT_MONITORING_PAGE_SIZE;
};
