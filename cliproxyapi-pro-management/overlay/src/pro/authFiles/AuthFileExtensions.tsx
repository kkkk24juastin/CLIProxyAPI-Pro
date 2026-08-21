import { useEffect, useSyncExternalStore } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { IconNetwork } from '@/components/ui/icons';
import styles from '@/features/authFiles/components/AuthFileCard.module.scss';
import type { AuthFileItem } from '@/types';
import { IconChartColumnIncreasing } from '@/pro/icons';
import { AccountUsageModal } from '@/pro/modules/monitoring';
import { AuthFileConnectionTestModal } from './AuthFileConnectionTestModal';

type AuthFileSurfaceKind = 'usage' | 'connection-test';

interface AuthFileSurfaceState {
  kind: AuthFileSurfaceKind;
  file: AuthFileItem;
}

let activeSurface: AuthFileSurfaceState | null = null;
const surfaceListeners = new Set<() => void>();

const subscribeToSurface = (listener: () => void) => {
  surfaceListeners.add(listener);
  return () => surfaceListeners.delete(listener);
};

const getSurfaceSnapshot = () => activeSurface;

const setSurface = (next: AuthFileSurfaceState | null) => {
  activeSurface = next;
  surfaceListeners.forEach((listener) => listener());
};

const openSurface = (kind: AuthFileSurfaceKind, file: AuthFileItem) => {
  setSurface({ kind, file });
};

const closeSurface = () => setSurface(null);

export function AuthFileUsageActionExtension({
  file,
  disabled,
}: {
  file: AuthFileItem;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  if (typeof file.authIndex !== 'string') return null;

  return (
    <Button
      variant="secondary"
      size="sm"
      onClick={() => openSurface('usage', file)}
      title={t('account_usage.card_action')}
      disabled={disabled}
    >
      <IconChartColumnIncreasing size={14} />
      {t('account_usage.card_action')}
    </Button>
  );
}

export function AuthFileConnectionActionExtension({
  file,
  disabled,
}: {
  file: AuthFileItem;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <Button
      variant="secondary"
      size="sm"
      onClick={() => openSurface('connection-test', file)}
      className={styles.iconButton}
      title={t('auth_files.connection_test_button')}
      disabled={disabled}
    >
      <IconNetwork size={15} />
    </Button>
  );
}

export function AuthFileSurfaceExtensions() {
  const surface = useSyncExternalStore(
    subscribeToSurface,
    getSurfaceSnapshot,
    getSurfaceSnapshot
  );

  useEffect(() => closeSurface, []);

  return (
    <>
      <AuthFileConnectionTestModal
        file={surface?.kind === 'connection-test' ? surface.file : null}
        onClose={closeSurface}
      />
      <AccountUsageModal
        file={surface?.kind === 'usage' ? surface.file : null}
        onClose={closeSurface}
      />
    </>
  );
}
