import importlib.util
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parents[1] / 'apply_customizations.py'
SPEC = importlib.util.spec_from_file_location('apply_customizations', MODULE_PATH)
assert SPEC and SPEC.loader
CUSTOMIZATIONS = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CUSTOMIZATIONS)


MODAL_SOURCE = """import { useEffect, useRef } from 'react';

export function Modal({ open }: { open: boolean }) {
  const [isVisible] = [false];
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (open || isVisible) return;
    previouslyFocusedRef.current?.focus();
    previouslyFocusedRef.current = null;
  }, [isVisible, open]);

  return null;
}
"""

SCROLL_LOCK_SOURCE = """const MODAL_LOCK_CLASS = 'modal-open';

let activeLockCount = 0;

const snapshot = {
  scrollY: 0,
};

export function lockScroll(): void {
  body.style.position = 'fixed';
  body.style.width = '100%';
}

export function unlockScroll(): void {
  contentEl.scrollTo({ top: 0 });
  window.scrollTo({ top: snapshot.scrollY });
}

export const FOCUSABLE_SELECTOR = 'button';
"""

MODAL_LIFECYCLE_SOURCE = """import { useCallback, useEffect, useRef, useState } from 'react';
import { FOCUSABLE_SELECTOR, lockScroll, unlockScroll } from './scrollLock';
interface ModalProps {
  open: boolean;
  onClose: () => void;
}
export function Modal({
  open,
  onClose,
  footer,
  closeDisabled = false,
}) {
  const titleId = 'modal-test';
  const [isVisible, setIsVisible] = useState(false);
  const [isClosing, setIsClosing] = useState(false);
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const getFocusableElements = useCallback(() => {
    return [];
  }, []);
  useEffect(() => {
    if (!open) return;
    const focusTimer = window.setTimeout(() => {
    }, 0);
    return () => window.clearTimeout(focusTimer);
  }, [getFocusableElements, open]);
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [closeDisabled, getFocusableElements, handleClose, open]);
  const startClose = useCallback(
    (notifyParent: boolean) => {
      if (closeTimerRef.current !== null) return;
      setIsClosing(true);
      closeTimerRef.current = window.setTimeout(() => {
        setIsVisible(false);
        setIsClosing(false);
        closeTimerRef.current = null;
        if (notifyParent) {
          onClose();
        }
      }, CLOSE_ANIMATION_DURATION);
    },
    [onClose]
  );
  useEffect(() => {
    let cancelled = false;
    if (open) {
      if (closeTimerRef.current !== null) {
        window.clearTimeout(closeTimerRef.current);
        closeTimerRef.current = null;
      }
      queueMicrotask(() => {
        if (cancelled) return;
        setIsVisible(true);
        setIsClosing(false);
      });
    } else if (isVisible) {
      queueMicrotask(() => {
        if (cancelled) return;
        startClose(false);
      });
    }
    return () => {
      cancelled = true;
    };
  }, [open, isVisible, startClose]);
  const handleClose = useCallback(() => {
    startClose(true);
  }, [startClose]);
  useEffect(() => {
    return () => {
      if (closeTimerRef.current !== null) {
        window.clearTimeout(closeTimerRef.current);
      }
    };
  }, []);
  return (
    <div
      role="dialog"
      aria-modal="true"
    />
  );
}
"""

