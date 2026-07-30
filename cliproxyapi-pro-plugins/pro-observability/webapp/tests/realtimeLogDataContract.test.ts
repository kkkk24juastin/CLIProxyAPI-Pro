import { expect, test } from 'bun:test';

test('realtime log refreshes the first event after an empty snapshot', async () => {
  const source = await Bun.file(
    new URL('../src/features/monitoring/hooks/useRealtimeLogData.ts', import.meta.url)
  ).text();
  expect(source).toContain('const [hasSnapshot, setHasSnapshot] = useState(false)');
  expect(source).toContain('setHasSnapshot(true)');
  expect(source).toContain('hasSnapshot ? Math.max(latestId - snapshotMaxId, 0) : 0');
  expect(source).not.toContain('snapshotMaxId > 0 ? Math.max(latestId - snapshotMaxId, 0) : 0');
});
