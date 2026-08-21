import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { useNotificationStore } from '@/stores';
import { checkManagementPanelUpdate } from './managementUpdate';
import styles from '@/pages/SystemPage.module.scss';

export interface ManagementVersionTileExtensionProps {
  appVersion: string;
  onVersionTap: () => void;
}

export function ManagementVersionTileExtension({
  appVersion,
  onVersionTap,
}: ManagementVersionTileExtensionProps) {
  const { t } = useTranslation();
  const showNotification = useNotificationStore((state) => state.showNotification);
  const [checkingManagementUpdate, setCheckingManagementUpdate] = useState(false);

  const handleManagementUpdateCheck = async () => {
    setCheckingManagementUpdate(true);
    try {
      const result = await checkManagementPanelUpdate();
      if (result.updated) {
        showNotification(t('system_info.management_check_update_updated'), 'success');
        window.setTimeout(() => {
          const nextUrl = new URL(window.location.href);
          nextUrl.searchParams.set('_management_updated', Date.now().toString());
          window.location.replace(nextUrl.toString());
        }, 500);
      } else {
        showNotification(t('system_info.management_check_update_unchanged'), 'success');
      }
    } catch (error: unknown) {
      const message =
        error instanceof Error ? error.message : typeof error === 'string' ? error : '';
      showNotification(
        `${t('system_info.management_check_update_error')}${message ? `: ${message}` : ''}`,
        'error'
      );
    } finally {
      setCheckingManagementUpdate(false);
    }
  };

  return (
    <div className={`${styles.infoTile} ${styles.tapTile}`} onClick={onVersionTap}>
      <div className={styles.tileHeader}>
        <div className={styles.tileLabel}>{t('footer.version')}</div>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={styles.tileAction}
          onClick={(event) => {
            event.stopPropagation();
            void handleManagementUpdateCheck();
          }}
          loading={checkingManagementUpdate}
          title={t('system_info.management_check_update_button')}
          aria-label={t('system_info.management_check_update_button')}
        >
          {t('system_info.management_check_update_button')}
        </Button>
      </div>
      <div className={styles.tileValue}>{appVersion}</div>
    </div>
  );
}