SHEET_LIFECYCLE_SOURCE = """import { useCallback, useEffect, useRef, useState } from 'react';
import { FOCUSABLE_SELECTOR, lockScroll, unlockScroll } from '../scrollLock';
interface SheetProps {
  open: boolean;
  onClose: () => void;
}
export function Sheet({
  open,
  onClose,
  size = 'md',
  closeDisabled = false,
  confirmClose,
}) {
  const titleId = 'sheet-test';
  const [isVisible, setIsVisible] = useState(false);
  const [isClosing, setIsClosing] = useState(false);
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const previouslyFocusedRef = useRef<HTMLElement | null>(null);
  const sheetRef = useRef<HTMLDivElement | null>(null);
  const closeBtnRef = useRef<HTMLButtonElement | null>(null);
  const getFocusableElements = useCallback(() => {
    if (!sheetRef.current) return [] as HTMLElement[];
    return Array.from(sheetRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
      (el) => !el.hasAttribute('disabled') && el.tabIndex !== -1
    );
  }, []);
  useEffect(() => {
    if (!open) return;
    const t = window.setTimeout(() => {
      const first = getFocusableElements()[0];
      (first ?? closeBtnRef.current ?? sheetRef.current)?.focus({ preventScroll: true });
    }, 0);
    return () => window.clearTimeout(t);
  }, [getFocusableElements, open]);
  useEffect(() => {
    if (!open) return;
    const handleKey = (event: KeyboardEvent) => {
    };
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [closeDisabled, getFocusableElements, handleClose, open]);
  const startClose = useCallback(
    (notifyParent: boolean) => {
      if (closeTimerRef.current !== null) return;
      setIsClosing(true);
      closeTimerRef.current = window.setTimeout(() => {
        setIsVisible(false);
        setIsClosing(false);
        closeTimerRef.current = null;
        if (notifyParent) onClose();
      }, CLOSE_ANIMATION_DURATION);
    },
    [onClose]
  );
  useEffect(() => {
    if (open) setIsVisible(true);
    else if (isVisible) startClose(false);
  }, [open, isVisible, startClose]);
  const handleClose = useCallback(async () => {
    if (confirmClose) {
      const ok = await confirmClose();
      if (ok === false) return;
    }
    startClose(true);
  }, [confirmClose, startClose]);
  useEffect(() => {
    return () => {
      if (closeTimerRef.current !== null) window.clearTimeout(closeTimerRef.current);
    };
  }, []);
  useEffect(() => {
    if (open || isVisible) return;
    previouslyFocusedRef.current?.focus();
  }, [isVisible, open]);
  return (
    <div onMouseDown={(e) => {
        if (closeDisabled) return;
        if (e.target === e.currentTarget) handleClose();
      }}>
      <div
        role="dialog"
        aria-modal="true"
      />
    </div>
  );
}
"""

CONFIRMATION_STORE_SOURCE = """import { create } from 'zustand';
import type { ReactNode } from 'react';
import { generateId } from '@/utils/helpers';
interface ConfirmationOptions {
  title?: string;
  message: ReactNode;
  confirmText?: string;
  cancelText?: string;
  onConfirm: () => void | Promise<void>;
}
interface NotificationState {
  confirmation: { isOpen: boolean; isLoading: boolean; options: ConfirmationOptions | null };
  showConfirmation: (options: ConfirmationOptions) => void;
  hideConfirmation: () => void;
  setConfirmationLoading: (loading: boolean) => void;
}
export const useNotificationStore = create<NotificationState>((set) => ({
  confirmation: { isOpen: false, isLoading: false, options: null },
  showConfirmation: (options) => {
    set({ confirmation: { isOpen: true, isLoading: false, options } });
  },
  hideConfirmation: () => {
    set((state) => ({ confirmation: { ...state.confirmation, isOpen: false, options: null } }));
  },
  setConfirmationLoading: (loading) => {
    set((state) => ({ confirmation: { ...state.confirmation, isLoading: loading } }));
  },
}));
"""

CONFIRMATION_MODAL_SOURCE = """import { useTranslation } from 'react-i18next';
export function ConfirmationModal() {
  const setConfirmationLoading = useNotificationStore((state) => state.setConfirmationLoading);
  const { isOpen, isLoading, options } = confirmation;
  if (!isOpen || !options) {
    return null;
  }
  const handleConfirm = async () => {
    try {
      setConfirmationLoading(true);
      await onConfirm();
      hideConfirmation();
    } finally {
      setConfirmationLoading(false);
    }
  };
  return (
    <Modal open={isOpen} onClose={handleCancel} title={title} closeDisabled={isLoading}>
      content
    </Modal>
  );
}
"""

GLOBAL_STYLE_SOURCE = """@use './layout.scss';

html.modal-open,
body.modal-open {
  overflow: hidden;
}

body.modal-open .content {
  overflow: hidden;
}

body {
  color: var(--text-primary);
}
"""


