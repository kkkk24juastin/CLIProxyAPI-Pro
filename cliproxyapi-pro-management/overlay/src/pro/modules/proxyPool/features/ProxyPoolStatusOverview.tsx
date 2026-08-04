import { useTranslation } from 'react-i18next';
import {
  IconAlertTriangle,
  IconCheckCircle2,
  IconNetwork,
  IconShield,
} from '@/components/ui/icons';
import type { ProxyPoolConfig, ProxyPoolSnapshot } from '@/pro/modules/proxyPool/proxyPool';
import styles from './ProxyPool.module.scss';

interface ProxyPoolStatusOverviewProps {
  snapshot: ProxyPoolSnapshot;
  draft: ProxyPoolConfig;
}

export function ProxyPoolStatusOverview({ snapshot, draft }: ProxyPoolStatusOverviewProps) {
  const { t } = useTranslation();
  const status = snapshot.status;
  const healthy = status?.healthyNodes ?? 0;
  const total = status?.totalNodes ?? draft.nodes.length;
  const endpoint = status?.proxyUrl || `socks5://${draft.listen}`;
  const ready = status?.ready === true;

  return (
    <section
      className={styles.statusOverview}
      aria-label={t('proxy_pool.health_overview', { defaultValue: 'Proxy pool health overview' })}
    >
      <div className={styles.overviewItem}>
        <span className={ready ? styles.overviewGood : styles.overviewBad}>
          {ready ? <IconCheckCircle2 size={18} /> : <IconAlertTriangle size={18} />}
        </span>
        <div>
          <small>{t('proxy_pool.runtime_state', { defaultValue: 'Runtime' })}</small>
          <strong>
            {ready
              ? t('proxy_pool.ready', { defaultValue: 'Ready' })
              : t('proxy_pool.not_ready', { defaultValue: 'Not ready' })}
          </strong>
        </div>
      </div>
      <div className={styles.overviewItem}>
        <span className={status?.ready ? styles.overviewGood : styles.overviewMuted}>
          <IconNetwork size={18} />
        </span>
        <div>
          <small>{t('proxy_pool.fixed_endpoint', { defaultValue: 'Fixed endpoint' })}</small>
          <code title={endpoint}>{endpoint}</code>
        </div>
      </div>
      <div className={styles.overviewItem}>
        <span className={healthy > 0 ? styles.overviewGood : styles.overviewMuted}>
          <IconShield size={18} />
        </span>
        <div>
          <small>{t('proxy_pool.available_nodes', { defaultValue: 'Available nodes' })}</small>
          <strong>
            {healthy} / {total}
          </strong>
        </div>
      </div>
      <div className={styles.overviewItem}>
        <span className={styles.overviewMuted}>
          <IconNetwork size={18} />
        </span>
        <div>
          <small>{t('proxy_pool.active_tunnels', { defaultValue: 'Active tunnels' })}</small>
          <strong>{status?.activeTunnels ?? 0}</strong>
        </div>
      </div>
    </section>
  );
}
