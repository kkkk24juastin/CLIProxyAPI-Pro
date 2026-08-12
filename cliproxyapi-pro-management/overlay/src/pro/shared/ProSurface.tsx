import {
  type PropsWithChildren,
  type ReactNode,
  useCallback,
  useEffect,
  useRef,
} from 'react';
import { Modal } from '@/components/ui/Modal';
import { Sheet, type SheetSize } from '@/components/ui/Sheet';
import { Button } from '@/components/ui/Button';
import styles from './ProSurface.module.scss';

type SurfaceKind = 'detail' | 'form' | 'task' | 'workspace';

interface ProDialogProps {
  open: boolean;
  title?: ReactNode;
  onClose: () => void | boolean | Promise<void | boolean>;
  onAfterClose?: () => void;
  footer?: ReactNode;
  className?: string;
  closeDisabled?: boolean;
}

interface ProSurfaceDialogProps extends ProDialogProps {
  kind: SurfaceKind;
}

function ProSurfaceDialog({
  open,
  title,
  onClose,
  onAfterClose,
  footer,
  className,
  closeDisabled,
  kind,
  children,
}: PropsWithChildren<ProSurfaceDialogProps>) {
  const surfaceClassName = [styles.surface, styles[kind], className].filter(Boolean).join(' ');

  return (
    <Modal
      open={open}
      title={title}
      onClose={onClose}
      onAfterClose={onAfterClose}
      footer={footer}
      className={surfaceClassName}
      closeDisabled={closeDisabled}
    >
      {children}
    </Modal>
  );
}

export function ProDetailDialog(props: PropsWithChildren<ProDialogProps>) {
  return <ProSurfaceDialog {...props} kind="detail" />;
}

export function ProFormDialog(props: PropsWithChildren<ProDialogProps>) {
  return <ProSurfaceDialog {...props} kind="form" />;
}

export function ProTaskDialog(props: PropsWithChildren<ProDialogProps>) {
  return <ProSurfaceDialog {...props} kind="task" />;
}

export function ProWorkspaceDialog(props: PropsWithChildren<ProDialogProps>) {
  return <ProSurfaceDialog {...props} kind="workspace" />;
}

interface ProWorkspaceSheetProps {
  open: boolean;
  onClose: () => void | boolean | Promise<void | boolean>;
  onAfterClose?: () => void;
  size?: SheetSize;
  eyebrow?: ReactNode;
  title?: ReactNode;
  description?: ReactNode;
  footer?: ReactNode;
  closeDisabled?: boolean;
  className?: string;
  ariaLabel?: string;
  confirmClose?: () => boolean | Promise<boolean>;
}

export function ProWorkspaceSheet({
  open,
  onClose,
  size = 'lg',
  className,
  children,
  ...props
}: PropsWithChildren<ProWorkspaceSheetProps>) {
  return (
    <Sheet
      {...props}
      open={open}
      onClose={onClose}
      size={size}
      className={[styles.sheet, className].filter(Boolean).join(' ')}
    >
      {children}
    </Sheet>
  );
}

interface ProSettingsSheetProps extends Omit<ProWorkspaceSheetProps, 'footer'> {
  dirty: boolean;
  loading?: boolean;
  saving?: boolean;
  saveDisabled?: boolean;
  cancelLabel: ReactNode;
  saveLabel: ReactNode;
  dirtyLabel?: ReactNode;
  footerStart?: ReactNode;
  onSave: () => void | Promise<void>;
  onDiscard?: () => void;
}

export function ProSettingsSheet({
  dirty,
  loading = false,
  saving = false,
  saveDisabled = false,
  cancelLabel,
  saveLabel,
  dirtyLabel,
  footerStart,
  onSave,
  onClose,
  confirmClose,
  onDiscard,
  open,
  children,
  ...props
}: PropsWithChildren<ProSettingsSheetProps>) {
  const busy = loading || saving;
  const closeCheckRef = useRef<Promise<boolean> | null>(null);
  const cancelRequestedRef = useRef(false);
  const closeCommittedRef = useRef(false);

  useEffect(() => {
    closeCheckRef.current = null;
    cancelRequestedRef.current = false;
    closeCommittedRef.current = false;
  }, [open]);

  const commitClose = useCallback(() => {
    if (closeCommittedRef.current) return;
    closeCommittedRef.current = true;
    void onClose();
  }, [onClose]);

  const confirmSettingsClose = useCallback(() => {
    if (busy) return Promise.resolve(false);
    if (closeCheckRef.current) return closeCheckRef.current;

    const closeCheck = Promise.resolve(confirmClose?.() ?? true)
      .then((confirmed) => {
        if (confirmed && dirty) onDiscard?.();
        return confirmed;
      })
      .catch(() => false)
      .finally(() => {
        closeCheckRef.current = null;
      });
    closeCheckRef.current = closeCheck;
    return closeCheck;
  }, [busy, confirmClose, dirty, onDiscard]);

  const handleCancelClick = useCallback(async () => {
    if (busy || cancelRequestedRef.current) return;
    cancelRequestedRef.current = true;
    const confirmed = await confirmSettingsClose();
    if (confirmed) {
      commitClose();
      return;
    }
    cancelRequestedRef.current = false;
  }, [busy, commitClose, confirmSettingsClose]);

  const footer = (
    <div className={styles.settingsFooter}>
      <fieldset className={styles.settingsFooterStart} disabled={busy} aria-busy={busy}>
        {footerStart}
        {dirty && dirtyLabel ? <span className={styles.dirtyStatus}>{dirtyLabel}</span> : null}
      </fieldset>
      <div className={styles.settingsFooterActions}>
        <Button variant="secondary" onClick={() => void handleCancelClick()} disabled={busy}>
          {cancelLabel}
        </Button>
        <Button
          variant="primary"
          onClick={() => void onSave()}
          loading={saving}
          disabled={busy || saveDisabled}
        >
          {saveLabel}
        </Button>
      </div>
    </div>
  );

  return (
    <ProWorkspaceSheet
      {...props}
      open={open}
      onClose={commitClose}
      confirmClose={confirmSettingsClose}
      closeDisabled={busy || props.closeDisabled}
      footer={footer}
    >
      <fieldset className={styles.settingsFields} disabled={busy} aria-busy={busy}>
        {children}
      </fieldset>
    </ProWorkspaceSheet>
  );
}
