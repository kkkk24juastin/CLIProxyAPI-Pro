import { describe, expect, test } from 'bun:test';
import {
  DEFAULT_MONITORING_PAGE_SIZE,
  MONITORING_PAGE_SIZE_OPTIONS,
  normalizeMonitoringPageSize,
  resolveMonitoringPaginationCopy,
} from '../src/features/monitoring/pagination';

describe('monitoring pagination', () => {
  test('defaults to 20 rows and exposes only the supported sizes', () => {
    expect(DEFAULT_MONITORING_PAGE_SIZE).toBe(20);
    expect(MONITORING_PAGE_SIZE_OPTIONS).toEqual([20, 50, 100]);
  });

  test('normalizes select values to a supported page size', () => {
    expect(normalizeMonitoringPageSize('50')).toBe(50);
    expect(normalizeMonitoringPageSize(100)).toBe(100);
    expect(normalizeMonitoringPageSize('25')).toBe(DEFAULT_MONITORING_PAGE_SIZE);
  });

  test('provides localized fallback copy when runtime locale keys are unavailable', () => {
    expect(resolveMonitoringPaginationCopy('zh-CN').pageSizeLabel).toBe('每页条数');
    expect(resolveMonitoringPaginationCopy('zh-CN').pageSizeValue(20)).toBe('20 条/页');
    expect(resolveMonitoringPaginationCopy('zh-TW').pageSizeValue(50)).toBe('50 筆/頁');
    expect(resolveMonitoringPaginationCopy('en-US').pageSizeValue(100)).toBe('100 / page');
  });
});
