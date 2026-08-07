import { apiClient } from "@/services/api/client";
import { parsePositiveGoDuration, serializeGoDuration } from '@/pro/shared/duration';

const OAUTH_MODEL_PROVIDER_KEYS = [
  "xai",
  "codex",
  "claude",
  "gemini-cli",
  "antigravity",
  "kimi",
] as const;

export type OAuthModelProviderKey = (typeof OAUTH_MODEL_PROVIDER_KEYS)[number];
export type OAuthModelPlanKey = string;
export type OAuthPolicyDurationUnit = "s" | "m";

export interface OAuthModelPlanDefinition {
  key: OAuthModelPlanKey;
  monthlyLimitCents?: number;
  kind: "plan" | "fallback" | "custom";
  localeSuffix: string;
}

export interface OAuthModelProviderDefinition {
  key: OAuthModelProviderKey;
  plans: OAuthModelPlanDefinition[];
}

export interface OAuthModelPlanRule {
  configured: boolean;
  excludedModels: string[];
  prefix?: string;
  priority?: number;
  weight?: number;
}

export interface OAuthPolicyEffectiveItem {
  authId: string;
  provider: string;
  planKey: string;
  planSource: string;
  matchedRule: string;
  prefix?: string;
  priority?: number;
  weight?: number;
  excludedModelCount: number;
  planError?: string;
}

export interface OAuthPolicyConfig {
  enabled: boolean;
  cacheTTL: string;
  resolveTimeout: string;
  providers: Record<
    string,
    { plans: Record<OAuthModelPlanKey, OAuthModelPlanRule> }
  >;
}

export interface OAuthPolicySnapshot {
  config: OAuthPolicyConfig;
  status: {
    enabled: boolean;
    cacheTTL: string;
    resolveTimeout: string;
    providers: number;
    lastError?: string;
  };
  effective: OAuthPolicyEffectiveItem[];
}

const fallbackPlans: OAuthModelPlanDefinition[] = [
  { key: "_unknown", kind: "fallback", localeSuffix: "unknown" },
  { key: "_default", kind: "fallback", localeSuffix: "default" },
];

export const OAUTH_MODEL_PROVIDER_DEFINITIONS: OAuthModelProviderDefinition[] =
  [
    {
      key: "xai",
      plans: [
        {
          key: "free",
          monthlyLimitCents: 0,
          kind: "plan",
          localeSuffix: "free",
        },
        {
          key: "supergrok",
          monthlyLimitCents: 15_000,
          kind: "plan",
          localeSuffix: "supergrok",
        },
        {
          key: "supergrok-heavy",
          monthlyLimitCents: 150_000,
          kind: "plan",
          localeSuffix: "supergrok_heavy",
        },
        { key: "paid-unknown", kind: "plan", localeSuffix: "paid_unknown" },
        ...fallbackPlans,
      ],
    },
    {
      key: "codex",
      plans: [
        { key: "free", kind: "plan", localeSuffix: "free" },
        { key: "plus", kind: "plan", localeSuffix: "plus" },
        { key: "pro", kind: "plan", localeSuffix: "pro" },
        { key: "pro-lite", kind: "plan", localeSuffix: "pro_lite" },
        { key: "team", kind: "plan", localeSuffix: "team" },
        ...fallbackPlans,
      ],
    },
    {
      key: "claude",
      plans: [
        { key: "free", kind: "plan", localeSuffix: "free" },
        { key: "pro", kind: "plan", localeSuffix: "pro" },
        { key: "max", kind: "plan", localeSuffix: "max" },
        { key: "team", kind: "plan", localeSuffix: "team" },
        ...fallbackPlans,
      ],
    },
    {
      key: "gemini-cli",
      plans: [
        { key: "free", kind: "plan", localeSuffix: "free" },
        { key: "legacy", kind: "plan", localeSuffix: "legacy" },
        { key: "standard", kind: "plan", localeSuffix: "standard" },
        { key: "pro", kind: "plan", localeSuffix: "pro" },
        { key: "ultra", kind: "plan", localeSuffix: "ultra" },
        ...fallbackPlans,
      ],
    },
    {
      key: "antigravity",
      plans: [
        { key: "free", kind: "plan", localeSuffix: "free" },
        { key: "pro", kind: "plan", localeSuffix: "pro" },
        { key: "ultra", kind: "plan", localeSuffix: "ultra" },
        { key: "ultra-lite", kind: "plan", localeSuffix: "ultra_lite" },
        ...fallbackPlans,
      ],
    },
    { key: "kimi", plans: [...fallbackPlans] },
  ];

