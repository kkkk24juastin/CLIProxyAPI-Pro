import { describe, expect, test } from 'bun:test';

describe('account inspection result viewport', () => {
  test('keeps expanded results inside a stable scrollable viewport', async () => {
    const styles = await Bun.file(
      new URL('../src/features/monitoring/account-inspection-styles/_tables-dialogs.scss', import.meta.url)
    ).text();
    expect(styles).toContain('.resultsTableViewport');
    expect(styles).toContain('max-height: min(620px, 68vh)');
    expect(styles).toContain('overflow-y: auto');
    expect(styles).toContain('scrollbar-gutter: stable');
  });
});
