import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type KeyboardEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ToggleSwitch } from "@/components/ui/ToggleSwitch";
import {
  IconAlertTriangle,
  IconCheckCircle2,
  IconInfo,
  IconModelCluster,
  IconPlus,
  IconRefreshCw,
  IconSettings,
  IconX,
} from "@/components/ui/icons";
import {
  defaultOAuthModelPolicyConfig,
  isPositiveDuration,
  oauthModelPolicyApi,
  XAI_PLAN_DEFINITIONS,
  type OAuthModelPlanKey,
  type OAuthModelPlanRule,
  type OAuthModelPolicyConfig,
  type OAuthModelPolicySnapshot,
} from "@/services/api/oauthModelPolicy";
import { useAuthStore, useNotificationStore } from "@/stores";
import styles from "./OAuthModelPolicyPage.module.scss";

const errorMessage = (error: unknown): string =>
  error instanceof Error ? error.message : String(error || "Unknown error");

const planLocaleSuffix: Record<OAuthModelPlanKey, string> = {
  free: "free",
  supergrok: "supergrok",
  "x-premium-plus": "x_premium_plus",
  "supergrok-heavy": "supergrok_heavy",
  "paid-unknown": "paid_unknown",
  _unknown: "unknown",
  _default: "default",
};

const isLikelyValidGlob = (value: string): boolean => {
  let escaped = false;
  for (let index = 0; index < value.length; index += 1) {
    const character = value[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (character === "\\") {
      escaped = true;
      continue;
    }
    if (character !== "[") continue;
    let closing = index + 1;
    while (closing < value.length && value[closing] !== "]") closing += 1;
    if (closing >= value.length || closing === index + 1) return false;
    index = closing;
  }
  return !escaped;
};

interface PatternEditorProps {
  planKey: OAuthModelPlanKey;
  disabled: boolean;
  patterns: string[];
  onChange: (patterns: string[]) => void;
}

function PatternEditor({
  planKey,
  disabled,
  patterns,
  onChange,
}: PatternEditorProps) {
  const { t } = useTranslation();
  const [value, setValue] = useState("");

  const addPatterns = () => {
    const seen = new Set(patterns.map((pattern) => pattern.toLowerCase()));
    const additions = value
      .split(/[\n,]/)
      .map((pattern) => pattern.trim().toLowerCase())
      .filter((pattern) => pattern && !seen.has(pattern));
    if (additions.length === 0) return;
    onChange([...patterns, ...additions]);
    setValue("");
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key !== "Enter") return;
    event.preventDefault();
    addPatterns();
  };

  return (
    <div className={styles.patternEditor}>
      <div className={styles.patternList}>
        {patterns.length === 0 ? (
          <span className={styles.patternEmpty}>
            {t("oauth_model_policy.no_exclusions", {
              defaultValue: "No excluded models",
            })}
          </span>
        ) : (
          patterns.map((pattern) => (
            <span
              key={pattern}
              className={`${styles.patternChip} ${
                isLikelyValidGlob(pattern) ? "" : styles.patternInvalid
              }`}
            >
              <code>{pattern}</code>
              <button
                type="button"
                disabled={disabled}
                onClick={() =>
                  onChange(patterns.filter((item) => item !== pattern))
                }
                aria-label={t("oauth_model_policy.remove_pattern", {
                  defaultValue: "Remove {{pattern}}",
                  pattern,
                })}
              >
                <IconX size={13} />
              </button>
            </span>
          ))
        )}
      </div>
      <div className={styles.patternInputRow}>
        <input
          value={value}
          disabled={disabled}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={t("oauth_model_policy.pattern_placeholder", {
            defaultValue: "e.g. grok-pro-*",
          })}
          aria-label={t("oauth_model_policy.pattern_input", {
            defaultValue: "Model pattern for {{plan}}",
            plan: planKey,
          })}
        />
        <Button
          variant="secondary"
          size="sm"
          disabled={disabled || !value.trim()}
          onClick={addPatterns}
        >
          <IconPlus size={14} />
          {t("common.add", { defaultValue: "Add" })}
        </Button>
      </div>
      <p className={styles.patternHint}>
        {t("oauth_model_policy.pattern_hint", {
          defaultValue:
            "Supports *, ?, and character ranges. Enter or commas add multiple rules.",
        })}
      </p>
    </div>
  );
}

