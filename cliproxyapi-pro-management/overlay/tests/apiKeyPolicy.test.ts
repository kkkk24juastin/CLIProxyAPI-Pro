import { describe, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  APIKeyPolicyCapabilityError,
  buildAPIKeyPolicyWorkspaceUpdate,
  cloneProfileInput,
  apiKeyPolicyErrorTranslationKey,
  validateAPIKeyPolicyCapabilities,
  validateProfileInput,
  type APIKeyPolicyCatalog,
  type APIKeyProfileInput,
} from '@/pro/modules/apiKeyPolicy';

const catalog: APIKeyPolicyCatalog = {
  providers: ['openai', 'claude', 'home'],
  models: ['gpt-5', 'claude-sonnet-4'],
};

const validProfile = (): APIKeyProfileInput => ({
  name: 'Production',
  providers: ['openai'],
  models: ['gpt-5'],
  mappings: [{ source: 'smart', target: 'gpt-5' }],
});

describe('usage policy backup preview contract', () => {
  test('previews replacement, orphaned state, and config key boundary before importing', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/monitoring/MonitoringCenterPage.tsx'), 'utf8');
    expect(page).toContain("'/usage/import/preview'");
    expect(page).toContain('import_policy_preview_replace');
    expect(page).toContain('import_policy_preview_preserve');
    expect(page).toContain('import_policy_no_api_keys');
    expect(page).toContain('onConfirm: () => executeUsageImport(content, allowLegacy)');
  });

  test('lists WebDAV backups and reuses the preview and restore endpoints', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/monitoring/MonitoringCenterPage.tsx'), 'utf8');
    const dialog = readFileSync(resolve(import.meta.dir, '../src/pro/modules/monitoring/features/components/WebDAVRestoreDialog.tsx'), 'utf8');
    expect(page).toContain("'/usage/webdav/backups'");
    expect(page).toContain("'/usage/webdav/preview'");
    expect(page).toContain("'/usage/webdav/restore'");
    expect(page).toContain('buildPolicyBackupSummary(preview.policyBackup ?? {}, t)');
    expect(page).toContain('onConfirm: () => executeWebDAVRestore(backup, allowLegacy)');
    expect(dialog).toContain('import_policy_no_api_keys');
    expect(dialog).toContain('backup.fileName');
  });
});

