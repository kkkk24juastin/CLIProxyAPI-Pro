import { useCallback, useState } from 'react';

export function useProSurfaceState<T extends string>() {
  const [activeSurface, setActiveSurface] = useState<T | null>(null);
  const openSurface = useCallback((surface: T) => setActiveSurface(surface), []);
  const closeSurface = useCallback(() => setActiveSurface(null), []);
  const isSurfaceOpen = useCallback(
    (surface: T) => activeSurface === surface,
    [activeSurface]
  );
  return { activeSurface, openSurface, closeSurface, isSurfaceOpen };
}
