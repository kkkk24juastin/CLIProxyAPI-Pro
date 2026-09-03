import { useCallback, useEffect, useRef, useState } from 'react';

type OverlayCloseHandler = () => void | boolean | Promise<void | boolean>;
type OverlayBeforeClose = () => boolean | Promise<boolean>;

interface OverlayLifecycleOptions {
  open: boolean;
  closeDisabled: boolean;
  closeAnimationDuration: number;
  onClose: OverlayCloseHandler;
  onAfterClose?: () => void;
}

export function useOverlayLifecycle({
  open,
  closeDisabled,
  closeAnimationDuration,
  onClose,
  onAfterClose,
}: OverlayLifecycleOptions) {
  const [isVisible, setIsVisible] = useState(false);
  const [isClosing, setIsClosing] = useState(false);
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const closeRequestedRef = useRef(false);
  const onCloseRef = useRef(onClose);
  const onAfterCloseRef = useRef(onAfterClose);

  useEffect(() => {
    onCloseRef.current = onClose;
    onAfterCloseRef.current = onAfterClose;
  }, [onAfterClose, onClose]);

  const finishClose = useCallback(() => {
    if (closeTimerRef.current !== null) return;
    setIsClosing(true);
    closeTimerRef.current = window.setTimeout(
      () => {
        setIsVisible(false);
        setIsClosing(false);
        closeTimerRef.current = null;
        closeRequestedRef.current = false;
        onAfterCloseRef.current?.();
      },
      window.matchMedia('(prefers-reduced-motion: reduce)').matches
        ? 0
        : closeAnimationDuration
    );
  }, [closeAnimationDuration]);

  useEffect(() => {
    let cancelled = false;
    if (open) {
      if (closeTimerRef.current !== null) {
        window.clearTimeout(closeTimerRef.current);
        closeTimerRef.current = null;
      }
      closeRequestedRef.current = false;
      queueMicrotask(() => {
        if (cancelled) return;
        setIsVisible(true);
        setIsClosing(false);
      });
    } else if (isVisible) {
      finishClose();
    }
    return () => {
      cancelled = true;
    };
  }, [finishClose, isVisible, open]);

  useEffect(() => {
    return () => {
      if (closeTimerRef.current !== null) {
        window.clearTimeout(closeTimerRef.current);
      }
    };
  }, []);

  const requestClose = useCallback(
    async (beforeClose?: OverlayBeforeClose) => {
      if (closeRequestedRef.current || closeDisabled) return;
      closeRequestedRef.current = true;
      try {
        if (beforeClose) {
          try {
            const shouldContinue = await beforeClose();
            if (shouldContinue === false) return;
          } catch {
            return;
          }
        }
        const shouldClose = await onCloseRef.current();
        if (shouldClose === false) return;
      } finally {
        closeRequestedRef.current = false;
      }
    },
    [closeDisabled]
  );

  return { isVisible, isClosing, requestClose };
}
