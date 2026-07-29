import { describe, expect, it } from "vitest";
import {
  isPositiveDuration,
  normalizeOAuthModelPolicyConfig,
  serializeOAuthModelPolicyConfig,
} from "@/services/api/oauthModelPolicy";

describe("oauth model policy service", () => {
  it("normalizes known plans and preserves fallback distinction", () => {
    const config = normalizeOAuthModelPolicyConfig({
      priority: 20,
      "cache-ttl": "45m",
      "resolve-timeout": "8s",
      providers: {
        xai: {
          plans: {
            free: { "excluded-models": [" GROK-PRO-* ", "grok-pro-*"] },
            _unknown: { "excluded-models": ["grok-preview-*"] },
          },
        },
      },
    });

    expect(config.priority).toBe(20);
    expect(config.cacheTTL).toBe("45m");
    expect(config.providers.xai.plans.free).toEqual({
      configured: true,
      excludedModels: ["grok-pro-*"],
    });
    expect(config.providers.xai.plans._unknown.configured).toBe(true);
    expect(config.providers.xai.plans._default.configured).toBe(false);
  });

  it("serializes only explicitly configured rules", () => {
    const config = normalizeOAuthModelPolicyConfig({});
    config.providers.xai.plans["x-premium-plus"] = {
      configured: true,
      excludedModels: [],
    };
    config.providers.xai.plans._default = {
      configured: true,
      excludedModels: ["grok-experimental-*"],
    };

    const serialized = serializeOAuthModelPolicyConfig(config);
    expect(serialized).toMatchObject({
      enabled: true,
      priority: 10,
      "cache-ttl": "30m",
      "resolve-timeout": "15s",
      providers: {
        xai: {
          plans: {
            "x-premium-plus": { "excluded-models": [] },
            _default: { "excluded-models": ["grok-experimental-*"] },
          },
        },
      },
    });
    expect(
      (
        serialized.providers as {
          xai: { plans: Record<string, unknown> };
        }
      ).xai.plans.free,
    ).toBeUndefined();
  });

  it("validates positive Go duration fields", () => {
    expect(isPositiveDuration("30m")).toBe(true);
    expect(isPositiveDuration("1.5s")).toBe(true);
    expect(isPositiveDuration("0s")).toBe(false);
    expect(isPositiveDuration("30")).toBe(false);
  });
});
