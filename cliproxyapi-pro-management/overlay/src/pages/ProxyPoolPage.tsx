import { useCallback, useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { IconRefreshCw } from '@/components/ui/icons';
import {
  defaultProxyPoolConfig,
  parseProxyPoolImport,
  proxyPoolApi,
  type ProxyPoolConfig,
  type ProxyPoolNodeConfig,
  type ProxyPoolProbeResult,
  type ProxyPoolSnapshot,
} from '@/services/api/proxyPool';
import { useAuthStore, useNotificationStore } from '@/stores';
import styles from './ProxyPoolPage.module.scss';

const errorMessage = (error: unknown): string =>
  error instanceof Error ? error.message : String(error || 'Unknown error');

const maskProxyCredentials = (value: string): string => {
  const schemeEnd = value.indexOf('//');
  const credentialsEnd = value.lastIndexOf('@');
  if (schemeEnd < 0 || credentialsEnd <= schemeEnd + 2) return value;
  return `${value.slice(0, schemeEnd + 2)}***@${value.slice(credentialsEnd + 1)}`;
};

const createNode = (index: number): ProxyPoolNodeConfig => ({
  id:
    typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
      ? `proxy-${crypto.randomUUID().slice(0, 8)}`
      : `proxy-${Date.now().toString(36)}-${index + 1}`,
  label: '',
  url: '',
  enabled: true,
  weight: 1,
  order: (index + 1) * 10,
});

const stateTone = (state: string): string => {
  if (state === 'healthy') return styles.stateHealthy;
  if (state === 'isolated') return styles.stateIsolated;
  if (state === 'degraded') return styles.stateDegraded;
  if (state === 'disabled') return styles.stateDisabled;
  return styles.stateUnknown;
};

const formatTime = (value: string, language: string): string => {
  if (!value || value.startsWith('0001-')) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return new Intl.DateTimeFormat(language, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date);
};

const formatSuccessRate = (success: number, total: number): string =>
  total > 0 ? `${Math.round((success / total) * 1000) / 10}%` : '-';

interface ValidationError {
  key: string;
  defaultValue: string;
  values?: Record<string, string>;
}

const parseLoopbackListener = (value: string): { host: string; port: string } | null => {
  const listen = value.trim();
  const ipv6Match = listen.match(/^\[::1\]:(\d{1,5})$/);
  if (ipv6Match) {
    const port = Number(ipv6Match[1]);
    return port >= 1 && port <= 65535 ? { host: '::1', port: String(port) } : null;
  }
  const separator = listen.lastIndexOf(':');
  if (separator <= 0) return null;
  const host = listen.slice(0, separator);
  const portText = listen.slice(separator + 1);
  const octets = host.split('.');
  const port = Number(portText);
  if (
    octets.length !== 4 ||
    octets[0] !== '127' ||
    octets.some((octet) => !/^\d{1,3}$/.test(octet) || Number(octet) > 255) ||
    !/^\d{1,5}$/.test(portText) ||
    port < 1 ||
    port > 65535
  ) {
    return null;
  }
  return { host, port: String(port) };
};

const validateProxyPoolConfig = (config: ProxyPoolConfig): ValidationError | null => {
  const listener = parseLoopbackListener(config.listen);
  if (!listener) {
    return {
      key: 'proxy_pool.validation_listener',
      defaultValue: 'Listener must be a numeric loopback address with a port from 1 to 65535',
    };
  }
  const ids = new Set<string>();
  const urls = new Set<string>();
  for (const node of config.nodes) {
    const id = node.id.trim();
    const url = node.url.trim();
    if (!id || !url) {
      return {
        key: 'proxy_pool.validation_required',
        defaultValue: 'Every node requires an ID and proxy URL',
      };
    }
    if (ids.has(id)) {
      return {
        key: 'proxy_pool.validation_duplicate_id',
        defaultValue: 'Duplicate node ID: {{value}}',
        values: { value: id },
      };
    }
    if (urls.has(url)) {
      return {
        key: 'proxy_pool.validation_duplicate_url',
        defaultValue: 'Duplicate proxy URL: {{value}}',
        values: { value: url },
      };
    }
    ids.add(id);
    urls.add(url);
    if (!/^(https?|socks5h?):\/\//i.test(url)) {
      return {
        key: 'proxy_pool.validation_unsupported_url',
        defaultValue: 'Unsupported proxy URL: {{value}}',
        values: { value: url },
      };
    }
    try {
      const parsed = new URL(url);
      if (!parsed.hostname) throw new Error('missing host');
      const normalizedHost = parsed.hostname.replace(/^\[|\]$/g, '');
      const normalizedPort =
        parsed.port || (parsed.protocol === 'http:' ? '80' : parsed.protocol === 'https:' ? '443' : '');
      if (normalizedHost === listener.host && normalizedPort === listener.port) {
        return {
          key: 'proxy_pool.validation_recursive_url',
          defaultValue: 'A proxy node cannot point back to the internal listener: {{value}}',
          values: { value: url },
        };
      }
    } catch {
      return {
        key: 'proxy_pool.validation_invalid_url',
        defaultValue: 'Invalid proxy URL: {{value}}',
        values: { value: url },
      };
    }
  }
  return null;
};

function MetricCard({ label, value, hint }: { label: string; value: string | number; hint?: string }) {
  return (
    <div className={styles.metricCard}>
      <span>{label}</span>
      <strong>{value}</strong>
      {hint && <small>{hint}</small>}
    </div>
  );
}

export function ProxyPoolPage() {
  const { t, i18n } = useTranslation();
  const connectionStatus = useAuthStore((state) => state.connectionStatus);
  const supportsPlugin = useAuthStore((state) => state.supportsPlugin);
  const showNotification = useNotificationStore((state) => state.showNotification);
  const [snapshot, setSnapshot] = useState<ProxyPoolSnapshot | null>(null);
  const [draft, setDraft] = useState<ProxyPoolConfig>(defaultProxyPoolConfig);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testingAll, setTestingAll] = useState(false);
  const [testingNode, setTestingNode] = useState('');
  const [recoveringNode, setRecoveringNode] = useState('');
  const [dirty, setDirty] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [probeResults, setProbeResults] = useState<Record<string, ProxyPoolProbeResult>>({});
  const [nodeQuery, setNodeQuery] = useState('');
  const [importOpen, setImportOpen] = useState(false);
  const [importText, setImportText] = useState('');

  const load = useCallback(async (silent = false, replaceDraft = false) => {
    if (connectionStatus !== 'connected' || !supportsPlugin) {
      setLoading(false);
      return;
    }
    if (!silent) setLoading(true);
    try {
      const next = await proxyPoolApi.load();
      setSnapshot(next);
      if (!dirty || replaceDraft) setDraft(next.config);
      setLoadError('');
    } catch (error) {
      setLoadError(errorMessage(error));
    } finally {
      if (!silent) setLoading(false);
    }
  }, [connectionStatus, dirty, supportsPlugin]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (connectionStatus !== 'connected' || !supportsPlugin) return;
    const timer = window.setInterval(() => void load(true), 10_000);
    return () => window.clearInterval(timer);
  }, [connectionStatus, load, supportsPlugin]);

  const statusByID = useMemo(
    () => new Map((snapshot?.status?.nodes ?? []).map((node) => [node.id, node])),
    [snapshot?.status?.nodes]
  );

  const visibleNodeIndexes = useMemo(() => {
    const query = nodeQuery.trim().toLowerCase();
    return draft.nodes
      .map((node, index) => ({ node, index }))
      .filter(({ node }) =>
        !query || [node.id, node.label, node.url].some((value) => value.toLowerCase().includes(query))
      );
  }, [draft.nodes, nodeQuery]);

  const updateDraft = useCallback((updater: (current: ProxyPoolConfig) => ProxyPoolConfig) => {
    setDraft((current) => updater(current));
    setDirty(true);
  }, []);

  const updateNode = (index: number, patch: Partial<ProxyPoolNodeConfig>) => {
    updateDraft((current) => ({
      ...current,
      nodes: current.nodes.map((node, nodeIndex) =>
        nodeIndex === index ? { ...node, ...patch } : node
      ),
    }));
  };

  const save = async (): Promise<boolean> => {
    const validationError = validateProxyPoolConfig(draft);
    if (validationError) {
      showNotification(
        t(validationError.key, {
          defaultValue: validationError.defaultValue,
          ...validationError.values,
        }),
        'error'
      );
      return false;
    }
    setSaving(true);
    try {
      await proxyPoolApi.save(draft, snapshot?.takeoverActive === true);
      setDirty(false);
      showNotification(t('proxy_pool.save_success', { defaultValue: 'Proxy pool saved' }), 'success');
      await load(true, true);
      return true;
    } catch (error) {
      showNotification(
        `${t('proxy_pool.save_failed', { defaultValue: 'Save failed' })}: ${errorMessage(error)}`,
        'error'
      );
      return false;
    } finally {
      setSaving(false);
    }
  };

  const toggleTakeover = async () => {
    const activating = !snapshot?.takeoverActive;
    const validationError = validateProxyPoolConfig(draft);
    if (activating && validationError) {
      showNotification(
        t(validationError.key, {
          defaultValue: validationError.defaultValue,
          ...validationError.values,
        }),
        'error'
      );
      return;
    }
    setSaving(true);
    try {
      if (activating) {
        const localProxyUrl = `socks5://${draft.listen.trim()}`;
        const activationDraft = {
          ...draft,
          restoreProxyUrl:
            snapshot?.globalProxyUrl && snapshot.globalProxyUrl !== localProxyUrl
              ? snapshot.globalProxyUrl
              : draft.restoreProxyUrl,
        };
        await proxyPoolApi.activate(activationDraft);
        setDraft(activationDraft);
      } else {
        await proxyPoolApi.deactivate(draft);
      }
      setDirty(false);
      await load(true, true);
      showNotification(
        activating
          ? t('proxy_pool.takeover_enabled', { defaultValue: 'Global proxy takeover enabled' })
          : t('proxy_pool.takeover_disabled', { defaultValue: 'Global proxy takeover disabled' }),
        'success'
      );
    } catch (error) {
      showNotification(errorMessage(error), 'error');
    } finally {
      setSaving(false);
    }
  };

  const testNode = async (node: ProxyPoolNodeConfig, index: number) => {
    const testKey = node.id.trim() || `draft-${index + 1}`;
    setTestingNode(testKey);
    try {
      const result = await proxyPoolApi.testNode(
        testKey,
        dirty || !statusByID.has(node.id) ? node.url : '',
        dirty ? draft.healthCheck.testUrl : ''
      );
      setProbeResults((current) => ({ ...current, [testKey]: result }));
      showNotification(
        result.success
          ? `${result.exitIp || testKey} · ${result.latencyMs} ms`
          : result.error || 'Proxy test failed',
        result.success ? 'success' : 'error'
      );
      await load(true);
    } catch (error) {
      showNotification(errorMessage(error), 'error');
    } finally {
      setTestingNode('');
    }
  };

  const testAll = async () => {
    setTestingAll(true);
    try {
      let results: ProxyPoolProbeResult[];
      if (dirty) {
        const enabledNodes = draft.nodes
          .map((node, index) => ({ node, index }))
          .filter(({ node }) => node.enabled && node.url.trim());
        results = [];
        for (let offset = 0; offset < enabledNodes.length; offset += 4) {
          const batch = enabledNodes.slice(offset, offset + 4);
          results.push(
            ...(await Promise.all(
              batch.map(({ node, index }) =>
                proxyPoolApi.testNode(
                  node.id.trim() || `draft-${index + 1}`,
                  node.url,
                  draft.healthCheck.testUrl
                )
              )
            ))
          );
        }
      } else {
        results = await proxyPoolApi.testAll();
      }
      setProbeResults(Object.fromEntries(results.map((result) => [result.nodeId, result])));
      if (results.length === 0) {
        showNotification(
          t('proxy_pool.no_nodes_to_test', { defaultValue: 'No enabled proxy nodes to test' }),
          'warning'
        );
        return;
      }
      const successCount = results.filter((result) => result.success).length;
      showNotification(
        t('proxy_pool.test_summary', {
          defaultValue: '{{success}}/{{total}} nodes passed',
          success: successCount,
          total: results.length,
        }),
        successCount === results.length ? 'success' : 'warning'
      );
      await load(true);
    } catch (error) {
      showNotification(errorMessage(error), 'error');
    } finally {
      setTestingAll(false);
    }
  };

  const recoverNode = async (nodeId: string) => {
    setRecoveringNode(nodeId);
    try {
      await proxyPoolApi.recoverNode(nodeId);
      await load(true);
      showNotification(
        t('proxy_pool.recover_success', { defaultValue: 'Node isolation cleared' }),
        'success'
      );
    } catch (error) {
      showNotification(errorMessage(error), 'error');
    } finally {
      setRecoveringNode('');
    }
  };

  const importNodes = () => {
    const result = parseProxyPoolImport(importText, draft.nodes);
    if (result.nodes.length > 0) {
      updateDraft((current) => ({ ...current, nodes: [...current.nodes, ...result.nodes] }));
    }
    if (result.errors.length > 0) {
      const preview = result.errors
        .slice(0, 3)
        .map((item) => `${item.line}: ${item.message}`)
        .join('; ');
      showNotification(
        t('proxy_pool.import_partial', {
          defaultValue: 'Imported {{added}} nodes; skipped {{duplicates}} duplicates; {{errors}} invalid lines ({{detail}})',
          added: result.nodes.length,
          duplicates: result.duplicateCount,
          errors: result.errors.length,
          detail: preview,
        }),
        result.nodes.length > 0 ? 'warning' : 'error'
      );
      return;
    }
    showNotification(
      t('proxy_pool.import_success', {
        defaultValue: 'Imported {{added}} nodes; skipped {{duplicates}} duplicates',
        added: result.nodes.length,
        duplicates: result.duplicateCount,
      }),
      result.nodes.length > 0 ? 'success' : 'warning'
    );
    if (result.nodes.length > 0) {
      setImportText('');
      setImportOpen(false);
    }
  };

  const resetStats = async () => {
    try {
      await proxyPoolApi.resetStats();
      setProbeResults({});
      await load(true);
      showNotification(t('proxy_pool.stats_reset', { defaultValue: 'Runtime stats reset' }), 'success');
    } catch (error) {
      showNotification(errorMessage(error), 'error');
    }
  };

  const discardChanges = () => {
    if (!snapshot) return;
    setDraft(snapshot.config);
    setDirty(false);
    setProbeResults({});
    showNotification(
      t('proxy_pool.changes_discarded', { defaultValue: 'Unsaved changes discarded' }),
      'success'
    );
  };

  if (!supportsPlugin) {
    return (
      <div className={styles.page}>
        <div className={styles.noticeCard}>
          <strong>{t('proxy_pool.unsupported_title', { defaultValue: 'Plugin runtime required' })}</strong>
          <p>{t('proxy_pool.unsupported_body', { defaultValue: 'This build does not support dynamic plugins. Use a standard Pro release instead of a _no-plugin build.' })}</p>
        </div>
      </div>
    );
  }

  const actionBarTarget =
    typeof document !== 'undefined' ? document.querySelector('.main-body') : null;
  const actionBar = snapshot?.pluginDiscovered ? (
    <footer className={styles.actionBar}>
      <div>
        <code>{snapshot.status?.proxyUrl || `socks5://${draft.listen}`}</code>
        <span>{t('proxy_pool.fixed_endpoint_hint', { defaultValue: 'Core only sees this fixed proxy endpoint.' })}</span>
      </div>
      <div>
        <Button variant="ghost" onClick={discardChanges} disabled={!dirty || saving}>
          {t('proxy_pool.discard_changes', { defaultValue: 'Discard changes' })}
        </Button>
        <Button variant="ghost" onClick={() => void resetStats()} disabled={!snapshot.status?.ready || saving}>{t('proxy_pool.reset_stats', { defaultValue: 'Reset stats' })}</Button>
        <Button variant="secondary" onClick={() => void testAll()} loading={testingAll} disabled={!snapshot.status?.ready}>{t('proxy_pool.test_all', { defaultValue: 'Test all' })}</Button>
        <Button onClick={() => void save()} loading={saving} disabled={!dirty}>{t('common.save')}</Button>
      </div>
    </footer>
  ) : null;

  return (
    <div className={styles.page}>
      <header className={styles.hero}>
        <div>
          <span className={styles.eyebrow}>PRO · SOCKS5</span>
          <h1>{t('proxy_pool.title', { defaultValue: 'Proxy Pool' })}</h1>
          <p>{t('proxy_pool.subtitle', { defaultValue: 'Aggregate multiple upstream proxies behind one local SOCKS5 endpoint with rotation, health isolation, and failover.' })}</p>
        </div>
        <div className={styles.heroActions}>
          <Button variant="secondary" onClick={() => void load()} disabled={loading || saving}>
            <IconRefreshCw size={16} />
            {t('common.refresh')}
          </Button>
          <Button
            variant={snapshot?.takeoverActive ? 'danger' : 'primary'}
            onClick={() => void toggleTakeover()}
            loading={saving}
            disabled={loading || !snapshot?.pluginDiscovered}
          >
            {snapshot?.takeoverActive
              ? t('proxy_pool.stop_takeover', { defaultValue: 'Stop takeover' })
              : t('proxy_pool.start_takeover', { defaultValue: 'Start takeover' })}
          </Button>
        </div>
      </header>

      {loadError && <div className={styles.errorBanner}>{loadError}</div>}
      {!loading && snapshot && !snapshot.pluginDiscovered && (
        <div className={styles.errorBanner}>
          {t('proxy_pool.plugin_missing', { defaultValue: 'Bundled proxy-pool plugin was not found. Check release packaging.' })}
        </div>
      )}
      {!loading && snapshot?.pluginDiscovered && !snapshot.pluginRegistered && (
        <div className={styles.errorBanner}>
          {t('proxy_pool.plugin_not_registered', {
            defaultValue: 'The proxy-pool plugin was discovered but did not start. Check its configuration, listener port, and Core logs.',
          })}
        </div>
      )}
      {snapshot?.status?.lastError && (
        <div className={styles.errorBanner}>
          <strong>{t('proxy_pool.runtime_error', { defaultValue: 'Last runtime error' })}</strong>
          <code>{snapshot.status.lastError}</code>
        </div>
      )}

      {!snapshot && (
        <div className={styles.noticeCard}>
          <strong>
            {loading
              ? t('proxy_pool.loading', { defaultValue: 'Loading proxy pool...' })
              : t('proxy_pool.load_unavailable', { defaultValue: 'Proxy pool data is unavailable' })}
          </strong>
          <p>
            {loading
              ? t('proxy_pool.loading_hint', { defaultValue: 'Reading plugin configuration and runtime status.' })
              : t('proxy_pool.load_unavailable_hint', { defaultValue: 'Fix the connection error above, then retry. No configuration can be changed while the current state is unknown.' })}
          </p>
          {!loading && (
            <Button variant="secondary" onClick={() => void load()}>
              {t('proxy_pool.retry_load', { defaultValue: 'Retry loading' })}
            </Button>
          )}
        </div>
      )}

      {snapshot?.pluginDiscovered && <><section className={styles.metrics}>
        <MetricCard
          label={t('proxy_pool.takeover', { defaultValue: 'Global takeover' })}
          value={snapshot?.takeoverActive ? t('proxy_pool.enabled', { defaultValue: 'Enabled' }) : t('proxy_pool.disabled', { defaultValue: 'Disabled' })}
          hint={snapshot?.globalProxyUrl || t('proxy_pool.no_global_proxy', { defaultValue: 'No global proxy' })}
        />
        <MetricCard
          label={t('proxy_pool.listener', { defaultValue: 'Internal listener' })}
          value={snapshot?.status?.ready ? t('proxy_pool.ready', { defaultValue: 'Ready' }) : t('proxy_pool.not_ready', { defaultValue: 'Not ready' })}
          hint={snapshot?.status?.proxyUrl || `socks5://${draft.listen}`}
        />
        <MetricCard label={t('proxy_pool.healthy_nodes', { defaultValue: 'Healthy nodes' })} value={`${snapshot?.status?.healthyNodes ?? 0}/${snapshot?.status?.totalNodes ?? draft.nodes.length}`} />
        <MetricCard label={t('proxy_pool.active_tunnels', { defaultValue: 'Active tunnels' })} value={snapshot?.status?.activeTunnels ?? 0} hint={t('proxy_pool.tunnel_scope', { defaultValue: 'Rotation occurs per TCP tunnel' })} />
      </section>

      {(snapshot?.bypassCredentials.length ?? 0) > 0 && (
        <details className={styles.warningCard}>
          <summary>
            {t('proxy_pool.bypass_warning', {
              defaultValue: '{{count}} credentials use their own proxy and bypass this pool',
              count: snapshot?.bypassCredentials.length ?? 0,
            })}
          </summary>
          <div className={styles.bypassList}>
            {snapshot?.bypassCredentials.map((item) => (
              <div key={`${item.name}-${item.proxyUrl}`}>
                <strong>{item.name}</strong>
                <span>{item.provider || '-'}</span>
                <code>{maskProxyCredentials(item.proxyUrl)}</code>
              </div>
            ))}
          </div>
        </details>
      )}

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <div>
            <h2>{t('proxy_pool.settings', { defaultValue: 'Pool settings' })}</h2>
            <p>{t('proxy_pool.settings_hint', { defaultValue: 'The listener is loopback-only. Fail-open stays off by default to prevent direct traffic leakage.' })}</p>
          </div>
          <span className={dirty ? styles.dirtyBadge : styles.savedBadge}>
            {dirty ? t('proxy_pool.unsaved', { defaultValue: 'Unsaved' }) : t('proxy_pool.saved', { defaultValue: 'Saved' })}
          </span>
        </div>
        <div className={styles.formGrid}>
          <Input label={t('proxy_pool.listen_address', { defaultValue: 'Listen address' })} value={draft.listen} onChange={(event) => updateDraft((current) => ({ ...current, listen: event.target.value }))} placeholder="127.0.0.1:8318" />
          <div className="form-group">
            <label>{t('proxy_pool.strategy', { defaultValue: 'Selection strategy' })}</label>
            <Select
              value={draft.strategy}
              onChange={(value) => updateDraft((current) => ({ ...current, strategy: value as ProxyPoolConfig['strategy'] }))}
              options={[
                { value: 'round-robin', label: t('proxy_pool.strategy_round_robin', { defaultValue: 'Round robin' }) },
                { value: 'weighted', label: t('proxy_pool.strategy_weighted', { defaultValue: 'Weighted' }) },
                { value: 'least-connections', label: t('proxy_pool.strategy_least_connections', { defaultValue: 'Least connections' }) },
              ]}
            />
          </div>
          <Input label={t('proxy_pool.dial_timeout', { defaultValue: 'Dial timeout' })} value={draft.dialTimeout} onChange={(event) => updateDraft((current) => ({ ...current, dialTimeout: event.target.value }))} placeholder="8s" />
          <Input type="number" min={1} label={t('proxy_pool.failover_attempts', { defaultValue: 'Max failover attempts' })} value={draft.maxFailoverAttempts} onChange={(event) => updateDraft((current) => ({ ...current, maxFailoverAttempts: Math.max(1, Number(event.target.value) || 1) }))} />
          <Input label={t('proxy_pool.health_interval', { defaultValue: 'Health-check interval' })} value={draft.healthCheck.interval} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, interval: event.target.value } }))} placeholder="30s" />
          <Input label={t('proxy_pool.health_timeout', { defaultValue: 'Health-check timeout' })} value={draft.healthCheck.timeout} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, timeout: event.target.value } }))} placeholder="8s" />
          <Input type="number" min={1} label={t('proxy_pool.isolation_threshold', { defaultValue: 'Failures before isolation' })} value={draft.healthCheck.isolationThreshold} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, isolationThreshold: Math.max(1, Number(event.target.value) || 1) } }))} />
          <Input label={t('proxy_pool.isolation_duration', { defaultValue: 'Isolation duration' })} value={draft.healthCheck.isolationDuration} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, isolationDuration: event.target.value } }))} placeholder="5m" />
          <Input label={t('proxy_pool.probe_target', { defaultValue: 'TCP probe target' })} value={draft.healthCheck.probeAddress} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, probeAddress: event.target.value } }))} placeholder="www.gstatic.com:443" />
          <Input label={t('proxy_pool.test_url', { defaultValue: 'Exit-IP test URL' })} value={draft.healthCheck.testUrl} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, testUrl: event.target.value } }))} placeholder="https://ipwho.is/" />
        </div>
        <div className={styles.switchRow}>
          <ToggleSwitch checked={draft.healthCheck.enabled} onChange={(enabled) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, enabled } }))} label={t('proxy_pool.health_checks', { defaultValue: 'Background health checks' })} />
          <ToggleSwitch checked={draft.failOpen} onChange={(failOpen) => updateDraft((current) => ({ ...current, failOpen }))} label={t('proxy_pool.fail_open', { defaultValue: 'Allow direct fallback (traffic leak risk)' })} />
        </div>
        {snapshot?.status && (
          <div className={styles.runtimeMeta}>
            <span>{t('proxy_pool.generation', { defaultValue: 'Generation' })}: <strong>{snapshot.status.generation}</strong></span>
            <span>{t('proxy_pool.last_applied', { defaultValue: 'Last applied' })}: <strong>{formatTime(snapshot.status.lastAppliedAt, i18n.language)}</strong></span>
            <span>{t('proxy_pool.last_health_cycle', { defaultValue: 'Last health cycle' })}: <strong>{formatTime(snapshot.status.lastHealthAt, i18n.language)}</strong></span>
          </div>
        )}
      </section>

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <div>
            <h2>{t('proxy_pool.nodes', { defaultValue: 'Proxy nodes' })}</h2>
            <p>{t('proxy_pool.nodes_hint', { defaultValue: 'Supports HTTP, HTTPS, SOCKS5, and SOCKS5H proxy URLs.' })}</p>
          </div>
          <div className={styles.nodeToolbarActions}>
            <Button variant="ghost" size="sm" onClick={() => setImportOpen((current) => !current)}>
              {t('proxy_pool.batch_import', { defaultValue: 'Batch import' })}
            </Button>
            <Button variant="secondary" size="sm" onClick={() => updateDraft((current) => ({ ...current, nodes: [...current.nodes, createNode(current.nodes.length)] }))}>
              {t('proxy_pool.add_node', { defaultValue: 'Add node' })}
            </Button>
          </div>
        </div>

        {importOpen && (
          <div className={styles.importPanel}>
            <label htmlFor="proxy-pool-import">
              {t('proxy_pool.import_label', { defaultValue: 'Paste proxy nodes' })}
            </label>
            <p>{t('proxy_pool.import_hint', { defaultValue: 'One URL per line, or: label | URL | weight. Blank lines, comments, and duplicates are ignored.' })}</p>
            <textarea
              id="proxy-pool-import"
              value={importText}
              onChange={(event) => setImportText(event.target.value)}
              placeholder={'socks5://user:pass@host:1080\nPrimary | http://host:8080 | 3'}
              rows={7}
            />
            <div>
              <Button variant="ghost" size="sm" onClick={() => { setImportOpen(false); setImportText(''); }}>
                {t('common.cancel')}
              </Button>
              <Button size="sm" onClick={importNodes} disabled={!importText.trim()}>
                {t('proxy_pool.import_nodes', { defaultValue: 'Import nodes' })}
              </Button>
            </div>
          </div>
        )}

        <div className={styles.nodeToolbar}>
          <Input
            value={nodeQuery}
            onChange={(event) => setNodeQuery(event.target.value)}
            placeholder={t('proxy_pool.search_nodes', { defaultValue: 'Search ID, label, or proxy URL' })}
          />
          <span>{t('proxy_pool.visible_nodes', { defaultValue: '{{visible}} of {{total}} nodes', visible: visibleNodeIndexes.length, total: draft.nodes.length })}</span>
          <Button variant="ghost" size="sm" disabled={visibleNodeIndexes.length === 0} onClick={() => updateDraft((current) => ({ ...current, nodes: current.nodes.map((node, index) => visibleNodeIndexes.some((item) => item.index === index) ? { ...node, enabled: true } : node) }))}>
            {t('proxy_pool.enable_visible', { defaultValue: 'Enable visible' })}
          </Button>
          <Button variant="ghost" size="sm" disabled={visibleNodeIndexes.length === 0} onClick={() => updateDraft((current) => ({ ...current, nodes: current.nodes.map((node, index) => visibleNodeIndexes.some((item) => item.index === index) ? { ...node, enabled: false } : node) }))}>
            {t('proxy_pool.disable_visible', { defaultValue: 'Disable visible' })}
          </Button>
        </div>

        <div className={styles.nodeList}>
          {draft.nodes.length === 0 && <div className={styles.emptyState}>{t('proxy_pool.no_nodes', { defaultValue: 'No proxy nodes configured yet.' })}</div>}
          {draft.nodes.length > 0 && visibleNodeIndexes.length === 0 && <div className={styles.emptyState}>{t('proxy_pool.no_matching_nodes', { defaultValue: 'No proxy nodes match the current search.' })}</div>}
          {visibleNodeIndexes.map(({ node, index }) => {
            const runtime = statusByID.get(node.id);
            const testKey = node.id.trim() || `draft-${index + 1}`;
            const probe = probeResults[testKey];
            const runtimeState = runtime?.state ?? 'unknown';
            return (
              <article key={index} className={styles.nodeCard}>
                <div className={styles.nodeTopline}>
                  <ToggleSwitch checked={node.enabled} onChange={(enabled) => updateNode(index, { enabled })} ariaLabel={t('proxy_pool.node_enabled_aria', { defaultValue: 'Enable node {{id}}', id: node.id })} />
                  <span className={`${styles.stateBadge} ${stateTone(runtimeState)}`}>
                    {t(`proxy_pool.state_${runtimeState}`, { defaultValue: runtimeState })}
                  </span>
                  <strong>{node.label || node.id}</strong>
                  <div className={styles.nodeActions}>
                    {runtimeState === 'isolated' && <Button variant="ghost" size="sm" disabled={dirty || recoveringNode === node.id} loading={recoveringNode === node.id} onClick={() => void recoverNode(node.id)}>{t('proxy_pool.recover', { defaultValue: 'Recover' })}</Button>}
                    <Button variant="ghost" size="sm" disabled={!node.url.trim() || testingNode === testKey} loading={testingNode === testKey} onClick={() => void testNode(node, index)}>{t('proxy_pool.test', { defaultValue: 'Test' })}</Button>
                    <Button variant="ghost" size="sm" onClick={() => updateDraft((current) => { const copy = { ...node, id: createNode(current.nodes.length).id, label: node.label ? `${node.label} copy` : '', order: (current.nodes.length + 1) * 10 }; return { ...current, nodes: [...current.nodes.slice(0, index + 1), copy, ...current.nodes.slice(index + 1)] }; })}>{t('proxy_pool.duplicate', { defaultValue: 'Duplicate' })}</Button>
                    <Button variant="danger" size="sm" onClick={() => updateDraft((current) => ({ ...current, nodes: current.nodes.filter((_, nodeIndex) => nodeIndex !== index) }))}>{t('common.delete')}</Button>
                  </div>
                </div>
                <div className={styles.nodeForm}>
                  <Input label="ID" value={node.id} onChange={(event) => updateNode(index, { id: event.target.value })} />
                  <Input label={t('proxy_pool.node_label', { defaultValue: 'Label' })} value={node.label} onChange={(event) => updateNode(index, { label: event.target.value })} />
                  <Input className={styles.urlInput} label={t('proxy_pool.proxy_url', { defaultValue: 'Proxy URL' })} value={node.url} onChange={(event) => updateNode(index, { url: event.target.value })} placeholder="socks5://user:pass@host:1080" />
                  <Input type="number" min={1} label={t('proxy_pool.weight', { defaultValue: 'Weight' })} value={node.weight} onChange={(event) => updateNode(index, { weight: Math.max(1, Number(event.target.value) || 1) })} />
                  <Input type="number" label={t('proxy_pool.order', { defaultValue: 'Order' })} value={node.order} onChange={(event) => updateNode(index, { order: Number(event.target.value) || (index + 1) * 10 })} />
                </div>
                <div className={styles.nodeRuntime}>
                  <span>{t('proxy_pool.latency', { defaultValue: 'Latency' })}: <strong>{runtime?.latencyMs ? `${runtime.latencyMs} ms` : '-'}</strong></span>
                  <span>{t('proxy_pool.exit_ip', { defaultValue: 'Exit IP' })}: <strong>{probe?.exitIp || runtime?.exitIp || '-'}</strong></span>
                  <span>{t('proxy_pool.location', { defaultValue: 'Location' })}: <strong>{probe?.location || runtime?.location || '-'}</strong></span>
                  <span>{t('proxy_pool.connections', { defaultValue: 'Connections' })}: <strong>{runtime?.successConnects ?? 0}/{runtime?.totalConnects ?? 0}</strong></span>
                  <span>{t('proxy_pool.success_rate', { defaultValue: 'Success rate' })}: <strong>{formatSuccessRate(runtime?.successConnects ?? 0, runtime?.totalConnects ?? 0)}</strong></span>
                  <span>{t('proxy_pool.failed_connections', { defaultValue: 'Failures' })}: <strong>{runtime?.failedConnects ?? 0}</strong></span>
                  <span>{t('proxy_pool.node_active_tunnels', { defaultValue: 'Active' })}: <strong>{runtime?.activeTunnels ?? 0}</strong></span>
                  <span>{t('proxy_pool.last_check', { defaultValue: 'Last check' })}: <strong>{formatTime(runtime?.lastCheck ?? probe?.checkedAt ?? '', i18n.language)}</strong></span>
                  <span>{t('proxy_pool.last_success', { defaultValue: 'Last success' })}: <strong>{formatTime(runtime?.lastSuccess ?? '', i18n.language)}</strong></span>
                  <span>{t('proxy_pool.last_failure', { defaultValue: 'Last failure' })}: <strong>{formatTime(runtime?.lastFailure ?? '', i18n.language)}</strong></span>
                </div>
                {(probe?.error || runtime?.lastError) && <div className={styles.nodeError}>{probe?.error || runtime?.lastError}</div>}
              </article>
            );
          })}
        </div>
      </section>

      {actionBarTarget && actionBar ? createPortal(actionBar, actionBarTarget) : actionBar}</>}
    </div>
  );
}
