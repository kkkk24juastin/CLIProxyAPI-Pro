import { apiClient } from '@/services/api/client';
import type { ApiError } from '@/types';

export type APIKeyPolicyState = 'unconfigured' | 'configured' | 'orphaned' | 'unavailable';

export interface APIKeyModelMapping {
  source: string;
  target: string;
}

export interface APIKeyProfileInput {
  name: string;
  providers: string[];
  models: string[];
  mappings: APIKeyModelMapping[];
}

export interface APIKeyProfile extends APIKeyProfileInput {
  id: string;
  policyId: string;
  createdAtMs: number;
  updatedAtMs: number;
}

export interface APIKeyQuota {
  enabled: boolean;
  requests?: number;
  totalTokens?: number;
  cost?: number;
  period: APIKeyQuotaPeriod;
  epoch: number;
  startedAtMs: number;
  updatedAtMs: number;
  usage: {
    requestsUsed: number;
    totalTokensUsed: number;
    costUsed: number;
    requestsRemaining?: number;
    totalTokensRemaining?: number;
    costRemaining?: number;
    windowStartedAtMs: number;
    windowEndsAtMs?: number;
    exhausted: string[];
  };
}

export type APIKeyQuotaAdmissionState = 'available' | 'disabled' | 'exhausted' | 'blocked';

export interface APIKeyQuotaSummary {
  policyId: string;
  policyVersion: number;
  quota?: APIKeyQuota;
  admissionState: APIKeyQuotaAdmissionState;
  blockedReason?: 'pricing_store_unavailable' | 'settlement_store_unavailable';
  nextRecoverAtMs?: number;
}

export interface APIKeyQuotaSummaryResponse {
  items: APIKeyQuotaSummary[];
  snapshotAtMs: number;
}

export interface APIKeyQuotaInput {
  enabled: boolean;
  requests?: number;
  totalTokens?: number;
  cost?: number;
  period: APIKeyQuotaPeriod;
}

export type APIKeyQuotaPeriod =
  | { type: 'all_time' }
  | { type: 'past_duration'; value: number; unit: 'minute' | 'hour' | 'day' }
  | { type: 'calendar_duration'; unit: 'day' | 'month' };

export interface APIKeyPolicy {
  id: string;
  displayName: string;
  state: APIKeyPolicyState;
  activeProfileId: string;
  profiles: APIKeyProfile[];
  quota?: APIKeyQuota;
  version: number;
  createdAtMs: number;
  updatedAtMs: number;
}

export interface APIKeyPolicyBinding {
  maskedKey: string;
  keyRef: string;
  state: APIKeyPolicyState;
  weakKey: boolean;
  policy?: APIKeyPolicy;
}

export interface APIKeyPolicyBindingPage {
  items: APIKeyPolicyBinding[];
  orphaned: APIKeyPolicy[];
  nextCursor: string;
  configGeneration: number;
}

export interface APIKeyPolicyCatalog {
  providers: string[];
  models: string[];
  modelProviders?: Record<string, string[]>;
}

export interface APIKeyPolicyCapabilities {
  apiVersion: number;
  features: string[];
}

export interface APIKeyPolicyProfileCatalogItem {
  id: string;
  name: string;
  updatedAtMs: number;
}

export interface APIKeyPolicyProfileCatalog {
  items: APIKeyPolicyProfileCatalogItem[];
  policyGeneration: number;
}

export interface APIKeyPolicyStatus {
	takeoverEnabled: boolean;
	healthy: boolean;
	policyGeneration: number;
	configuredGeneration: number;
}

export interface APIKeyPolicyUsageTarget {
  apiKeyHash: string;
  configGeneration: number;
}

export interface APIKeyPolicyDeletePreview {
  policyId: string;
  version: number;
  change: 'restricted_profile_to_unrestricted_passthrough';
  targetPolicyMode: 'passthrough';
  affectsNewRequestsOnly: boolean;
  requiresConfirmation: typeof PASSTHROUGH_CONFIRMATION;
  activeProfile: {
    id: string;
    name: string;
    providers: string[];
    models: string[];
  };
}

export interface APIKeyPolicySnapshot {
  bindings: APIKeyPolicyBindingPage;
  catalog: APIKeyPolicyCatalog;
  capabilities: APIKeyPolicyCapabilities;
}

const REQUIRED_API_KEY_POLICY_FEATURES = [
  'policy_crud',
  'profile_crud',
  'optimistic_concurrency',
  'atomic_workspace_save',
  'policy_backup_restore',
  'policy_delete_preview',
  'orphaned_purge_guard',
  'takeover_control',
] as const;

