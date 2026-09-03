import { describe, expect, test } from 'bun:test';
import { resolveCodexChatgptAccountId } from '@/utils/quota';

describe('Codex account ID resolver', () => {
  test('falls through empty aliases to a valid direct account ID', () => {
    expect(
      resolveCodexChatgptAccountId({
        name: 'codex-test.json',
        chatgpt_account_id: '  ',
        account_id: 'acct-fallback',
      })
    ).toBe('acct-fallback');
  });

  test('falls through invalid metadata aliases before parsing the ID token', () => {
    expect(
      resolveCodexChatgptAccountId({
        name: 'codex-test.json',
        metadata: {
          chatgpt_account_id: {},
          accountId: 'acct-metadata',
        },
      })
    ).toBe('acct-metadata');
  });
});
