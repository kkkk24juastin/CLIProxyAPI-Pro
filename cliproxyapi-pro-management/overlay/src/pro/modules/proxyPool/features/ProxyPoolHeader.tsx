import { useTranslation } from 'react-i18next';
import { IconShield } from '@/components/ui/icons';
import type { ProxyPoolSnapshot } from '@/pro/modules/proxyPool/proxyPool';
import { ProFeatureHeader } from '@/pro/shared/ProFeatureHeader';

interface ProxyPoolHeaderProps {
  snapshot: ProxyPoolSnapshot | null;
  loading: boolean;
  busy: boolean;
  onRefresh: () => void;
  onTakeover: () => void;
}

export function ProxyPoolHeader({
  snapshot,
  loading,
  busy,
  onRefresh,
  onTakeover,
}: ProxyPoolHeaderProps) {
  const { t } = useTranslation();
  const active = snapshot?.takeoverActive === true;

  return (
    <ProFeatureHeader
      title={t('proxy_pool.title', { defaultValue: 'Proxy Management' })}
      subtitle={t('proxy_pool.subtitle_compact', {
        defaultValue: 'A stable local SOCKS5 endpoint backed by multiple managed proxies.',
      })}
      icon={<IconShield size={20} />}
      active={active}
      loading={loading}
      actionBusy={busy}
      actionDisabled={!snapshot}
      onRefresh={onRefresh}
      onToggle={onTakeover}
    />
  );
}