export function OAuthModelPolicyPage() {
  const { t } = useTranslation();
  const connectionStatus = useAuthStore((state) => state.connectionStatus);
  const supportsPlugin = useAuthStore((state) => state.supportsPlugin);
  const showNotification = useNotificationStore(
    (state) => state.showNotification,
  );
  const [snapshot, setSnapshot] = useState<OAuthModelPolicySnapshot | null>(
    null,
  );
  const [draft, setDraft] = useState<OAuthModelPolicyConfig>(
    defaultOAuthModelPolicyConfig,
  );
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [loadError, setLoadError] = useState("");

  const load = useCallback(
    async (replaceDraft = false) => {
      if (connectionStatus !== "connected" || !supportsPlugin) {
        setLoading(false);
        return;
      }
      setLoading(true);
      try {
        const next = await oauthModelPolicyApi.load();
        setSnapshot(next);
        if (!dirty || replaceDraft) setDraft(next.config);
        setLoadError("");
      } catch (error) {
        setLoadError(errorMessage(error));
      } finally {
        setLoading(false);
      }
    },
    [connectionStatus, dirty, supportsPlugin],
  );

  useEffect(() => {
    void load();
  }, [load]);

  const updateDraft = useCallback(
    (
      next:
        | OAuthModelPolicyConfig
        | ((current: OAuthModelPolicyConfig) => OAuthModelPolicyConfig),
    ) => {
      setDraft((current) =>
        typeof next === "function" ? next(current) : next,
      );
      setDirty(true);
    },
    [],
  );

  const patchPlan = (
    key: OAuthModelPlanKey,
    patch: Partial<OAuthModelPlanRule>,
  ) => {
    updateDraft((current) => ({
      ...current,
      providers: {
        ...current.providers,
        xai: {
          plans: {
            ...current.providers.xai.plans,
            [key]: { ...current.providers.xai.plans[key], ...patch },
          },
        },
      },
    }));
  };

  const configuredCount = useMemo(
    () =>
      XAI_PLAN_DEFINITIONS.filter(
        ({ key }) => draft.providers.xai.plans[key].configured,
      ).length,
    [draft.providers.xai.plans],
  );
  const excludedCount = useMemo(
    () =>
      XAI_PLAN_DEFINITIONS.reduce(
        (total, { key }) =>
          total +
          (draft.providers.xai.plans[key].configured
            ? draft.providers.xai.plans[key].excludedModels.length
            : 0),
        0,
      ),
    [draft.providers.xai.plans],
  );

  const inheritedRule = (key: OAuthModelPlanKey): string => {
    const plans = draft.providers.xai.plans;
    if (plans[key].configured) return "";
    if (key === "_default")
      return t("oauth_model_policy.no_rule", { defaultValue: "No rule" });
    if (key === "_unknown" && plans._unknown.configured) return "";
    if (key === "_unknown" && plans._default.configured)
      return t("oauth_model_policy.inherits_default", {
        defaultValue: "Uses _default",
      });
    if (!key.startsWith("_") && plans._default.configured)
      return t("oauth_model_policy.inherits_default", {
        defaultValue: "Uses _default",
      });
    return t("oauth_model_policy.no_rule", { defaultValue: "No plugin rule" });
  };

  const validate = (): string => {
    if (!isPositiveDuration(draft.cacheTTL))
      return t("oauth_model_policy.invalid_cache_ttl", {
        defaultValue: "Cache TTL must be a positive Go duration, such as 30m.",
      });
    if (!isPositiveDuration(draft.resolveTimeout))
      return t("oauth_model_policy.invalid_resolve_timeout", {
        defaultValue:
          "Resolve timeout must be a positive Go duration, such as 15s.",
      });
    for (const { key } of XAI_PLAN_DEFINITIONS) {
      const invalid = draft.providers.xai.plans[key].excludedModels.find(
        (pattern) => !isLikelyValidGlob(pattern),
      );
      if (invalid)
        return t("oauth_model_policy.invalid_pattern", {
          defaultValue: "Invalid model pattern: {{pattern}}",
          pattern: invalid,
        });
    }
    return "";
  };

  const save = async () => {
    const validation = validate();
    if (validation) {
      showNotification(validation, "error");
      return;
    }
    setSaving(true);
    try {
      const next = await oauthModelPolicyApi.save(draft);
      setSnapshot(next);
      setDraft(next.config);
      setDirty(false);
      setLoadError("");
      showNotification(
        t("oauth_model_policy.save_success", {
          defaultValue: "OAuth model policy saved",
        }),
        "success",
      );
    } catch (error) {
      showNotification(
        t("oauth_model_policy.save_failed", {
          defaultValue: "Save failed: {{message}}",
          message: errorMessage(error),
        }),
        "error",
      );
    } finally {
      setSaving(false);
    }
  };

  const discard = () => {
    setDraft(snapshot?.config ?? defaultOAuthModelPolicyConfig());
    setDirty(false);
  };

  if (!supportsPlugin) {
    return (
      <div className={styles.page}>
        <div className={styles.noticeCard}>
          <IconAlertTriangle size={22} />
          <div>
            <strong>
              {t("oauth_model_policy.unsupported_title", {
                defaultValue: "Plugin runtime required",
              })}
            </strong>
            <p>
              {t("oauth_model_policy.unsupported_body", {
                defaultValue:
                  "Use a standard Pro release instead of a _no-plugin build.",
              })}
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={`${styles.page} ${dirty ? styles.pageWithSave : ""}`}>
      <header className={styles.header}>
        <div className={styles.headerIdentity}>
          <span
            className={`${styles.headerIcon} ${
              snapshot?.pluginRegistered ? styles.headerIconActive : ""
            }`}
          >
            <IconModelCluster size={22} />
          </span>
          <div>
            <div className={styles.titleLine}>
              <h1>
                {t("oauth_model_policy.title", {
                  defaultValue: "OAuth Model Policy",
                })}
              </h1>
              {snapshot?.pluginVersion && (
                <code>v{snapshot.pluginVersion}</code>
              )}
            </div>
            <p>
              {t("oauth_model_policy.subtitle", {
                defaultValue:
                  "Filter each xAI OAuth account model set by its detected plan.",
              })}
            </p>
          </div>
        </div>
        <Button
          variant="secondary"
          size="sm"
          disabled={loading || saving}
          onClick={() => void load()}
        >
          <IconRefreshCw size={15} />
          {t("common.refresh")}
        </Button>
      </header>

      {loadError && <div className={styles.errorBanner}>{loadError}</div>}
      {!loading && snapshot && !snapshot.pluginDiscovered && (
        <div className={styles.errorBanner}>
          {t("oauth_model_policy.plugin_missing", {
            defaultValue: "Bundled oauth-model-policy plugin was not found.",
          })}
        </div>
      )}
      {!loading && snapshot?.pluginDiscovered && !snapshot.pluginRegistered && (
        <div className={styles.warningBanner}>
          {t("oauth_model_policy.plugin_not_registered", {
            defaultValue:
              "The plugin is installed but not running. Saving valid settings will enable it.",
          })}
        </div>
      )}

      {!snapshot ? (
        <div className={styles.noticeCard}>
          <IconInfo size={21} />
          <div>
            <strong>
              {loading
                ? t("oauth_model_policy.loading", {
                    defaultValue: "Loading model policy...",
                  })
                : t("oauth_model_policy.load_unavailable", {
                    defaultValue: "Model policy is unavailable",
                  })}
            </strong>
            <p>
              {t("oauth_model_policy.loading_hint", {
                defaultValue:
                  "Reading plugin discovery state and configuration.",
              })}
            </p>
          </div>
        </div>
      ) : (
        snapshot.pluginDiscovered && (
          <>
            <section className={styles.statusGrid}>
              <div>
                <span
                  className={
                    snapshot.pluginRegistered
                      ? styles.statusGood
                      : styles.statusMuted
                  }
                >
                  {snapshot.pluginRegistered ? (
                    <IconCheckCircle2 size={18} />
                  ) : (
                    <IconAlertTriangle size={18} />
                  )}
                </span>
                <small>
                  {t("oauth_model_policy.runtime", { defaultValue: "Runtime" })}
                </small>
                <strong>
                  {snapshot.pluginRegistered
                    ? t("oauth_model_policy.running", {
                        defaultValue: "Running",
                      })
                    : t("oauth_model_policy.stopped", {
                        defaultValue: "Not running",
                      })}
                </strong>
              </div>
              <div>
                <span className={styles.statusAccent}>{configuredCount}</span>
                <small>
                  {t("oauth_model_policy.configured_plans", {
                    defaultValue: "Plan rules",
                  })}
                </small>
                <strong>
                  {t("oauth_model_policy.configured_count", {
                    defaultValue: "{{count}} configured",
                    count: configuredCount,
                  })}
                </strong>
              </div>
              <div>
                <span className={styles.statusAccent}>{excludedCount}</span>
                <small>
                  {t("oauth_model_policy.model_patterns", {
                    defaultValue: "Model patterns",
                  })}
                </small>
                <strong>
                  {t("oauth_model_policy.pattern_count", {
                    defaultValue: "{{count}} exclusions",
                    count: excludedCount,
                  })}
                </strong>
              </div>
              <div>
                <span className={styles.statusMuted}>xAI</span>
                <small>
                  {t("oauth_model_policy.provider", {
                    defaultValue: "Provider",
                  })}
                </small>
                <strong>OAuth</strong>
              </div>
            </section>

            <section className={styles.settingsPanel}>
              <div className={styles.sectionHeading}>
                <span>
                  <IconSettings size={19} />
                </span>
                <div>
                  <h2>
                    {t("oauth_model_policy.discovery_settings", {
                      defaultValue: "Plan discovery",
                    })}
                  </h2>
                  <p>
                    {t("oauth_model_policy.discovery_hint", {
                      defaultValue:
                        "Auth metadata is preferred; billing is queried only when the plan is missing.",
                    })}
                  </p>
                </div>
              </div>
              <div className={styles.settingsGrid}>
                <Input
                  label={t("oauth_model_policy.cache_ttl", {
                    defaultValue: "Plan cache TTL",
                  })}
                  value={draft.cacheTTL}
                  onChange={(event) =>
                    updateDraft({ ...draft, cacheTTL: event.target.value })
                  }
                  placeholder="30m"
                />
                <Input
                  label={t("oauth_model_policy.resolve_timeout", {
                    defaultValue: "Billing timeout",
                  })}
                  value={draft.resolveTimeout}
                  onChange={(event) =>
                    updateDraft({
                      ...draft,
                      resolveTimeout: event.target.value,
                    })
                  }
                  placeholder="15s"
                />
                <Input
                  type="number"
                  label={t("oauth_model_policy.priority", {
                    defaultValue: "Plugin priority",
                  })}
                  value={draft.priority}
                  onChange={(event) =>
                    updateDraft({
                      ...draft,
                      priority: Number(event.target.value) || 0,
                    })
                  }
                />
              </div>
            </section>

            <section className={styles.policyPanel}>
              <div className={styles.policyHeader}>
                <div>
                  <h2>
                    {t("oauth_model_policy.xai_rules", {
                      defaultValue: "xAI plan rules",
                    })}
                  </h2>
                  <p>
                    {t("oauth_model_policy.xai_rules_hint", {
                      defaultValue:
                        "Each enabled rule subtracts matching model IDs from that account only.",
                    })}
                  </p>
                </div>
                <span className={styles.flowBadge}>
                  {t("oauth_model_policy.processing_order", {
                    defaultValue:
                      "excluded_models → plan policy → alias / prefix",
                  })}
                </span>
              </div>
              <div className={styles.ruleGrid}>
                {XAI_PLAN_DEFINITIONS.map((definition) => {
                  const rule = draft.providers.xai.plans[definition.key];
                  const suffix = planLocaleSuffix[definition.key];
                  const inherited = inheritedRule(definition.key);
                  return (
                    <article
                      key={definition.key}
                      className={`${styles.ruleCard} ${
                        rule.configured ? styles.ruleCardActive : ""
                      }`}
                    >
                      <div className={styles.ruleHeader}>
                        <div>
                          <div className={styles.ruleTitleLine}>
                            <h3>
                              {t(`oauth_model_policy.plan_${suffix}`, {
                                defaultValue: definition.key,
                              })}
                            </h3>
                            <code>{definition.key}</code>
                          </div>
                          <p>
                            {t(`oauth_model_policy.plan_${suffix}_hint`, {
                              defaultValue:
                                definition.kind === "fallback"
                                  ? "Fallback policy"
                                  : "Detected xAI subscription plan",
                            })}
                          </p>
                          {definition.monthlyLimitCents !== undefined && (
                            <small>
                              {definition.monthlyLimitCents === 0
                                ? t("oauth_model_policy.no_paid_limit", {
                                    defaultValue: "Free plan",
                                  })
                                : t("oauth_model_policy.monthly_limit", {
                                    defaultValue:
                                      "{{count}} cents monthly limit",
                                    count: definition.monthlyLimitCents,
                                  })}
                            </small>
                          )}
                        </div>
                        <ToggleSwitch
                          checked={rule.configured}
                          onChange={(configured) =>
                            patchPlan(definition.key, { configured })
                          }
                          ariaLabel={t("oauth_model_policy.configure_plan", {
                            defaultValue: "Configure {{plan}} rule",
                            plan: definition.key,
                          })}
                        />
                      </div>
                      {rule.configured ? (
                        <PatternEditor
                          planKey={definition.key}
                          disabled={saving}
                          patterns={rule.excludedModels}
                          onChange={(excludedModels) =>
                            patchPlan(definition.key, { excludedModels })
                          }
                        />
                      ) : (
                        <div className={styles.inheritedRule}>
                          <IconInfo size={15} />
                          <span>{inherited}</span>
                        </div>
                      )}
                    </article>
                  );
                })}
              </div>
              <div className={styles.behaviorNote}>
                <IconInfo size={18} />
                <p>
                  {t("oauth_model_policy.empty_rule_behavior", {
                    defaultValue:
                      "An enabled rule with no patterns explicitly allows the full current model set and stops fallback matching.",
                  })}
                </p>
              </div>
            </section>
          </>
        )
      )}

      {dirty && (
        <div
          className={styles.saveBar}
          role="region"
          aria-label={t("common.save")}
        >
          <div>
            <strong>
              {t("oauth_model_policy.unsaved", {
                defaultValue: "Unsaved policy changes",
              })}
            </strong>
            <span>
              {t("oauth_model_policy.save_enables_plugin", {
                defaultValue: "Saving also enables the bundled plugin runtime.",
              })}
            </span>
          </div>
          <div className={styles.saveActions}>
            <Button variant="ghost" disabled={saving} onClick={discard}>
              {t("oauth_model_policy.discard", { defaultValue: "Discard" })}
            </Button>
            <Button loading={saving} onClick={() => void save()}>
              {t("common.save")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
