import type { Dispatch, SetStateAction } from 'react';
import type { TFunction } from 'i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { IconTrash2 } from '@/components/ui/icons';
import { LoadingSpinner } from '@/components/ui/LoadingSpinner';
import { formatCompactNumber } from '@/pro/modules/monitoring/features/usage';
import { ProSettingsSheet } from '@/pro/shared/ProSurface';
import type { MonitoringSettingsDraft } from '../monitoringSettings';
import styles from '../monitoring.module.scss';

export function MonitoringSettingsModal({
  isMonitoringSettingsOpen,
  closeMonitoringSettings,
  confirmMonitoringSettingsClose,
  discardMonitoringSettingsDraft,
  monitoringSettingsDirty,
  monitoringSettingsDraft,
  setMonitoringSettingsDraft,
  usageTotalRequests,
  isMonitoringSettingsLoading,
  isMonitoringStatisticsResetting,
  isMonitoringSettingsSaving,
  handleMonitoringStatisticsReset,
  handleSaveMonitoringSettings,
  t,
}: {
  isMonitoringSettingsOpen: boolean;
  closeMonitoringSettings: () => void | boolean;
  confirmMonitoringSettingsClose: () => Promise<boolean>;
  discardMonitoringSettingsDraft: () => void;
  monitoringSettingsDirty: boolean;
  monitoringSettingsDraft: MonitoringSettingsDraft;
  setMonitoringSettingsDraft: Dispatch<SetStateAction<MonitoringSettingsDraft>>;
  usageTotalRequests: number;
  isMonitoringSettingsLoading: boolean;
  isMonitoringStatisticsResetting: boolean;
  isMonitoringSettingsSaving: boolean;
  handleMonitoringStatisticsReset: () => void;
  handleSaveMonitoringSettings: () => void | Promise<void>;
  t: TFunction;
}) {
  return (
      <ProSettingsSheet
        open={isMonitoringSettingsOpen}
        onClose={closeMonitoringSettings}
        confirmClose={confirmMonitoringSettingsClose}
        onDiscard={discardMonitoringSettingsDraft}
        title={t('usage_stats.monitoring_settings')}
        dirty={monitoringSettingsDirty}
        loading={isMonitoringSettingsLoading}
        saving={isMonitoringSettingsSaving || isMonitoringStatisticsResetting}
        cancelLabel={t('common.cancel')}
        saveLabel={isMonitoringSettingsSaving ? t('common.loading') : t('common.save')}
        dirtyLabel={t('common.unsaved_changes_title')}
        onSave={handleSaveMonitoringSettings}
        footerStart={(
          <Button
            variant="danger"
            size="sm"
            className={styles.resetStatisticsButton}
            onClick={handleMonitoringStatisticsReset}
            disabled={isMonitoringSettingsLoading || isMonitoringStatisticsResetting || isMonitoringSettingsSaving}
          >
            <IconTrash2 size={15} />
            {isMonitoringStatisticsResetting
              ? t('usage_stats.monitoring_settings_resetting')
              : t('usage_stats.monitoring_settings_reset_button')}
          </Button>
        )}
      >
        <div className={styles.monitoringSettingsEditor} aria-busy={isMonitoringSettingsLoading}>
          {isMonitoringSettingsLoading ? (
            <div className={styles.surfaceLoadingStatus} role="status">
              <LoadingSpinner size={16} />
              {t('common.loading')}
            </div>
          ) : null}
          <div className={styles.settingsSectionCard}>
            <div className={styles.settingsSectionHeader}>
              <strong>{t('usage_stats.monitoring_settings_retention_title')}</strong>
              <span>{t('usage_stats.monitoring_settings_retention_desc')}</span>
            </div>
            <label className={styles.settingsField}>
              <span>{t('usage_stats.monitoring_settings_retention_days')}</span>
              <Input
                type="number"
                min="0"
                step="1"
                value={monitoringSettingsDraft.retentionDays}
                onChange={(event) => setMonitoringSettingsDraft((previous) => ({ ...previous, retentionDays: event.target.value }))}
                placeholder="0"
              />
              <small>{t('usage_stats.monitoring_settings_retention_hint')}</small>
              <div className={styles.settingsScheduleNote}>{t('usage_stats.monitoring_settings_retention_schedule')}</div>
            </label>
          </div>

          <div className={styles.settingsSectionCard}>
            <div className={styles.settingsSectionHeader}>
              <strong>{t('usage_stats.monitoring_settings_webdav_title')}</strong>
              <span>{t('usage_stats.monitoring_settings_webdav_desc')}</span>
            </div>
            <label className={styles.settingsCheckboxField}>
              <input
                type="checkbox"
                checked={monitoringSettingsDraft.webdavEnabled}
                onChange={(event) => setMonitoringSettingsDraft((previous) => ({ ...previous, webdavEnabled: event.target.checked }))}
              />
              <span>{t('usage_stats.monitoring_settings_webdav_enabled')}</span>
            </label>
            <div className={styles.settingsGrid}>
              <label className={styles.settingsField}>
                <span>{t('usage_stats.monitoring_settings_webdav_interval')}</span>
                <Input
                  type="number"
                  min="1"
                  step="1"
                  value={monitoringSettingsDraft.webdavIntervalMinutes}
                  onChange={(event) => setMonitoringSettingsDraft((previous) => ({ ...previous, webdavIntervalMinutes: event.target.value }))}
                  placeholder="1440"
                />
              </label>
              <label className={styles.settingsField}>
                <span>{t('usage_stats.monitoring_settings_webdav_retention_days')}</span>
                <Input
                  type="number"
                  min="0"
                  step="1"
                  value={monitoringSettingsDraft.webdavRetentionDays}
                  onChange={(event) => setMonitoringSettingsDraft((previous) => ({ ...previous, webdavRetentionDays: event.target.value }))}
                  placeholder="0"
                />
                <small>{t('usage_stats.monitoring_settings_webdav_retention_hint')}</small>
              </label>
              <label className={styles.settingsField}>
                <span>{t('usage_stats.monitoring_settings_webdav_url')}</span>
                <Input
                  value={monitoringSettingsDraft.webdavUrl}
                  onChange={(event) => setMonitoringSettingsDraft((previous) => ({ ...previous, webdavUrl: event.target.value }))}
                  placeholder="https://example.com/dav/path"
                />
              </label>
              <label className={styles.settingsField}>
                <span>{t('usage_stats.monitoring_settings_webdav_username')}</span>
                <Input
                  value={monitoringSettingsDraft.webdavUsername}
                  onChange={(event) => setMonitoringSettingsDraft((previous) => ({ ...previous, webdavUsername: event.target.value }))}
                  autoComplete="username"
                />
              </label>
              <label className={styles.settingsField}>
                <span>{t('usage_stats.monitoring_settings_webdav_password')}</span>
                <Input
                  type="password"
                  value={monitoringSettingsDraft.webdavPassword}
                  onChange={(event) => setMonitoringSettingsDraft((previous) => ({ ...previous, webdavPassword: event.target.value }))}
                  autoComplete="current-password"
                />
              </label>
            </div>
            <small className={styles.settingsHint}>{t('usage_stats.monitoring_settings_webdav_hint')}</small>
          </div>

          <div className={`${styles.settingsSectionCard} ${styles.settingsDangerSection}`}>
            <div className={styles.settingsSectionHeader}>
              <strong>{t('usage_stats.monitoring_settings_data_title')}</strong>
              <span>{t('usage_stats.monitoring_settings_data_desc')}</span>
            </div>
            <div className={styles.settingsDangerAction}>
              <div>
                <span>{t('usage_stats.monitoring_settings_data_count')}</span>
                <strong>{formatCompactNumber(usageTotalRequests)}</strong>
              </div>
            </div>
          </div>

        </div>
      </ProSettingsSheet>
  );
}
