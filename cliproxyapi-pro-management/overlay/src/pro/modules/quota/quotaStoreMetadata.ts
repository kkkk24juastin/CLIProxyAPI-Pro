export type ProQuotaProviderType =
  | 'antigravity'
  | 'claude'
  | 'codex'
  | 'gemini-cli'
  | 'kimi'
  | 'xai';

type QuotaProviderMetadata = {
  quotaMapName:
    | 'antigravityQuota'
    | 'claudeQuota'
    | 'codexQuota'
    | 'geminiCliQuota'
    | 'kimiQuota'
    | 'xaiQuota';
  setterName:
    | 'setAntigravityQuota'
    | 'setClaudeQuota'
    | 'setCodexQuota'
    | 'setGeminiCliQuota'
    | 'setKimiQuota'
    | 'setXaiQuota';
};

const QUOTA_PROVIDER_METADATA: Record<ProQuotaProviderType, QuotaProviderMetadata> = {
  antigravity: { quotaMapName: 'antigravityQuota', setterName: 'setAntigravityQuota' },
  claude: { quotaMapName: 'claudeQuota', setterName: 'setClaudeQuota' },
  codex: { quotaMapName: 'codexQuota', setterName: 'setCodexQuota' },
  'gemini-cli': { quotaMapName: 'geminiCliQuota', setterName: 'setGeminiCliQuota' },
  kimi: { quotaMapName: 'kimiQuota', setterName: 'setKimiQuota' },
  xai: { quotaMapName: 'xaiQuota', setterName: 'setXaiQuota' },
};

export const PRO_QUOTA_PROVIDER_TYPES = Object.keys(
  QUOTA_PROVIDER_METADATA
) as ProQuotaProviderType[];

export const isProQuotaProviderType = (provider: string): provider is ProQuotaProviderType =>
  provider in QUOTA_PROVIDER_METADATA;

export const getQuotaProviderMapName = (provider: ProQuotaProviderType) =>
  QUOTA_PROVIDER_METADATA[provider].quotaMapName;

export const getQuotaProviderSetterName = (provider: ProQuotaProviderType) =>
  QUOTA_PROVIDER_METADATA[provider].setterName;