export class APIKeyPolicyCapabilityError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'APIKeyPolicyCapabilityError';
    (this as ApiError).apiCode = 'api_key_policy_capability_incompatible';
  }
}

const apiKeyPolicyClientError = (apiCode: string, message: string): ApiError => {
  const error = new Error(message) as ApiError;
  error.name = 'APIKeyPolicyClientError';
  error.apiCode = apiCode;
  return error;
};

export const validateAPIKeyPolicyCapabilities = (
  capabilities: APIKeyPolicyCapabilities,
): APIKeyPolicyCapabilities => {
  const features = new Set(Array.isArray(capabilities.features) ? capabilities.features : []);
  if (
    !Number.isInteger(capabilities.apiVersion) ||
    capabilities.apiVersion < 1 ||
    REQUIRED_API_KEY_POLICY_FEATURES.some((feature) => !features.has(feature))
  ) {
    throw new APIKeyPolicyCapabilityError('Core API Key Policy capabilities are below the required contract');
  }
  return capabilities;
};

export const supportsAPIKeyPolicyUsageTarget = (
  capabilities: APIKeyPolicyCapabilities,
): boolean => capabilities.features?.includes('usage_key_target') === true;

const REQUIRED_API_KEY_QUOTA_FEATURES = [
  'key_quota_requests_tokens',
  'key_quota_explicit_reset',
  'key_quota_cost_period',
] as const;

export const supportsAPIKeyQuota = (
  capabilities: APIKeyPolicyCapabilities,
): boolean => {
  const features = new Set(Array.isArray(capabilities.features) ? capabilities.features : []);
  return REQUIRED_API_KEY_QUOTA_FEATURES.every((feature) => features.has(feature));
};

export const supportsAPIKeyQuotaOverview = (
  capabilities: APIKeyPolicyCapabilities,
): boolean => capabilities.features?.includes('key_quota_overview') === true;

export const PASSTHROUGH_CONFIRMATION = 'RESTORE_UNRESTRICTED_PASSTHROUGH';
const API_KEY_POLICY_WRITE_FEATURES = ['provider_model_linkage', 'key_quota_cost_period'] as const;

const policyPath = (policyId: string) => `/api-key-policies/${encodeURIComponent(policyId)}`;
const profilePath = (policyId: string, profileId: string) =>
  `${policyPath(policyId)}/profiles/${encodeURIComponent(profileId)}`;

export const buildAPIKeyPolicyWorkspaceUpdate = (
  displayName: string,
  version: number,
  profileId: string,
  profile: APIKeyProfileInput | undefined,
  createProfile: boolean,
  quota?: APIKeyQuotaInput | null,
) => ({
  displayName,
  version,
  clientFeatures: [...API_KEY_POLICY_WRITE_FEATURES],
  ...(profile ? {
    profileId: createProfile ? '' : profileId,
    profile,
    createProfile,
  } : {}),
  ...(quota !== undefined ? { quota } : {}),
});

const normalizePolicy = (policy: APIKeyPolicy): APIKeyPolicy => ({
  ...policy,
  ...(policy.quota ? {
    quota: {
      ...policy.quota,
      period: policy.quota.period ?? { type: 'all_time' },
      usage: { ...policy.quota.usage },
    },
  } : {}),
  profiles: (policy.profiles ?? []).map((profile) => ({
    ...profile,
    providers: [...(profile.providers ?? [])],
    models: [...(profile.models ?? [])],
    mappings: (profile.mappings ?? []).map((mapping) => ({ ...mapping })),
  })),
});

