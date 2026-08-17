import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

describe('data-management destructive-operation fencing', () => {
  test('executes cleanup against the previewed domains, cutoff, and record counts', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/dataManagement/DataManagementPage.tsx'), 'utf8');

    expect(page).toContain('domains: cleanupPreview.domains.map((domain) => domain.id)');
    expect(page).toContain('beforeMs: cleanupPreview.cutoffMs');
    expect(page).toContain('expectedRecords: Object.fromEntries(cleanupPreview.domains.map((domain) => [domain.id, domain.records]))');
  });

  test('saves only visible settings sections with an optimistic snapshot', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/dataManagement/DataManagementPage.tsx'), 'utf8');

    expect(page).toContain("sections.push('retention')");
    expect(page).toContain("sections.push('webdav')");
    expect(page).toContain('dataManagementApi.saveSettings(settings, expectedSettings, sections)');
    expect(page).not.toContain("sections.push('modelPriceSync')");
  });

  test('model-price settings save cannot overwrite data-management sections', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/monitoring/MonitoringCenterPage.tsx'), 'utf8');

    expect(page).toContain("sections: ['modelPriceSync']");
    expect(page).toContain('expectedSettings,');
  });

  test('sends encrypted-export passphrases in a POST body', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/dataManagement/DataManagementPage.tsx'), 'utf8');

    expect(page).toContain("apiClient.post<Blob>('/data/backups/export', { passphrase }");
    expect(page).not.toContain("'X-CLIProxy-Backup-Passphrase': passphrase");
  });
});
