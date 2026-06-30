/**
 * 配额管理界面的凭证勾选与删除逻辑。
 *
 * selectedNames 为全局集合（跨 provider section、跨分页保留），删除流程统一走 danger 确认弹窗，
 * 删除成功后从集合中移除并调用 onAfterDelete 刷新列表。
 */

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { authFilesApi } from '@/services/api';
import { useNotificationStore } from '@/stores';

export interface QuotaSelectionApi {
  selectedNames: Set<string>;
  toggleSelect: (name: string) => void;
  clearSelection: () => void;
  areAllSelected: (names: string[]) => boolean;
  toggleSelectAll: (names: string[]) => void;
  selectedCountIn: (names: string[]) => number;
  deleteOne: (name: string) => void;
  deleteSelected: (names: string[]) => void;
}

export function useQuotaSelection(onAfterDelete: () => void | Promise<void>): QuotaSelectionApi {
  const { t } = useTranslation();
  const showConfirmation = useNotificationStore((state) => state.showConfirmation);
  const showNotification = useNotificationStore((state) => state.showNotification);
  const [selectedNames, setSelectedNames] = useState<Set<string>>(new Set());

  const toggleSelect = useCallback((name: string) => {
    setSelectedNames((prev) => {
      const next = new Set(prev);
      if (next.has(name)) {
        next.delete(name);
      } else {
        next.add(name);
      }
      return next;
    });
  }, []);

  const clearSelection = useCallback(() => setSelectedNames(new Set()), []);

  const areAllSelected = useCallback(
    (names: string[]) => names.length > 0 && names.every((name) => selectedNames.has(name)),
    [selectedNames]
  );

  const toggleSelectAll = useCallback((names: string[]) => {
    setSelectedNames((prev) => {
      const next = new Set(prev);
      const allSelected = names.length > 0 && names.every((name) => next.has(name));
      names.forEach((name) => {
        if (allSelected) {
          next.delete(name);
        } else {
          next.add(name);
        }
      });
      return next;
    });
  }, []);

  const selectedCountIn = useCallback(
    (names: string[]) => names.reduce((count, name) => (selectedNames.has(name) ? count + 1 : count), 0),
    [selectedNames]
  );

  const runDelete = useCallback(
    (names: string[], titleKey: string, messageKey: string) => {
      const unique = Array.from(new Set(names));
      if (unique.length === 0) return;
      showConfirmation({
        title: t(titleKey),
        message: t(messageKey, { count: unique.length, name: unique[0] }),
        variant: 'danger',
        confirmText: t('common.confirm'),
        onConfirm: async () => {
          try {
            const result = await authFilesApi.deleteFiles(unique);
            setSelectedNames((prev) => {
              const next = new Set(prev);
              unique.forEach((name) => next.delete(name));
              return next;
            });
            if (result.failed.length === 0) {
              showNotification(`${t('quota_management.delete_success')} (${result.deleted})`, 'success');
            } else {
              showNotification(
                t('quota_management.delete_partial', {
                  success: result.deleted,
                  failed: result.failed.length,
                }),
                'warning'
              );
            }
          } catch (err: unknown) {
            const message = err instanceof Error ? err.message : '';
            showNotification(`${t('notification.delete_failed')}: ${message}`, 'error');
          } finally {
            await onAfterDelete();
          }
        },
      });
    },
    [onAfterDelete, showConfirmation, showNotification, t]
  );

  const deleteOne = useCallback(
    (name: string) =>
      runDelete([name], 'quota_management.delete_one_title', 'quota_management.delete_one_confirm'),
    [runDelete]
  );

  const deleteSelected = useCallback(
    (names: string[]) =>
      runDelete(
        names.filter((name) => selectedNames.has(name)),
        'quota_management.delete_selected_title',
        'quota_management.delete_selected_confirm'
      ),
    [runDelete, selectedNames]
  );

  return useMemo(
    () => ({
      selectedNames,
      toggleSelect,
      clearSelection,
      areAllSelected,
      toggleSelectAll,
      selectedCountIn,
      deleteOne,
      deleteSelected,
    }),
    [
      selectedNames,
      toggleSelect,
      clearSelection,
      areAllSelected,
      toggleSelectAll,
      selectedCountIn,
      deleteOne,
      deleteSelected,
    ]
  );
}