describe('API Key Policy profile drafts', () => {
  test('requires the explicit minimum Core capability contract', () => {
    expect(validateAPIKeyPolicyCapabilities({
		apiVersion: 2,
		features: ['policy_crud', 'profile_crud', 'optimistic_concurrency', 'atomic_workspace_save', 'policy_backup_restore', 'policy_delete_preview', 'orphaned_purge_guard', 'takeover_control'],
		}).apiVersion).toBe(2);
    expect(() => validateAPIKeyPolicyCapabilities({
      apiVersion: 1,
      features: ['policy_crud', 'profile_crud', 'optimistic_concurrency'],
    })).toThrow(APIKeyPolicyCapabilityError);
  });

  test('rejects Core that omits only policy backup and restore support', () => {
    expect(() => validateAPIKeyPolicyCapabilities({
      apiVersion: 1,
		features: ['policy_crud', 'profile_crud', 'optimistic_concurrency', 'atomic_workspace_save', 'policy_delete_preview', 'orphaned_purge_guard', 'takeover_control'],
    })).toThrow(APIKeyPolicyCapabilityError);
  });

  test('rejects Core that cannot provide a server-derived delete preview', () => {
    expect(() => validateAPIKeyPolicyCapabilities({
      apiVersion: 1,
		features: ['policy_crud', 'profile_crud', 'optimistic_concurrency', 'atomic_workspace_save', 'policy_backup_restore', 'orphaned_purge_guard', 'takeover_control'],
    })).toThrow(APIKeyPolicyCapabilityError);
  });

  test('rejects Core that cannot atomically guard orphaned-policy purge', () => {
    expect(() => validateAPIKeyPolicyCapabilities({
      apiVersion: 1,
		features: ['policy_crud', 'profile_crud', 'optimistic_concurrency', 'atomic_workspace_save', 'policy_backup_restore', 'policy_delete_preview', 'takeover_control'],
    })).toThrow(APIKeyPolicyCapabilityError);
  });

  test('localizes API errors and keeps orphan purge bound to version and config generation', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
    const client = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/apiKeyPolicy.ts'), 'utf8');
    expect(apiKeyPolicyErrorTranslationKey({ apiCode: 'api_key_policy_orphaned' })).toBe('api_key_policy.error.api_key_policy_orphaned');
    expect(page).toContain('apiKeyPolicyApi.purgeOrphaned(');
    expect(page).toContain('snapshot.bindings.configGeneration');
    expect(client).toContain('data: { version, configGeneration }');
    expect(client).toContain("'orphaned_purge_guard'");
  });

  test('has complete distinct translations for every supported language', async () => {
    const locales = await import('../src/pro/apiKeyPolicyLocales');
    const bundles = locales.apiKeyPolicyLocales as Record<string, { api_key_policy: Record<string, unknown> }>;
    expect(Object.keys(bundles).sort()).toEqual(['en', 'ru', 'zh-CN', 'zh-TW']);
    const flatten = (value: unknown, prefix = ''): Record<string, string> => {
      if (!value || typeof value !== 'object') return {};
      return Object.entries(value as Record<string, unknown>).reduce<Record<string, string>>((out, [key, child]) => {
        const path = prefix ? `${prefix}.${key}` : key;
        if (typeof child === 'string') out[path] = child;
        else Object.assign(out, flatten(child, path));
        return out;
      }, {});
    };
    const english = flatten(bundles.en);
    for (const language of ['ru', 'zh-CN', 'zh-TW']) {
      const translated = flatten(bundles[language]);
      expect(Object.keys(translated).sort()).toEqual(Object.keys(english).sort());
      for (const key of Object.keys(english)) expect(translated[key]).not.toBe(english[key]);
    }
    expect(bundles['zh-CN'].api_key_policy.delete_policy_preview).not.toContain('unrestricted passthrough');
  });

  test('uses one atomic workspace request and synchronous duplicate-save guard', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
    const client = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/apiKeyPolicy.ts'), 'utf8');
    expect(page).toContain('apiKeyPolicyApi.updateWorkspace(');
    expect(page).not.toContain('policy = await apiKeyPolicyApi.rename');
    expect(page).toContain('savingRef.current = true');
    expect(page).toContain('if (!workspaceTarget || !draft || !validateDraft() || savingRef.current) return;');
    expect(client).toContain("'/api-key-policy-capabilities'");
    expect(client).toContain('buildAPIKeyPolicyWorkspaceUpdate(');
    expect(buildAPIKeyPolicyWorkspaceUpdate('Renamed', 2, 'profile-1', undefined, false)).toEqual({
      displayName: 'Renamed',
      version: 2,
    });
    expect(buildAPIKeyPolicyWorkspaceUpdate('Renamed', 2, 'profile-1', validProfile(), false)).toEqual({
      displayName: 'Renamed',
      version: 2,
      profileId: 'profile-1',
      profile: validProfile(),
      createProfile: false,
    });
  });

	test('uses the standard 720px Workspace Sheet with fixed save and cancel actions', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
    const styles = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.module.scss'), 'utf8');
		expect(page).toContain('className={styles.policySheet}');
		expect(page).toContain('footer={');
		expect(page).toContain('disabled={!dirty || saving}');
		expect(page).not.toContain('workspaceActionBar');
		expect(styles).toContain('width: min(720px, 100vw) !important;');
  });

	test('loads and updates the explicit takeover contract', () => {
		const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
		const client = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/apiKeyPolicy.ts'), 'utf8');
		expect(client).toContain("'takeover_control'");
		expect(client).toContain("'/api-key-policy-status'");
		expect(client).toContain("'/api-key-policy-takeover'");
		expect(page).toContain('active={snapshot?.takeoverEnabled === true}');
		expect(page).toContain('apiKeyPolicyApi.setTakeover(enabled)');
	});

  test('keeps the draft on 409 and does not replace it when server state is reloaded for manual merge', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
    expect(page).toContain("apiKeyPolicyErrorCode(error) === 'config_version_conflict'");
    expect(page).toContain('setConflict(true);');
    const reload = page.slice(page.indexOf('const reloadWorkspace'), page.indexOf('const validateDraft'));
    expect(reload).toContain('setWorkspaceTarget(target);');
    expect(reload).not.toContain('setDraft(');
  });

  test('invalidates delayed activate and danger responses and resets synchronous busy guards', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
    const activate = page.slice(page.indexOf('const activateProfile'), page.indexOf('const runDangerAction'));
    expect(activate).toContain('const revision = ++saveRevisionRef.current;');
    expect(activate).toContain('if (revision !== saveRevisionRef.current) return;');
    const danger = page.slice(page.indexOf('const runDangerAction'), page.indexOf('const visibleItems'));
    expect(danger).toContain('if (!dangerPolicy || !dangerKind || dangerBusyRef.current) return;');
    expect(danger).toContain('const revision = ++dangerRevisionRef.current;');
    expect(danger).toContain('if (revision !== dangerRevisionRef.current) return;');
    expect(page).toContain('setSaving(false);');
  });

  test('reload invalidates an in-flight save without replacing edits made after submit', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
    const reload = page.slice(page.indexOf('const reloadWorkspace'), page.indexOf('const validateDraft'));
    const save = page.slice(page.indexOf('const saveWorkspace'), page.indexOf('const activateProfile'));
    expect(reload).toContain('const revision = ++saveRevisionRef.current;');
    expect(reload).toContain('savingRef.current = true;');
    expect(save).toContain('const submittedDraftRevision = draftRevisionRef.current;');
    expect(save).toContain('if (revision !== saveRevisionRef.current) return;');
    expect(save).toContain('if (submittedDraftRevision === draftRevisionRef.current)');
  });

  test('closes a conflicting danger dialog and leaves the action reopenable', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/APIKeyPolicyPage.tsx'), 'utf8');
    const danger = page.slice(page.indexOf('const runDangerAction'), page.indexOf('const visibleItems'));
    expect(danger).toContain("apiKeyPolicyErrorCode(error) === 'config_version_conflict'");
    expect(danger).toContain('setDangerPolicy(null);');
    expect(danger).toContain('setDangerKind(null);');
    expect(danger).toContain('dangerBusyRef.current = false;');
    expect(page).toContain('onClick={() => void openPolicyDeletePreview(currentPolicy)}');
    expect(page).toContain('apiKeyPolicyApi.deletePreview(policy.id)');
    expect(page).toContain('preview.version !== policy.version');
  });

  test('restarts orphan pagination on config-generation drift and never merges mixed pages', () => {
    const client = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/apiKeyPolicy.ts'), 'utf8');
    expect(client).toContain('page.configGeneration !== first.configGeneration');
    expect(client).toContain("(error as ApiError).apiCode === 'api_key_policy_config_changed'");
    expect(client).toContain('for (let attempt = 0; attempt < 2; attempt += 1)');
  });

  test('requires the complete policy-filter scope before reusing failed aggregate results', () => {
    const page = readFileSync(resolve(import.meta.dir, '../src/pro/modules/monitoring/MonitoringCenterPage.tsx'), 'utf8');
    const hook = readFileSync(resolve(import.meta.dir, '../src/pro/modules/monitoring/features/hooks/useUsageAggregates.ts'), 'utf8');
    expect(page).toContain("usageAggregates.scopeAPIKeyPolicyId === (selectedAPIKeyPolicy === 'all' ? '' : selectedAPIKeyPolicy)");
    expect(page).toContain("usageAggregates.scopeProfileId === (selectedProfile === 'all' ? '' : selectedProfile)");
    expect(page).toContain("usageAggregates.scopePolicyMode === (selectedPolicyMode === 'all' ? '' : selectedPolicyMode)");
    expect(page).toContain('if (!serverUsageTrendAnalytics || !aggregateTrendScopeMatches)');
    expect(hook).toContain('hasDataRef.current = false;');
    expect(hook).toContain('setData(null);');
  });

  test('accepts only server-catalog providers, models, and allowed mapping targets', () => {
    expect(validateProfileInput(validProfile(), catalog)).toBeNull();
    expect(validateProfileInput({ ...validProfile(), providers: ['unknown'] }, catalog)).toBe('providers');
    expect(validateProfileInput({ ...validProfile(), models: ['unknown'] }, catalog)).toBe('models');
    expect(validateProfileInput({ ...validProfile(), mappings: [{ source: 'smart', target: 'claude-sonnet-4' }] }, catalog)).toBe('mappings');
  });

  test('rejects duplicate or partial mapping sources', () => {
    expect(validateProfileInput({
      ...validProfile(),
      mappings: [
        { source: 'smart', target: 'gpt-5' },
        { source: 'smart', target: 'gpt-5' },
      ],
    }, catalog)).toBe('mappings');
    expect(validateProfileInput({ ...validProfile(), mappings: [{ source: '', target: 'gpt-5' }] }, catalog)).toBe('mappings');
  });

  test('clones mappings and selection arrays so server state cannot overwrite a draft', () => {
    const original = validProfile();
    const draft = cloneProfileInput(original);
    draft.providers.push('home');
    draft.models.push('claude-sonnet-4');
    draft.mappings[0].target = 'claude-sonnet-4';
    expect(original).toEqual(validProfile());
  });

  test('normalizes legacy null mapping collections at the API boundary', () => {
    const legacy = { ...validProfile(), mappings: null } as unknown as APIKeyProfileInput;
    expect(cloneProfileInput(legacy).mappings).toEqual([]);
    const client = readFileSync(resolve(import.meta.dir, '../src/pro/modules/apiKeyPolicy/apiKeyPolicy.ts'), 'utf8');
    expect(client).toContain('mappings: (profile.mappings ?? []).map');
    expect(client).toContain('policy: normalizePolicy(binding.policy)');
  });
});
