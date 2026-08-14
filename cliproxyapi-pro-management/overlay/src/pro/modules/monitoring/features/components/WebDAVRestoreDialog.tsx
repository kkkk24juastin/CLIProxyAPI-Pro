import { Button } from '@/components/ui/Button';
import { ProWorkspaceDialog } from '@/pro/shared/ProSurface';
import styles from '@/pro/modules/monitoring/features/monitoring.module.scss';

export type WebDAVBackup = {
  fileName: string;
  sizeBytes: number;
  lastModified?: string;
  lastModifiedMs?: number;
};

type Props = {
  open: boolean;
  loading: boolean;
  restoring: boolean;
  backups: WebDAVBackup[];
  onClose: () => void;
  onRefresh: () => void;
  onSelect: (backup: WebDAVBackup) => void;
  t: (key: string, options?: Record<string, unknown>) => string;
};

const formatBackupSize = (bytes: number) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '—';
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
};

export function WebDAVRestoreDialog({ open, loading, restoring, backups, onClose, onRefresh, onSelect, t }: Props) {
  return (
    <ProWorkspaceDialog
      open={open}
      onClose={onClose}
      closeDisabled={loading || restoring}
      title={t('usage_stats.webdav_restore_title')}
      footer={(
        <div className={styles.monitorModalActions}>
          <Button variant="secondary" size="sm" onClick={onRefresh} disabled={loading || restoring}>
            {t('common.refresh')}
          </Button>
          <Button variant="primary" size="sm" onClick={onClose} disabled={loading || restoring}>
            {t('common.close')}
          </Button>
        </div>
      )}
    >
      <div className={styles.webdavRestoreDialog}>
        <p className={styles.settingsHint}>{t('usage_stats.webdav_restore_description')}</p>
        <p className={styles.webdavRestoreWarning}>{t('usage_stats.import_policy_no_api_keys')}</p>
        {loading ? (
          <div className={styles.surfaceLoadingStatus}>{t('common.loading')}</div>
        ) : backups.length === 0 ? (
          <div className={styles.webdavRestoreEmpty}>{t('usage_stats.webdav_restore_empty')}</div>
        ) : (
          <div className={styles.webdavBackupList}>
            {backups.map((backup) => (
              <button
                type="button"
                className={styles.webdavBackupRow}
                key={backup.fileName}
                onClick={() => onSelect(backup)}
                disabled={restoring}
              >
                <span>
                  <strong>{backup.fileName}</strong>
                  <small>
                    {backup.lastModifiedMs
                      ? new Date(backup.lastModifiedMs).toLocaleString()
                      : backup.lastModified || t('common.unknown')}
                  </small>
                </span>
                <span>{formatBackupSize(backup.sizeBytes)}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </ProWorkspaceDialog>
  );
}
