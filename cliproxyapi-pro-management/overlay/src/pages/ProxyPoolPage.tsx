import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Select } from '@/components/ui/Select';
import { ToggleSwitch } from '@/components/ui/ToggleSwitch';
import { IconRefreshCw } from '@/components/ui/icons';
import {
  defaultProxyPoolConfig,
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

const validateConfig = (config: ProxyPoolConfig): string => {
  if (!/^(127(?:\.\d{1,3}){3}|\[?::1\]?):\d{1,5}$/.test(config.listen.trim())) {
    return 'Listener must be a numeric loopback address with a port';
  }
  const ids = new Set<string>();
  const urls = new Set<string>();
  for (const node of config.nodes) {
    const id = node.id.trim();
    const url = node.url.trim();
    if (!id || !url) return 'Every node requires an ID and proxy URL';
    if (ids.has(id)) return `Duplicate node ID: ${id}`;
    if (urls.has(url)) return `Duplicate proxy URL: ${url}`;
    ids.add(id);
    urls.add(url);
    if (!/^(https?|socks5h?):\/\//i.test(url)) return `Unsupported proxy URL: ${url}`;
  }
  return '';
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
  const [dirty, setDirty] = useState(false);
  const [loadError, setLoadError] = useState('');
  const [probeResults, setProbeResults] = useState<Record<string, ProxyPoolProbeResult>>({});

  const load = useCallback(async (silent = false) => {
    if (connectionStatus !== 'connected' || !supportsPlugin) {
      setLoading(false);
      return;
    }
    if (!silent) setLoading(true);
    try {
      const next = await proxyPoolApi.load();
      setSnapshot(next);
      if (!dirty) setDraft(next.config);
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
    const validationError = validateConfig(draft);
    if (validationError) {
      showNotification(validationError, 'error');
      return false;
    }
    setSaving(true);
    try {
      await proxyPoolApi.save(draft);
      setDirty(false);
      showNotification(t('proxy_pool.save_success', { defaultValue: 'Proxy pool saved' }), 'success');
      await load(true);
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
    const validationError = validateConfig(draft);
    if (activating && validationError) {
      showNotification(validationError, 'error');
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
      await load(true);
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

  const testNode = async (nodeId: string) => {
    setTestingNode(nodeId);
    try {
      const result = await proxyPoolApi.testNode(nodeId);
      setProbeResults((current) => ({ ...current, [nodeId]: result }));
      showNotification(
        result.success
          ? `${result.exitIp || nodeId} · ${result.latencyMs} ms`
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
      const results = await proxyPoolApi.testAll();
      setProbeResults(Object.fromEntries(results.map((result) => [result.nodeId, result])));
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

      <section className={styles.metrics}>
        <MetricCard
          label={t('proxy_pool.takeover', { defaultValue: 'Global takeover' })}
          value={snapshot?.takeoverActive ? t('common.enabled', { defaultValue: 'Enabled' }) : t('common.disabled', { defaultValue: 'Disabled' })}
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
              <div key={item.name}>
                <strong>{item.name}</strong>
                <span>{item.provider || '-'}</span>
                <code>{item.proxyUrl}</code>
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
            {dirty ? t('common.unsaved_changes', { defaultValue: 'Unsaved' }) : t('common.saved', { defaultValue: 'Saved' })}
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
          <Input type="number" min={1} label={t('proxy_pool.isolation_threshold', { defaultValue: 'Failures before isolation' })} value={draft.healthCheck.isolationThreshold} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, isolationThreshold: Math.max(1, Number(event.target.value) || 1) } }))} />
          <Input label={t('proxy_pool.isolation_duration', { defaultValue: 'Isolation duration' })} value={draft.healthCheck.isolationDuration} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, isolationDuration: event.target.value } }))} placeholder="5m" />
          <Input label={t('proxy_pool.probe_target', { defaultValue: 'TCP probe target' })} value={draft.healthCheck.probeAddress} onChange={(event) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, probeAddress: event.target.value } }))} placeholder="www.gstatic.com:443" />
        </div>
        <div className={styles.switchRow}>
          <ToggleSwitch checked={draft.healthCheck.enabled} onChange={(enabled) => updateDraft((current) => ({ ...current, healthCheck: { ...current.healthCheck, enabled } }))} label={t('proxy_pool.health_checks', { defaultValue: 'Background health checks' })} />
          <ToggleSwitch checked={draft.failOpen} onChange={(failOpen) => updateDraft((current) => ({ ...current, failOpen }))} label={t('proxy_pool.fail_open', { defaultValue: 'Allow direct fallback (traffic leak risk)' })} />
        </div>
      </section>

      <section className={styles.panel}>
        <div className={styles.panelHeader}>
          <div>
            <h2>{t('proxy_pool.nodes', { defaultValue: 'Proxy nodes' })}</h2>
            <p>{t('proxy_pool.nodes_hint', { defaultValue: 'Supports HTTP, HTTPS, SOCKS5, and SOCKS5H proxy URLs.' })}</p>
          </div>
          <Button variant="secondary" size="sm" onClick={() => updateDraft((current) => ({ ...current, nodes: [...current.nodes, createNode(current.nodes.length)] }))}>
            {t('proxy_pool.add_node', { defaultValue: 'Add node' })}
          </Button>
        </div>

        <div className={styles.nodeList}>
          {draft.nodes.length === 0 && <div className={styles.emptyState}>{t('proxy_pool.no_nodes', { defaultValue: 'No proxy nodes configured yet.' })}</div>}
          {draft.nodes.map((node, index) => {
            const runtime = statusByID.get(node.id);
            const probe = probeResults[node.id];
            return (
              <article key={`${node.id}-${index}`} className={styles.nodeCard}>
                <div className={styles.nodeTopline}>
                  <ToggleSwitch checked={node.enabled} onChange={(enabled) => updateNode(index, { enabled })} ariaLabel={`${node.id} enabled`} />
                  <span className={`${styles.stateBadge} ${stateTone(runtime?.state ?? 'unknown')}`}>{runtime?.state ?? 'unknown'}</span>
                  <strong>{node.label || node.id}</strong>
                  <div className={styles.nodeActions}>
                    <Button variant="ghost" size="sm" disabled={dirty || !runtime || testingNode === node.id} loading={testingNode === node.id} onClick={() => void testNode(node.id)}>{t('proxy_pool.test', { defaultValue: 'Test' })}</Button>
                    <Button variant="danger" size="sm" onClick={() => updateDraft((current) => ({ ...current, nodes: current.nodes.filter((_, nodeIndex) => nodeIndex !== index) }))}>{t('common.delete')}</Button>
                  </div>
                </div>
                <div className={styles.nodeForm}>
                  <Input label="ID" value={node.id} onChange={(event) => updateNode(index, { id: event.target.value })} />
                  <Input label={t('proxy_pool.node_label', { defaultValue: 'Label' })} value={node.label} onChange={(event) => updateNode(index, { label: event.target.value })} />
                  <Input className={styles.urlInput} label={t('proxy_pool.proxy_url', { defaultValue: 'Proxy URL' })} value={node.url} onChange={(event) => updateNode(index, { url: event.target.value })} placeholder="socks5://user:pass@host:1080" />
                  <Input type="number" min={1} label={t('proxy_pool.weight', { defaultValue: 'Weight' })} value={node.weight} onChange={(event) => updateNode(index, { weight: Math.max(1, Number(event.target.value) || 1) })} />
                </div>
                <div className={styles.nodeRuntime}>
                  <span>{t('proxy_pool.latency', { defaultValue: 'Latency' })}: <strong>{runtime?.latencyMs ? `${runtime.latencyMs} ms` : '-'}</strong></span>
                  <span>{t('proxy_pool.exit_ip', { defaultValue: 'Exit IP' })}: <strong>{probe?.exitIp || runtime?.exitIp || '-'}</strong></span>
                  <span>{t('proxy_pool.location', { defaultValue: 'Location' })}: <strong>{probe?.location || runtime?.location || '-'}</strong></span>
                  <span>{t('proxy_pool.connections', { defaultValue: 'Connections' })}: <strong>{runtime?.successConnects ?? 0}/{runtime?.totalConnects ?? 0}</strong></span>
                  <span>{t('proxy_pool.last_check', { defaultValue: 'Last check' })}: <strong>{formatTime(runtime?.lastCheck ?? probe?.checkedAt ?? '', i18n.language)}</strong></span>
                </div>
                {(probe?.error || runtime?.lastError) && <div className={styles.nodeError}>{probe?.error || runtime?.lastError}</div>}
              </article>
            );
          })}
        </div>
      </section>

      <footer className={styles.actionBar}>
        <div>
          <code>{snapshot?.status?.proxyUrl || `socks5://${draft.listen}`}</code>
          <span>{t('proxy_pool.fixed_endpoint_hint', { defaultValue: 'Core only sees this fixed proxy endpoint.' })}</span>
        </div>
        <div>
          <Button variant="ghost" onClick={() => void resetStats()} disabled={!snapshot?.status?.ready || saving}>{t('proxy_pool.reset_stats', { defaultValue: 'Reset stats' })}</Button>
          <Button variant="secondary" onClick={() => void testAll()} loading={testingAll} disabled={dirty || !snapshot?.status?.ready}>{t('proxy_pool.test_all', { defaultValue: 'Test all' })}</Button>
          <Button onClick={() => void save()} loading={saving} disabled={!dirty}>{t('common.save')}</Button>
        </div>
      </footer>
    </div>
  );
}