export const apiKeyPolicyApi = {
  async bindings(): Promise<APIKeyPolicyBindingPage> {
    for (let attempt = 0; attempt < 2; attempt += 1) {
      try {
        const first = await apiClient.get<APIKeyPolicyBindingPage>('/api-key-policy-bindings');
        const orphaned = [...(first.orphaned ?? [])];
        let nextCursor = first.nextCursor;
        let generationChanged = false;
        const seen = new Set<string>();
        while (nextCursor) {
          if (seen.has(nextCursor)) {
            throw apiKeyPolicyClientError(
              'api_key_policy_pagination_cursor_repeated',
              'API Key Policy pagination cursor repeated',
            );
          }
          seen.add(nextCursor);
          const page = await apiClient.get<APIKeyPolicyBindingPage>('/api-key-policy-bindings', {
            params: { orphaned_cursor: nextCursor },
          });
          if (page.configGeneration !== first.configGeneration) {
            generationChanged = true;
            break;
          }
          orphaned.push(...(page.orphaned ?? []));
          nextCursor = page.nextCursor;
        }
        if (generationChanged) continue;
        return {
          ...first,
          items: (first.items ?? []).map((binding) => ({
            ...binding,
            ...(binding.policy ? { policy: normalizePolicy(binding.policy) } : {}),
          })),
          orphaned: orphaned.map(normalizePolicy),
          nextCursor: '',
        };
      } catch (error) {
        const generationChanged = error && typeof error === 'object'
          && (error as ApiError).apiCode === 'api_key_policy_config_changed';
        if (!generationChanged || attempt > 0) throw error;
      }
    }
    throw apiKeyPolicyClientError(
      'api_key_policy_pagination_unstable',
      'API key configuration kept changing during policy pagination',
    );
  },

  usageTarget(keyRef: string): Promise<APIKeyPolicyUsageTarget> {
    return apiClient.post<APIKeyPolicyUsageTarget>('/api-key-policy-usage-target', { keyRef });
  },

  quotaSummaries(): Promise<APIKeyQuotaSummaryResponse> {
    return apiClient.get<APIKeyQuotaSummaryResponse>('/api-key-policy-quota-summaries');
  },

  profileCatalog(): Promise<APIKeyPolicyProfileCatalog> {
    return apiClient.get<APIKeyPolicyProfileCatalog>('/api-key-policy-profile-catalog');
  },

  async snapshot(): Promise<APIKeyPolicySnapshot> {
    const capabilities = validateAPIKeyPolicyCapabilities(
      await apiClient.get<APIKeyPolicyCapabilities>('/api-key-policy-capabilities'),
    );
	const [bindings, catalog] = await Promise.all([
      this.bindings(),
      apiClient.get<APIKeyPolicyCatalog>('/api-key-policy-catalog'),
    ]);
	return { capabilities, bindings, catalog };
  },

	status(): Promise<APIKeyPolicyStatus> {
		return apiClient.get<APIKeyPolicyStatus>('/api-key-policy-status');
	},

	async setTakeover(enabled: boolean, status?: APIKeyPolicyStatus): Promise<APIKeyPolicyStatus> {
		return apiClient.put<APIKeyPolicyStatus>('/api-key-policy-takeover', {
			enabled,
			...(enabled && status ? {
				policyGeneration: status.policyGeneration,
				configuredGeneration: status.configuredGeneration,
			} : {}),
		});
	},

  async get(policyId: string): Promise<APIKeyPolicy> {
    return normalizePolicy(await apiClient.get<APIKeyPolicy>(policyPath(policyId)));
  },

  async create(keyRef: string, displayName: string, initialProfile: APIKeyProfileInput, quota?: APIKeyQuotaInput | null): Promise<APIKeyPolicy> {
    return normalizePolicy(await apiClient.post<APIKeyPolicy>('/api-key-policies', {
      keyRef,
      displayName,
      initialProfile,
      clientFeatures: [...API_KEY_POLICY_WRITE_FEATURES],
      ...(quota !== undefined ? { quota } : {}),
    }));
  },

  async rename(policyId: string, displayName: string, version: number): Promise<APIKeyPolicy> {
    return normalizePolicy(await apiClient.patch<APIKeyPolicy>(policyPath(policyId), { displayName, version }));
  },

  updateWorkspace(
    policyId: string,
    displayName: string,
    version: number,
    profileId: string,
    profile: APIKeyProfileInput | undefined,
    createProfile: boolean,
    quota?: APIKeyQuotaInput | null,
  ): Promise<APIKeyPolicy> {
    return apiClient.patch<APIKeyPolicy>(policyPath(policyId), buildAPIKeyPolicyWorkspaceUpdate(
      displayName,
      version,
      profileId,
      profile,
      createProfile,
      quota,
    )).then(normalizePolicy);
  },

  createProfile(policyId: string, profile: APIKeyProfileInput, version: number): Promise<APIKeyPolicy> {
    return apiClient.post<APIKeyPolicy>(`${policyPath(policyId)}/profiles`, {
      ...profile,
      version,
      clientFeatures: [...API_KEY_POLICY_WRITE_FEATURES],
    }).then(normalizePolicy);
  },

  replaceProfile(
    policyId: string,
    profileId: string,
    profile: APIKeyProfileInput,
    version: number,
  ): Promise<APIKeyPolicy> {
    return apiClient.put<APIKeyPolicy>(profilePath(policyId, profileId), {
      ...profile,
      version,
      clientFeatures: [...API_KEY_POLICY_WRITE_FEATURES],
    }).then(normalizePolicy);
  },

  async deleteProfile(policyId: string, profileId: string, version: number): Promise<void> {
    await apiClient.delete(profilePath(policyId, profileId), { data: { version } });
  },

  activate(policyId: string, profileId: string, version: number): Promise<APIKeyPolicy> {
    return apiClient.put<APIKeyPolicy>(`${policyPath(policyId)}/active-profile`, {
      profileId,
      version,
    }).then(normalizePolicy);
  },

  async deletePolicy(policyId: string, version: number): Promise<void> {
    await apiClient.delete(policyPath(policyId), {
      data: { version, confirmPassthrough: PASSTHROUGH_CONFIRMATION },
    });
  },

  deletePreview(policyId: string): Promise<APIKeyPolicyDeletePreview> {
    return apiClient.get<APIKeyPolicyDeletePreview>(`${policyPath(policyId)}/delete-preview`);
  },

  async purgeOrphaned(policyId: string, version: number, configGeneration: number): Promise<void> {
    await apiClient.delete(`/orphaned-api-key-policies/${encodeURIComponent(policyId)}`, {
      data: { version, configGeneration },
    });
  },

  resetQuota(policyId: string, version: number): Promise<APIKeyPolicy> {
    return apiClient.post<APIKeyPolicy>(`${policyPath(policyId)}/quota/reset`, {
      version,
      confirmReset: 'RESET_API_KEY_QUOTA',
    }).then(normalizePolicy);
  },
};

