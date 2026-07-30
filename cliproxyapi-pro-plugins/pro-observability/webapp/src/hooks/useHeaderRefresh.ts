import { useEffect } from 'react';
import { RESPONSE_SOURCE } from '@/services/bridge';

export function useHeaderRefresh(refresh: () => void | Promise<void>): void {
  useEffect(() => {
    const listener = (event: MessageEvent<unknown>) => {
      if (event.source !== window.parent || !event.data || typeof event.data !== 'object') return;
      const message = event.data as Record<string, unknown>;
      if (message.source === RESPONSE_SOURCE && message.kind === 'ui.refresh') void refresh();
    };
    window.addEventListener('message', listener);
    return () => window.removeEventListener('message', listener);
  }, [refresh]);
}
