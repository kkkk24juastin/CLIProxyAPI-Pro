import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { ProTaskDialog } from '@/pro/shared/ProSurface';
import { IconAlertTriangle, IconCheckCircle2 } from '@/components/ui/icons';
import type { ProxyPoolConfig, ProxyPoolSnapshot } from '@/pro/modules/proxyPool/proxyPool';
import { maskProxyCredentials } from './proxyPoolUi';
import styles from './ProxyPool.module.scss';

interface ProxyPoolTakeoverDialogProps {
  open: boolean;
  snapshot: ProxyPoolSnapshot;
  draft: ProxyPoolConfig;
  busy: boolean;
  onClose: () => void;
  onConfirm: () => void;
}

export function ProxyPoolTakeoverDialog({
  open,
  snapshot,
  draft,
  busy,
  onClose,
  onConfirm,
}: ProxyPoolTakeoverDialogProps) {
  const { t } = useTranslation();
  const activating = !snapshot.takeoverActive;
  const readyNodes =
    snapshot.status?.eligibleNodes ??
    snapshot.status?.healthyNodes ??
    draft.nodes.filter((node) => node.enabled).length;
  const endpoint = snapshot.status?.proxyUrl || `socks5://${draft.listen}`;

  return (
    <ProTaskDialog
      open={open}
      onClose={onClose}
      closeDisabled={busy}
      title={
        activating
          ? t('proxy_pool.takeover_confirm_title', { defaultValue: 'Start global proxy takeover?' })
          : t('proxy_pool.stop_confirm_title', { defaultValue: 'Stop global proxy takeover?' })
      }
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </Button>
          <Button variant={activating ? 'primary' : 'danger'} onClick={onConfirm} loading={busy}>
            {activating
              ? t('proxy_pool.start_takeover', { defaultValue: 'Start takeover' })
              : t('proxy_pool.stop_takeover', { defaultValue: 'Stop takeover' })}
          </Button>
        </>
      }
    >
      <p className={styles.takeoverLead}>
        {activating
          ? t('proxy_pool.takeover_confirm_body', {
              defaultValue:
                'Core traffic will be routed through the built-in proxy pool at runtime. Review readiness before applying.',
            })
          : t('proxy_pool.stop_confirm_body', {
              defaultValue:
                'The runtime override will be removed; config.yaml remains unchanged.',
            })}
      </p>
      <dl className={styles.takeoverChecklist}>
        <div>
          <dt>{t('proxy_pool.listener_ready', { defaultValue: 'Runtime ready' })}</dt>
          <dd className={snapshot.status?.ready ? styles.checkGood : styles.checkBad}>
            {snapshot.status?.ready ? <IconCheckCircle2 size={16} /> : <IconAlertTriangle size={16} />}
            {snapshot.status?.ready ? t('common.yes') : t('common.no')}
          </dd>
        </div>
        <div>
          <dt>{t('proxy_pool.available_nodes', { defaultValue: 'Available nodes' })}</dt>
          <dd className={readyNodes > 0 ? styles.checkGood : styles.checkBad}>{readyNodes}</dd>
        </div>
        <div>
          <dt>{t('proxy_pool.internal_endpoint', { defaultValue: 'Internal endpoint' })}</dt>
          <dd>
            <code>{endpoint}</code>
          </dd>
        </div>
        <div>
          <dt>{t('proxy_pool.current_global_proxy', { defaultValue: 'Current global proxy' })}</dt>
          <dd>
            <code>
              {maskProxyCredentials(snapshot.globalProxyUrl) ||
                t('proxy_pool.none', { defaultValue: 'None' })}
            </code>
          </dd>
        </div>
        <div>
          <dt>
            {t('proxy_pool.bypass_credentials_count', {
              defaultValue: 'Credentials bypassing pool',
            })}
          </dt>
          <dd>{snapshot.bypassCredentials.length}</dd>
        </div>
      </dl>
      {draft.failOpen && (
        <div className={styles.riskBanner}>
          <IconAlertTriangle size={18} />
          <div>
            <strong>
              {t('proxy_pool.fail_open_enabled', { defaultValue: 'Direct fallback is enabled' })}
            </strong>
            <p>
              {t('proxy_pool.fail_open_risk', {
                defaultValue: 'Traffic may leave without a proxy when all nodes are unavailable.',
              })}
            </p>
          </div>
        </div>
      )}
    </ProTaskDialog>
  );
}
