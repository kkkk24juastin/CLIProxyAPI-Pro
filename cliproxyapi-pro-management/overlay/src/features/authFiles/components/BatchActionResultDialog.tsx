import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import type { BatchActionSummary } from '@/features/authFiles/hooks/useAuthFilesBatchActions';
import styles from '@/pages/AuthFilesPage.module.scss';

export type BatchActionResultDialogProps = {
  open: boolean;
  title: string;
  summary: BatchActionSummary | null;
  onClose: () => void;
};

export function BatchActionResultDialog({
  open,
  title,
  summary,
  onClose,
}: BatchActionResultDialogProps) {
  const { t } = useTranslation();

  if (!summary) {
    return null;
  }

  const rows = summary.results;
  const hasFailures = summary.failed > 0 || summary.skipped > 0;

  return (
    <Modal open={open} onClose={onClose} title={title} width={720}>
      <div className={styles.batchResultSummary}>
        <span className={styles.batchResultSuccess}>
          {t('auth_files.batch_result_success', { count: summary.success })}
        </span>
        {summary.failed > 0 && (
          <span className={styles.batchResultFailed}>
            {t('auth_files.batch_result_failed', { count: summary.failed })}
          </span>
        )}
        {summary.skipped > 0 && (
          <span className={styles.batchResultSkipped}>
            {t('auth_files.batch_result_skipped', { count: summary.skipped })}
          </span>
        )}
        <span className={styles.batchResultTotal}>
          {t('auth_files.batch_result_total', { count: summary.total })}
        </span>
      </div>

      {rows.length > 0 && (
        <div className={styles.batchResultTableWrapper}>
          <table className={styles.batchResultTable}>
            <thead>
              <tr>
                <th>{t('auth_files.batch_result_col_name')}</th>
                <th>{t('auth_files.batch_result_col_provider')}</th>
                <th>{t('auth_files.batch_result_col_result')}</th>
                <th>{t('auth_files.batch_result_col_error')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((item, idx) => (
                <tr key={`${item.name}-${idx}`}>
                  <td className={styles.batchResultCellName}>{item.name}</td>
                  <td>{item.provider}</td>
                  <td>
                    {item.skipped ? (
                      <span className={styles.batchResultBadgeSkipped}>
                        {t('auth_files.batch_result_badge_skipped')}
                      </span>
                    ) : item.ok ? (
                      <span className={styles.batchResultBadgeSuccess}>
                        {t('auth_files.batch_result_badge_success')}
                      </span>
                    ) : (
                      <span className={styles.batchResultBadgeFailed}>
                        {t('auth_files.batch_result_badge_failed')}
                      </span>
                    )}
                  </td>
                  <td className={styles.batchResultCellError}>
                    {item.error || (hasFailures ? '' : '—')}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className={styles.batchResultActions}>
        <button type="button" className={styles.batchResultCloseButton} onClick={onClose}>
          {t('common.close')}
        </button>
      </div>
    </Modal>
  );
}