export const normalizeOAuthModelPlanKey = (
  value: string,
  provider?: string,
): string => {
  const normalized = value.trim().toLowerCase();
  if (normalized.startsWith("_")) {
    return `_${normalized.slice(1).replace(/_/g, "-")}`;
  }
  let key = normalized.replace(/_/g, "-");
  if (key.startsWith("plan-")) key = key.slice(5);
  if (provider === "codex" && key === "prolite") return "pro-lite";
  const aliases: Record<string, Record<string, string>> = {
    xai: {
      "super-grok": "supergrok",
      "super-grok-heavy": "supergrok-heavy",
    },
    "gemini-cli": {
      "free-tier": "free",
      "legacy-tier": "legacy",
      "standard-tier": "standard",
      "g1-pro-tier": "pro",
      "pro-tier": "pro",
      "g1-ultra-tier": "ultra",
      "ultra-tier": "ultra",
    },
    antigravity: {
      "free-tier": "free",
      "g1-pro-tier": "pro",
      "g1-ultra-tier": "ultra",
      "g1-ultra-lite-tier": "ultra-lite",
    },
  };
  return (provider && aliases[provider]?.[key]) || key;
};

export const planDefinitionsForProvider = (
  provider: OAuthModelProviderDefinition,
  plans: Record<string, OAuthModelPlanRule>,
): OAuthModelPlanDefinition[] => {
  const fallback = provider.plans.filter(({ kind }) => kind === "fallback");
  const regular = provider.plans.filter(({ kind }) => kind !== "fallback");
  const known = new Set(provider.plans.map(({ key }) => key));
  const custom = Object.keys(plans)
    .filter((key) => !known.has(key) && !key.startsWith("_"))
    .sort((left, right) => left.localeCompare(right))
    .map((key) => ({ key, kind: "custom" as const, localeSuffix: "custom" }));
  return [...regular, ...custom, ...fallback];
};

const asRecord = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};

const asString = (value: unknown, fallback = ""): string =>
  typeof value === "string" ? value : value == null ? fallback : String(value);

export const normalizeOAuthPolicyPrefix = (value: unknown): string | undefined => {
  const prefix = asString(value).trim().replace(/^\/+|\/+$/g, "");
  return prefix || undefined;
};

const hasOwn = (source: Record<string, unknown>, key: string): boolean =>
  Object.prototype.hasOwnProperty.call(source, key);

const normalizeModelPatterns = (value: unknown): string[] => {
  if (!Array.isArray(value)) return [];
  const seen = new Set<string>();
  return value
    .map((item) => asString(item).trim().toLowerCase())
    .filter((item) => {
      if (!item || seen.has(item)) return false;
      seen.add(item);
      return true;
    });
};

export const defaultOAuthPolicyConfig = (): OAuthPolicyConfig => ({
  enabled: false,
  cacheTTL: "30m",
  resolveTimeout: "15s",
  providers: Object.fromEntries(
    OAUTH_MODEL_PROVIDER_DEFINITIONS.map((provider) => [
      provider.key,
      {
        plans: Object.fromEntries(
          provider.plans.map(({ key }) => [
            key,
            { configured: false, excludedModels: [] },
          ]),
        ),
      },
    ]),
  ),
});

