import { describe, expect, test } from 'bun:test';
import {
  buildZipArchive,
  mapWithConcurrency,
  mapWithKeyedConcurrency,
} from '../src/pro/modules/inspection/features/accountInspectionExport';

const readZipEntryNames = async (archive: Blob) => {
  const bytes = new Uint8Array(await archive.arrayBuffer());
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const endOffset = bytes.length - 22;
  expect(view.getUint32(endOffset, true)).toBe(0x06054b50);
  const count = view.getUint16(endOffset + 10, true);
  let offset = view.getUint32(endOffset + 16, true);
  const decoder = new TextDecoder();
  const names: string[] = [];

  for (let index = 0; index < count; index += 1) {
    expect(view.getUint32(offset, true)).toBe(0x02014b50);
    const nameLength = view.getUint16(offset + 28, true);
    const extraLength = view.getUint16(offset + 30, true);
    const commentLength = view.getUint16(offset + 32, true);
    names.push(decoder.decode(bytes.slice(offset + 46, offset + 46 + nameLength)));
    offset += 46 + nameLength + extraLength + commentLength;
  }

  return names;
};

describe('account inspection export helpers', () => {
  test('keeps mapper output ordered while bounding concurrency', async () => {
    let active = 0;
    let maximumActive = 0;
    const output = await mapWithConcurrency([3, 1, 2, 0], 2, async (value) => {
      active += 1;
      maximumActive = Math.max(maximumActive, active);
      for (let step = 0; step < value; step += 1) await Promise.resolve();
      active -= 1;
      return value * 10;
    });

    expect(output).toEqual([30, 10, 20, 0]);
    expect(maximumActive).toBe(2);
  });

  test('dispatches unrelated providers without keyed head-of-line blocking', async () => {
    const items = [
      ...Array.from({ length: 8 }, (_, index) => ({ id: `xai-${index}`, provider: 'xai' })),
      ...Array.from({ length: 2 }, (_, index) => ({ id: `codex-${index}`, provider: 'codex' })),
    ];
    let releaseGate = () => {};
    const gate = new Promise<void>((resolve) => {
      releaseGate = resolve;
    });
    const started: string[] = [];
    let active = 0;
    let maximumActive = 0;
    const activeByProvider = new Map<string, number>();
    const maximumByProvider = new Map<string, number>();

    const running = mapWithKeyedConcurrency(
      items,
      4,
      1,
      (item) => item.provider,
      async (item) => {
        started.push(item.provider);
        active += 1;
        maximumActive = Math.max(maximumActive, active);
        const providerActive = (activeByProvider.get(item.provider) ?? 0) + 1;
        activeByProvider.set(item.provider, providerActive);
        maximumByProvider.set(item.provider, Math.max(maximumByProvider.get(item.provider) ?? 0, providerActive));
        await gate;
        active -= 1;
        activeByProvider.set(item.provider, providerActive - 1);
        return item.id;
      }
    );

    await Promise.resolve();
    await Promise.resolve();
    const initiallyStarted = [...started];
    releaseGate();
    const output = await running;

    expect(initiallyStarted.sort()).toEqual(['codex', 'xai']);
    expect(maximumActive).toBe(2);
    expect(maximumByProvider).toEqual(new Map([['xai', 1], ['codex', 1]]));
    expect(output).toEqual(items.map((item) => item.id));
  });

  test('sanitizes and de-duplicates exported credential paths', async () => {
    const archive = await buildZipArchive([
      { name: 'team/account', content: '{"id":1}' },
      { name: 'team:account', content: '{"id":2}' },
      { name: 'ready.json', content: '{"id":3}' },
    ]);

    expect(archive.type).toBe('application/zip');
    await expect(readZipEntryNames(archive)).resolves.toEqual([
      'team_account.json',
      'team_account-2.json',
      'ready.json',
    ]);
  });
});
