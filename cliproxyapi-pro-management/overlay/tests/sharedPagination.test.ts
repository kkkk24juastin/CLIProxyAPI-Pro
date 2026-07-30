import { describe, expect, test } from 'bun:test';
import {
  DEFAULT_MONITORING_PAGE_SIZE,
  MONITORING_PAGE_SIZE_OPTIONS,
  normalizeMonitoringPageSize,
} from '../src/features/shared/pagination';

describe('shared account inspection pagination', () => {
  test('keeps the established page sizes after monitoring moves to the plugin', () => {
    expect(DEFAULT_MONITORING_PAGE_SIZE).toBe(20);
    expect(MONITORING_PAGE_SIZE_OPTIONS).toEqual([20, 50, 100]);
    expect(normalizeMonitoringPageSize('50')).toBe(50);
  });

  test('keeps expanded inspection results inside a scrollable viewport', async () => {
    const styles = await Bun.file(
      new URL('../src/features/monitoring/account-inspection-styles/_tables-dialogs.scss', import.meta.url)
    ).text();
    expect(styles).toContain('.resultsTableViewport');
    expect(styles).toContain('max-height: min(620px, 68vh)');
    expect(styles).toContain('overflow-y: auto');
    expect(styles).toContain('scrollbar-gutter: stable');
  });
});
