import { describe, expect, test } from 'bun:test';
import {
  buildMonitoringSettingsFromDraft,
  createMonitoringSettingsDraft,
} from '../src/pro/modules/monitoring/features/monitoringSettings';

describe('monitoring settings form model', () => {
  test('uses stable defaults for a new settings form', () => {
    expect(createMonitoringSettingsDraft()).toMatchObject({
      retentionDays: '0',
      webdavEnabled: false,
      webdavIntervalMinutes: '1440',
      modelPriceSyncEnabled: false,
      modelPriceSyncIntervalMinutes: '1440',
    });
  });

  test('normalizes numeric fields and trims connection identity fields', () => {
    const settings = buildMonitoringSettingsFromDraft({
      retentionDays: '-1',
      webdavEnabled: true,
      webdavIntervalMinutes: '0',
      webdavRetentionDays: '7',
      webdavUrl: ' https://example.com/dav ',
      webdavUsername: ' owner ',
      webdavPassword: ' preserve spaces ',
      modelPriceSyncEnabled: true,
      modelPriceSyncIntervalMinutes: '60',
    });

    expect(settings.retentionDays).toBe(0);
    expect(settings.webdav.intervalMinutes).toBe(1440);
    expect(settings.webdav.url).toBe('https://example.com/dav');
    expect(settings.webdav.username).toBe('owner');
    expect(settings.webdav.password).toBe(' preserve spaces ');
    expect(settings.modelPriceSync.intervalMinutes).toBe(60);
  });

  test('keeps integer settings within the backend contract', () => {
    const settings = buildMonitoringSettingsFromDraft({
      retentionDays: '7.9',
      webdavEnabled: false,
      webdavIntervalMinutes: '10.8',
      webdavRetentionDays: '3.2',
      webdavUrl: '',
      webdavUsername: '',
      webdavPassword: '',
      modelPriceSyncEnabled: true,
      modelPriceSyncIntervalMinutes: '1',
    });

    expect(settings.retentionDays).toBe(7);
    expect(settings.webdav.intervalMinutes).toBe(10);
    expect(settings.webdav.retentionDays).toBe(3);
    expect(settings.modelPriceSync.intervalMinutes).toBe(1440);
  });

  test('uses backend defaults when interval inputs are empty', () => {
    const settings = buildMonitoringSettingsFromDraft({
      retentionDays: '0',
      webdavEnabled: true,
      webdavIntervalMinutes: '',
      webdavRetentionDays: '0',
      webdavUrl: 'https://example.com/dav',
      webdavUsername: '',
      webdavPassword: '',
      modelPriceSyncEnabled: true,
      modelPriceSyncIntervalMinutes: '   ',
    });

    expect(settings.webdav.intervalMinutes).toBe(1440);
    expect(settings.modelPriceSync.intervalMinutes).toBe(1440);
  });
});