export const normalizeOAuthPolicyConfig = (
  value: unknown,
): OAuthPolicyConfig => {
  const source = asRecord(value);
  const defaults = defaultOAuthPolicyConfig();
  const providers = asRecord(source.providers);
  const providerDefinitions = new Map(
    OAUTH_MODEL_PROVIDER_DEFINITIONS.map((provider) => [
      provider.key,
      provider,
    ]),
  );
  const providerKeys = new Set([
    ...OAUTH_MODEL_PROVIDER_KEYS,
    ...Object.keys(providers),
  ]);
  const normalizedProviders = Object.fromEntries(
    [...providerKeys].map((providerKey) => {
      const provider = providerDefinitions.get(
        providerKey as OAuthModelProviderKey,
      );
      const providerSource = asRecord(providers[providerKey]);
      const plans = asRecord(providerSource.plans);
      const normalizedPlanSources = Object.fromEntries(
        Object.entries(plans).map(([key, plan]) => [
          normalizeOAuthModelPlanKey(key, providerKey),
          plan,
        ]),
      );
      const keys = new Set([
        ...(provider?.plans.map(({ key }) => key) ?? []),
        ...Object.keys(normalizedPlanSources),
      ]);
      const normalizedPlans = Object.fromEntries(
        [...keys].map((key) => {
          const configured = hasOwn(normalizedPlanSources, key);
          const plan = asRecord(normalizedPlanSources[key]);
          return [
            key,
            {
              configured,
              excludedModels: normalizeModelPatterns(plan["excluded-models"]),
              ...(hasOwn(plan, "prefix") && normalizeOAuthPolicyPrefix(plan.prefix) !== undefined
                ? { prefix: normalizeOAuthPolicyPrefix(plan.prefix) }
                : {}),
              ...(hasOwn(plan, "priority") && Number.isInteger(Number(plan.priority))
                ? { priority: Number(plan.priority) }
                : {}),
              ...(hasOwn(plan, "weight") && Number.isInteger(Number(plan.weight))
                ? { weight: Math.min(1_000_000, Math.max(0, Number(plan.weight))) }
                : {}),
            },
          ];
        }),
      );
      return [providerKey, { plans: normalizedPlans }];
    }),
  ) as OAuthPolicyConfig["providers"];
  return {
    enabled: source.enabled === true,
    cacheTTL:
      asString(source["cache-ttl"], defaults.cacheTTL).trim() ||
      defaults.cacheTTL,
    resolveTimeout:
      asString(source["resolve-timeout"], defaults.resolveTimeout).trim() ||
      defaults.resolveTimeout,
    providers: normalizedProviders,
  };
};

export const serializeOAuthPolicyConfig = (
  config: OAuthPolicyConfig,
): Record<string, unknown> => {
  const providers = Object.fromEntries(
    Object.entries(config.providers).map(([providerKey, provider]) => {
      const plans: Record<string, unknown> = {};
      Object.entries(provider.plans).forEach(([key, rule]) => {
        if (!rule.configured) return;
        const prefix = normalizeOAuthPolicyPrefix(rule.prefix);
        plans[key] = {
          "excluded-models": normalizeModelPatterns(rule.excludedModels),
          ...(prefix !== undefined ? { prefix } : {}),
          ...(rule.priority !== undefined ? { priority: Math.trunc(rule.priority) } : {}),
          ...(rule.weight !== undefined
            ? { weight: Math.min(1_000_000, Math.max(0, Math.trunc(rule.weight))) }
            : {}),
        };
      });
      return [providerKey, { plans }];
    }),
  );
  return {
    enabled: config.enabled,
    "cache-ttl": config.cacheTTL.trim(),
    "resolve-timeout": config.resolveTimeout.trim(),
    providers,
  };
};

export const oauthPolicyDurationValue = (
  value: string,
  targetUnit: OAuthPolicyDurationUnit,
): number | null => parsePositiveGoDuration(value, targetUnit);

export const serializeOAuthPolicyDuration = (
  value: number,
  unit: OAuthPolicyDurationUnit,
): string => serializeGoDuration(value, unit);

export const isPositiveDuration = (value: string): boolean =>
  oauthPolicyDurationValue(value, "s") !== null;

export const oauthPolicyApi = {
  async load(): Promise<OAuthPolicySnapshot> {
    const [rawConfig, status, effective] = await Promise.all([
      apiClient.get('/pro/oauth-policy/config'),
      apiClient.get('/pro/oauth-policy/status'),
      apiClient.get('/pro/oauth-policy/effective'),
    ]);
    return {
      config: normalizeOAuthPolicyConfig(rawConfig),
      status: status as OAuthPolicySnapshot['status'],
      effective: Array.isArray((effective as { items?: unknown }).items)
        ? ((effective as { items: OAuthPolicyEffectiveItem[] }).items)
        : [],
    };
  },

  async save(
    config: OAuthPolicyConfig,
  ): Promise<OAuthPolicySnapshot> {
    await apiClient.patch(
      '/pro/oauth-policy/config',
      serializeOAuthPolicyConfig({ ...config, enabled: true }),
    );
    return this.load();
  },
};