class ModalCustomizationTest(unittest.TestCase):
    def setUp(self) -> None:
        CUSTOMIZATIONS._writes.clear()

    def test_restores_only_connected_trigger_without_scrolling(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            modal_dir = target / 'src/components/ui'
            modal_dir.mkdir(parents=True)
            modal_path = modal_dir / 'Modal.tsx'
            modal_path.write_text(MODAL_SOURCE)

            CUSTOMIZATIONS.patch_modal_focus_restore(target)
            CUSTOMIZATIONS.flush_writes()

            patched = modal_path.read_text()
            self.assertIn('previouslyFocused?.isConnected', patched)
            self.assertIn('previouslyFocused.focus({ preventScroll: true });', patched)
            self.assertNotIn('previouslyFocusedRef.current?.focus();', patched)

            CUSTOMIZATIONS.patch_modal_focus_restore(target)
            CUSTOMIZATIONS.flush_writes()
            self.assertEqual(patched, modal_path.read_text())

    def test_scroll_lock_restores_only_document_scroll(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            ui_dir = target / 'src/components/ui'
            ui_dir.mkdir(parents=True)
            scroll_lock_path = ui_dir / 'scrollLock.ts'
            scroll_lock_path.write_text(SCROLL_LOCK_SOURCE)

            CUSTOMIZATIONS.patch_modal_scroll_lock(target)
            CUSTOMIZATIONS.flush_writes()

            patched = scroll_lock_path.read_text()
            self.assertIn("body.style.overflow = 'hidden';", patched)
            self.assertIn("html.style.overflow = 'hidden';", patched)
            self.assertIn('scrollingElement.scrollHeight > scrollingElement.clientHeight + 1', patched)
            self.assertIn('if (snapshot.locksDocumentScroll)', patched)
            self.assertIn("body.style.position = 'fixed';", patched)
            self.assertIn("body.style.top = `-${snapshot.scrollY}px`;", patched)
            self.assertIn('if (restoreDocumentScroll)', patched)
            self.assertIn("window.scrollTo({ top: scrollY, left: scrollX, behavior: 'auto' });", patched)
            self.assertNotIn('contentEl.scrollTo(', patched)

            CUSTOMIZATIONS.patch_modal_scroll_lock(target)
            CUSTOMIZATIONS.flush_writes()
            self.assertEqual(patched, scroll_lock_path.read_text())

    def test_modal_close_is_parent_controlled_and_rerender_safe(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            modal_dir = target / 'src/components/ui'
            modal_dir.mkdir(parents=True)
            modal_path = modal_dir / 'Modal.tsx'
            modal_path.write_text(MODAL_LIFECYCLE_SOURCE)

            CUSTOMIZATIONS.patch_modal_lifecycle(target)
            CUSTOMIZATIONS.flush_writes()

            patched = modal_path.read_text()
            self.assertIn('const closeRequestedRef = useRef(false);', patched)
            self.assertIn('const onCloseRef = useRef(onClose);', patched)
            self.assertIn('const onAfterCloseRef = useRef(onAfterClose);', patched)
            self.assertIn('}, [onAfterClose, onClose]);', patched)
            self.assertIn('onAfterCloseRef.current?.();', patched)
            self.assertIn('registerOverlayLayer(titleId)', patched)
            self.assertIn('if (!isTopOverlayLayer(titleId)) return;', patched)
            self.assertIn("window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0", patched)
            self.assertIn('closeRequestedRef.current = true;\n    try {', patched)
            self.assertIn('const shouldClose = await onCloseRef.current();', patched)
            self.assertIn('if (shouldClose === false) {', patched)
            self.assertIn('closeRequestedRef.current = false;', patched)
            self.assertIn('window.clearTimeout(closeTimerRef.current);', patched)
            self.assertNotIn('if (closeRequestedRef.current) return;', patched)
            self.assertNotIn('setIsClosing(true);\n    } catch', patched)
            self.assertIn("role={open ? 'dialog' : undefined}", patched)
            self.assertNotIn('if (notifyParent)', patched)

    def test_sheet_close_confirmation_is_single_flight(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            sheet_dir = target / 'src/components/ui/Sheet'
            sheet_dir.mkdir(parents=True)
            sheet_path = sheet_dir / 'Sheet.tsx'
            sheet_path.write_text(SHEET_LIFECYCLE_SOURCE)

            CUSTOMIZATIONS.patch_sheet_lifecycle(target)
            CUSTOMIZATIONS.flush_writes()

            patched = sheet_path.read_text()
            self.assertIn('const closeRequestedRef = useRef(false);', patched)
            self.assertIn('}, [onAfterClose, onClose]);', patched)
            self.assertIn('onAfterCloseRef.current?.();', patched)
            self.assertIn('registerOverlayLayer(titleId)', patched)
            self.assertIn('const shouldRegisterOverlay = open || isVisible;', patched)
            self.assertIn('}, [shouldRegisterOverlay, titleId]);', patched)
            self.assertNotIn('}, [isVisible, open, titleId]);', patched)
            self.assertIn('if (!isVisible) return;', patched)
            self.assertIn('}, [getFocusableElements, isVisible, open, titleId]);', patched)
            self.assertIn("!el.matches(':disabled')", patched)
            self.assertIn('const fallback = closeBtnRef.current?.disabled ? sheetRef.current : closeBtnRef.current;', patched)
            self.assertIn('if (!isTopOverlayLayer(titleId)) return;', patched)
            self.assertIn("window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0", patched)
            self.assertIn('closeRequestedRef.current = true;\n    if (confirmClose)', patched)
            self.assertIn('closeRequestedRef.current = false;', patched)
            self.assertNotIn('if (closeRequestedRef.current) return;', patched)
            self.assertNotIn('setIsClosing(true);\n    onCloseRef.current();', patched)
            self.assertIn("aria-hidden={open ? undefined : true}", patched)
            self.assertIn('previouslyFocused?.isConnected', patched)

    def test_confirmation_requests_are_deduplicated_and_queued(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            store_dir = target / 'src/stores'
            common_dir = target / 'src/components/common'
            store_dir.mkdir(parents=True)
            common_dir.mkdir(parents=True)
            store_path = store_dir / 'useNotificationStore.ts'
            modal_path = common_dir / 'ConfirmationModal.tsx'
            store_path.write_text(CONFIRMATION_STORE_SOURCE)
            modal_path.write_text(CONFIRMATION_MODAL_SOURCE)

            CUSTOMIZATIONS.patch_confirmation_queue(target)
            CUSTOMIZATIONS.flush_writes()

            store = store_path.read_text()
            confirmation = modal_path.read_text()
            self.assertIn('dedupeKey?: string;', store)
            self.assertIn('confirmationQueue:', store)
            self.assertIn('state.confirmationQueue.some', store)
            self.assertIn('showConfirmation: (options: ConfirmationOptions) => boolean;', store)
            self.assertIn('let accepted = false;', store)
            self.assertIn('accepted = true;', store)
            self.assertIn('return accepted;', store)
            self.assertIn("typeof confirmationOptions.message === 'string'", store)
            self.assertIn(": 'react-node'", store)
            self.assertLess(store.index('const confirmationDedupeKey'), store.index('interface NotificationState'))
            self.assertIn('const dedupeKey = confirmationDedupeKey(options);', store)
            self.assertIn('confirmationDedupeKey(state.confirmation.options)', store)
            self.assertIn('if (state.confirmation.id && currentKey === dedupeKey)', store)
            self.assertIn('confirmationDedupeKey(item.options) === dedupeKey', store)
            self.assertIn('const [next, ...remaining]', store)
            self.assertIn('advanceConfirmation:', store)
            self.assertIn('if (state.confirmation.id)', store)
            self.assertIn('if (!options)', confirmation)
            self.assertNotIn('if (!isOpen || !options)', confirmation)
            self.assertIn('onAfterClose={advanceConfirmation}', confirmation)
            self.assertIn('const submittingRef = useRef(false);', confirmation)
            self.assertIn('if (submittingRef.current || isLoading) return;', confirmation)

    def test_modal_keeps_the_content_scrollbar_layout(self) -> None:
        with tempfile.TemporaryDirectory() as temp_dir:
            target = Path(temp_dir)
            styles_dir = target / 'src/styles'
            styles_dir.mkdir(parents=True)
            global_style_path = styles_dir / 'global.scss'
            global_style_path.write_text(GLOBAL_STYLE_SOURCE)

            CUSTOMIZATIONS.patch_modal_content_scrollbar_layout(target)
            CUSTOMIZATIONS.flush_writes()

            patched = global_style_path.read_text()
            self.assertIn('html.modal-open,\nbody.modal-open {\n  overflow: hidden;\n}', patched)
            self.assertNotIn('body.modal-open .content', patched)

            CUSTOMIZATIONS.patch_modal_content_scrollbar_layout(target)
            CUSTOMIZATIONS.flush_writes()
            self.assertEqual(patched, global_style_path.read_text())


if __name__ == '__main__':
    unittest.main()
