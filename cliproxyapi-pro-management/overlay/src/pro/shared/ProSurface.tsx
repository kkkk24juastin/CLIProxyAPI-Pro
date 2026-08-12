import {
  type PropsWithChildren,
  type ReactNode,
} from 'react';
import { Modal } from '@/components/ui/Modal';
import { Sheet, type SheetSize } from '@/components/ui/Sheet';
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
  onClose: () => void;
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
