import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  buildProfileFilterOptions,
  resolveUsageProfileSnapshot,
} from '@/pro/modules/monitoring/features/profileUsage';
import {
  buildMonitoringUsageLocationState,
  readMonitoringUsageLocationState,
} from '@/pro/shared/monitoringNavigation';
import { resolveAPIKeyUsageHash } from '@/pro/modules/apiKeyPolicy/usageKey';

const copy = {
  allProfiles: 'All profiles',
  renamed: (name: string, previous: string) => `${name} (previously: ${previous})`,
  deleted: (name: string) => `${name} (deleted)`,
};

describe('monitoring API key and Profile usage navigation', () => {
  test('keeps the API key hash in route state instead of the URL', () => {
    const state = buildMonitoringUsageLocationState({
      apiKeyHash: 'hash-1',
      apiKeyLabel: 'sk-****-01',
      profileId: 'profile-1',
      profileName: 'Current',
    });
    expect(readMonitoringUsageLocationState(state)).toEqual({
      apiKeyHash: 'hash-1',
      apiKeyLabel: 'sk-****-01',
      profileId: 'profile-1',
      profileName: 'Current',
    });
    expect(JSON.stringify(state)).not.toContain('api_key_policy_id');
  });

  test('derives the selected usage hash from the already loaded upstream Key configuration', async () => {
    const hash = await resolveAPIKeyUsageHash({
      configuredKeys: ['sk-first-abcdefgh', 'sk-second-ijklmnop'],
      bindingIndex: 1,
      maskedKey: 'sk******op',
    });
    expect(hash).toMatch(/^[a-f0-9]{64}$/);
    await expect(resolveAPIKeyUsageHash({
      configuredKeys: ['aa-collision-zz', 'aa-different-zz'],
      bindingIndex: -1,
      maskedKey: 'aa******zz',
    })).rejects.toThrow();
  });

  test('filters a renamed Profile by stable ID while exposing name history', () => {
    const options = buildProfileFilterOptions({
      observations: [
        { profileId: 'profile-1', profileName: 'Old', timestampMs: 100 },
        { profileId: 'profile-1', profileName: 'New', timestampMs: 200 },
      ],
      currentNames: new Map([['profile-1', 'Current']]),
      currentNamesLoaded: true,
      selectedProfileId: 'profile-1',
      selectedProfileName: '',
      copy,
    });
    expect(options).toEqual([
      { value: 'all', label: 'All profiles' },
      { value: 'profile-1', label: 'Current (previously: New, Old)' },
    ]);
  });

  test('keeps historical snapshot names immutable and labels deleted Profiles', () => {
    expect(resolveUsageProfileSnapshot('Old name', 'profile-1', 'No profile')).toBe('Old name');
    expect(resolveUsageProfileSnapshot('', 'profile-1', 'No profile')).toBe('profile-1');
    expect(resolveUsageProfileSnapshot('', '', 'No profile')).toBe('No profile');
    expect(buildProfileFilterOptions({
      observations: [{ profileId: 'deleted-profile', profileName: 'Archived', timestampMs: 100 }],
      currentNames: new Map(),
      currentNamesLoaded: true,
      selectedProfileId: 'all',
      selectedProfileName: '',
      copy,
    })).toContainEqual({ value: 'deleted-profile', label: 'Archived (deleted)' });
  });

  test('removes policy and policy-mode filters and renders Profile under the API key', () => {
    const policyPage = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
    const monitoringPage = readFileSync(resolve(import.meta.dir, '../src/pro/modules/monitoring/MonitoringCenterPage.tsx'), 'utf8');
    expect(policyPage).toContain("navigate('/monitoring#request-events'");
    expect(policyPage).toContain('resolveAPIKeyUsageHash({');
    expect(policyPage).toContain('apiKeyHash,');
    expect(policyPage).not.toContain('/monitoring?api_key_policy_id=');
    expect(monitoringPage).not.toContain('selectedAPIKeyPolicy');
    expect(monitoringPage).not.toContain('selectedPolicyMode');
    expect(monitoringPage).toContain("t('monitoring.api_key_profile'");
    expect(monitoringPage).toContain('profileId: selectedProfile');
  });
});
