import { create } from 'zustand';
import type { Config } from '@/types';
import { confirmHost, notifyHost, type HostBootstrap } from '@/services/bridge';
export { useAuthStore } from './useAuthStore';

type ConfigState = { config: Config | null };
export const useConfigStore = create<ConfigState>(() => ({ config: null }));

type QuotaState = {
  antigravityQuota: Record<string, any>;
  claudeQuota: Record<string, any>;
  codexQuota: Record<string, any>;
  geminiCliQuota: Record<string, any>;
  kimiQuota: Record<string, any>;
  xaiQuota: Record<string, any>;
};
export const useQuotaStore = create<QuotaState>(() => ({
  antigravityQuota: {}, claudeQuota: {}, codexQuota: {}, geminiCliQuota: {}, kimiQuota: {}, xaiQuota: {},
}));

type ConfirmationOptions = {
  title?: string;
  message: unknown;
  confirmText?: string;
  cancelText?: string;
  variant?: 'danger' | 'primary' | 'secondary';
  onConfirm: () => void | Promise<void>;
  onCancel?: () => void;
};
type NotificationState = {
  showNotification: (message: string, type?: string) => void;
  showConfirmation: (options: ConfirmationOptions) => void;
};
export const useNotificationStore = create<NotificationState>(() => ({
  showNotification: (message, type = 'info') => { void notifyHost(message, type); },
  showConfirmation: (options) => {
    void confirmHost({
      title: options.title,
      message: typeof options.message === 'string' ? options.message : String(options.message ?? ''),
      confirmText: options.confirmText,
      cancelText: options.cancelText,
      variant: options.variant,
    }).then((confirmed) => confirmed ? options.onConfirm() : options.onCancel?.());
  },
}));

export function initializeStores(bootstrap: HostBootstrap): void {
  useConfigStore.setState({ config: bootstrap.config as Config });
}
