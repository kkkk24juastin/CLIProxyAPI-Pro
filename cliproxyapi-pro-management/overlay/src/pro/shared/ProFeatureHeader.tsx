import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { IconRefreshCw } from '@/components/ui/icons';
import styles from './ProFeatureHeader.module.scss';

interface ProFeatureHeaderProps {
  title: ReactNode;
  subtitle: ReactNode;
  icon: ReactNode;
  active: boolean;
  loading: boolean;
  actionBusy: boolean;
  actionDisabled?: boolean;
  onRefresh: () => void;
  onToggle: () => void;
}

export function ProFeatureHeader({
  title,
  subtitle,
  icon,
  active,
  loading,
  actionBusy,
  actionDisabled = false,
  onRefresh,
  onToggle,
}: ProFeatureHeaderProps) {
  const { t } = useTranslation();

  return (
    <header className={styles.header} data-pro-feature-header>
      <div className={styles.identity}>
        <span className={`${styles.icon} ${active ? styles.iconActive : ''}`}>{icon}</span>
        <div className={styles.copy}>
          <div className={styles.titleLine}>
            <h1>{title}</h1>
            <span className={`${styles.status} ${active ? styles.statusActive : ''}`}>
              <span aria-hidden="true" />
              {active
                ? t('pro_feature_header.active', { defaultValue: 'Taking over' })
                : t('pro_feature_header.inactive', { defaultValue: 'Not taking over' })}
            </span>
          </div>
          <p>{subtitle}</p>
        </div>
      </div>
      <div className={styles.actions}>
        <Button
          variant="ghost"
          size="sm"
          onClick={onRefresh}
          disabled={loading || actionBusy}
          aria-label={t('common.refresh')}
        >
          <IconRefreshCw size={16} />
          {t('common.refresh')}
        </Button>
        <Button
          variant={active ? 'danger' : 'primary'}
          size="sm"
          onClick={onToggle}
          loading={actionBusy}
          disabled={loading || actionBusy || actionDisabled}
        >
          {active
            ? t('pro_feature_header.stop_takeover', { defaultValue: 'Stop takeover' })
            : t('pro_feature_header.start_takeover', { defaultValue: 'Start takeover' })}
        </Button>
      </div>
    </header>
  );
}