export const apiKeyPolicyErrorCode = (error: unknown): string =>
  error && typeof error === 'object' && typeof (error as ApiError).apiCode === 'string'
    ? (error as ApiError).apiCode ?? ''
    : '';

export const apiKeyPolicyErrorTranslationKey = (error: unknown): string => {
  const code = apiKeyPolicyErrorCode(error);
  return code ? `api_key_policy.error.${code}` : '';
};

export const isAPIKeyPolicyUnsupported = (error: unknown): boolean =>
  error instanceof APIKeyPolicyCapabilityError ||
  Boolean(error && typeof error === 'object' && (error as ApiError).status === 404);

export const cloneProfileInput = (profile: APIKeyProfileInput): APIKeyProfileInput => ({
  name: profile.name,
  providers: [...(profile.providers ?? [])],
  models: [...(profile.models ?? [])],
  mappings: (profile.mappings ?? []).map((mapping) => ({ ...mapping })),
});

export const updateProfileProviders = (
  profile: APIKeyProfileInput,
  providers: string[],
): APIKeyProfileInput => ({
  ...profile,
  providers: [...providers],
});

export const resolveMappingTargetModels = (
  selectedModels: string[],
  availableModels: string[],
): string[] => {
  if (selectedModels.length === 0) return [...availableModels];
  const available = new Set(availableModels);
  return selectedModels.filter((model) => available.has(model));
};

export const resolveModelsForProviders = (
  selectedProviders: string[],
  catalog: APIKeyPolicyCatalog,
): string[] => {
  if (
    selectedProviders.length === 0 ||
    !catalog.modelProviders ||
    Object.keys(catalog.modelProviders).length === 0
  ) return [...catalog.models];
  const providers = new Set(selectedProviders.map((provider) => provider.toLowerCase()));
  return catalog.models.filter((model) =>
    (catalog.modelProviders?.[model] ?? []).some((provider) =>
      providers.has(provider.toLowerCase()),
    ),
  );
};

export const validateProfileInput = (
  profile: APIKeyProfileInput,
  catalog: APIKeyPolicyCatalog,
): string | null => {
  if (!profile.name.trim()) return 'name';
  const providers = new Set(catalog.providers);
  const models = new Set(catalog.models);
  const providerModels = new Set(resolveModelsForProviders(profile.providers, catalog));
  if (profile.providers.some((provider) => !providers.has(provider))) return 'providers';
  if (profile.models.some((model) => !models.has(model) || !providerModels.has(model))) return 'models';
  const sources = new Set<string>();
  for (const mapping of profile.mappings) {
    const source = mapping.source.trim();
    const target = mapping.target.trim();
    const targetAllowed = profile.models.length === 0
      ? models.has(target) && providerModels.has(target)
      : profile.models.includes(target);
    if (!source || !target || sources.has(source) || !targetAllowed) return 'mappings';
    sources.add(source);
  }
  return null;
};
