import { parseDocument } from "yaml";
import { configFileApi } from "./configFile";
import { pluginsApi } from "./plugins";

export const OAUTH_MODEL_POLICY_PLUGIN_ID = "oauth-model-policy";

export type OAuthModelPlanKey =
  | "free"
  | "supergrok"
  | "x-premium-plus"
  | "supergrok-heavy"
  | "paid-unknown"
  | "_unknown"
  | "_default";

export interface OAuthModelPlanDefinition {
  key: OAuthModelPlanKey;
  monthlyLimitCents?: number;
  kind: "plan" | "fallback";
}

export interface OAuthModelPlanRule {
  configured: boolean;
  excludedModels: string[];
}

export interface OAuthModelPolicyConfig {
  priority: number;
  cacheTTL: string;
  resolveTimeout: string;
  providers: {
    xai: {
      plans: Record<OAuthModelPlanKey, OAuthModelPlanRule>;
    };
  };
}

export interface OAuthModelPolicySnapshot {
  pluginsEnabled: boolean;
  pluginDiscovered: boolean;
  pluginEnabled: boolean;
  pluginRegistered: boolean;
  pluginVersion: string;
  config: OAuthModelPolicyConfig;
}

export const XAI_PLAN_DEFINITIONS: OAuthModelPlanDefinition[] = [
  { key: "free", monthlyLimitCents: 0, kind: "plan" },
  { key: "supergrok", monthlyLimitCents: 15_000, kind: "plan" },
  { key: "x-premium-plus", monthlyLimitCents: 20_000, kind: "plan" },
  { key: "supergrok-heavy", monthlyLimitCents: 150_000, kind: "plan" },
  { key: "paid-unknown", kind: "plan" },
  { key: "_unknown", kind: "fallback" },
  { key: "_default", kind: "fallback" },
];

const asRecord = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};

const asString = (value: unknown, fallback = ""): string =>
  typeof value === "string" ? value : value == null ? fallback : String(value);

const asNumber = (value: unknown, fallback: number): number => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
};

const hasOwn = (source: Record<string, unknown>, key: string): boolean =>
  Object.prototype.hasOwnProperty.call(source, key);

export const normalizeModelPatterns = (value: unknown): string[] => {
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

export const defaultOAuthModelPolicyConfig = (): OAuthModelPolicyConfig => ({
  priority: 10,
  cacheTTL: "30m",
  resolveTimeout: "15s",
  providers: {
    xai: {
      plans: {
        free: { configured: false, excludedModels: [] },
        supergrok: { configured: false, excludedModels: [] },
        "x-premium-plus": { configured: false, excludedModels: [] },
        "supergrok-heavy": { configured: false, excludedModels: [] },
        "paid-unknown": { configured: false, excludedModels: [] },
        _unknown: { configured: false, excludedModels: [] },
        _default: { configured: false, excludedModels: [] },
      },
    },
  },
});

export const normalizeOAuthModelPolicyConfig = (
  value: unknown,
): OAuthModelPolicyConfig => {
  const source = asRecord(value);
  const defaults = defaultOAuthModelPolicyConfig();
  const providers = asRecord(source.providers);
  const xai = asRecord(providers.xai);
  const plans = asRecord(xai.plans);
  const normalizedPlans = Object.fromEntries(
    XAI_PLAN_DEFINITIONS.map(({ key }) => {
      const configured = hasOwn(plans, key);
      const plan = asRecord(plans[key]);
      return [
        key,
        {
          configured,
          excludedModels: normalizeModelPatterns(plan["excluded-models"]),
        },
      ];
    }),
  ) as Record<OAuthModelPlanKey, OAuthModelPlanRule>;
  return {
    priority: Math.trunc(asNumber(source.priority, defaults.priority)),
    cacheTTL:
      asString(source["cache-ttl"], defaults.cacheTTL).trim() ||
      defaults.cacheTTL,
    resolveTimeout:
      asString(source["resolve-timeout"], defaults.resolveTimeout).trim() ||
      defaults.resolveTimeout,
    providers: { xai: { plans: normalizedPlans } },
  };
};

export const serializeOAuthModelPolicyConfig = (
  config: OAuthModelPolicyConfig,
): Record<string, unknown> => {
  const plans: Record<string, unknown> = {};
  XAI_PLAN_DEFINITIONS.forEach(({ key }) => {
    const rule = config.providers.xai.plans[key];
    if (!rule.configured) return;
    plans[key] = {
      "excluded-models": normalizeModelPatterns(rule.excludedModels),
    };
  });
  return {
    enabled: true,
    priority: Math.trunc(config.priority),
    "cache-ttl": config.cacheTTL.trim(),
    "resolve-timeout": config.resolveTimeout.trim(),
    providers: { xai: { plans } },
  };
};

export const isPositiveDuration = (value: string): boolean =>
  /^(?:\d+(?:\.\d+)?)(?:ns|us|µs|ms|s|m|h)$/.test(value.trim()) &&
  Number.parseFloat(value) > 0;

const ensureGlobalPluginSwitch = async (): Promise<void> => {
  const raw = await configFileApi.fetchConfigYaml();
  const document = parseDocument(raw || "{}");
  if (document.errors.length > 0) throw document.errors[0];
  document.setIn(["plugins", "enabled"], true);
  await configFileApi.saveConfigYaml(document.toString());
};

const waitForRegistration = async (attempts = 24): Promise<void> => {
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    const list = await pluginsApi.list();
    const plugin = list.plugins.find(
      (entry) => entry.id === OAUTH_MODEL_POLICY_PLUGIN_ID,
    );
    if (plugin?.registered) return;
    await new Promise((resolve) => globalThis.setTimeout(resolve, 250));
  }
  throw new Error("OAuth model policy plugin did not become ready in time");
};

export const oauthModelPolicyApi = {
  async load(): Promise<OAuthModelPolicySnapshot> {
    const [pluginList, rawConfig] = await Promise.all([
      pluginsApi.list(),
      pluginsApi.getConfig(OAUTH_MODEL_POLICY_PLUGIN_ID),
    ]);
    const plugin = pluginList.plugins.find(
      (entry) => entry.id === OAUTH_MODEL_POLICY_PLUGIN_ID,
    );
    return {
      pluginsEnabled: pluginList.pluginsEnabled,
      pluginDiscovered: Boolean(plugin),
      pluginEnabled: Boolean(plugin?.enabled),
      pluginRegistered: Boolean(plugin?.registered),
      pluginVersion: plugin?.metadata?.version ?? "",
      config: normalizeOAuthModelPolicyConfig(rawConfig),
    };
  },

  async save(
    config: OAuthModelPolicyConfig,
  ): Promise<OAuthModelPolicySnapshot> {
    const pluginList = await pluginsApi.list();
    const plugin = pluginList.plugins.find(
      (entry) => entry.id === OAUTH_MODEL_POLICY_PLUGIN_ID,
    );
    if (!plugin)
      throw new Error("Bundled oauth-model-policy plugin was not found");
    await pluginsApi.patchConfig(
      OAUTH_MODEL_POLICY_PLUGIN_ID,
      serializeOAuthModelPolicyConfig(config),
    );
    if (!plugin.enabled) {
      await pluginsApi.updateEnabled(OAUTH_MODEL_POLICY_PLUGIN_ID, true);
    }
    if (!pluginList.pluginsEnabled) await ensureGlobalPluginSwitch();
    await waitForRegistration();
    return this.load();
  },
};
