/**
 * Zustand persistence middleware for quota data.
 * Automatically syncs quota state to SQLite quota cache.
 */

import { useQuotaStore } from '@/stores';
import {
  getQuotaProviderMapName,
  getQuotaProviderSetterName,
  isProQuotaProviderType,
  PRO_QUOTA_PROVIDER_TYPES,
  type ProQuotaProviderType,
} from '../quotaStoreMetadata';
import { sqliteQuotaCache, type QuotaCacheEntry } from './sqliteQuotaCache';
import {
  isAuthCardQuotaCacheDataCompatible,
  normalizePersistedQuotaState,
  selectPreferredQuotaCacheEntries,
} from './normalizedQuotaSnapshot';

interface QuotaStatusState {
  status: 'idle' | 'loading' | 'success' | 'error';
  cachedAt?: number;
  quotaProviderSnapshot?: boolean;
}

type QuotaStoreState = ReturnType<typeof useQuotaStore.getState>;
type QuotaMapUpdater = (
  previous: Record<string, QuotaStatusState>
) => Record<string, QuotaStatusState>;

class QuotaPersistenceMiddleware {
  private unsubscribe: (() => void) | null = null;
  private isPreloading = false;
  private syncQueue = new Set<string>();
  private isFlushing = false;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private retryDelayMs = 1_000;
  private syncedVersions = new Map<string, string>();
  private loadedGeneration = 0;
  private reloadRequested = false;
  private preloadPromise: Promise<void> | null = null;
  private ensureFreshPromise: Promise<void> | null = null;
  private lastQuotaMaps = new Map<ProQuotaProviderType, Record<string, QuotaStatusState>>();
  private hydratedKeys = new Map<ProQuotaProviderType, Set<string>>();

  /**
   * Start the middleware
   */
  start() {
    if (this.unsubscribe) {
      console.warn('QuotaPersistenceMiddleware already started');
      return;
    }

    // Check if upstream store structure is compatible
    if (!this.checkCompatibility()) {
      console.warn('QuotaPersistenceMiddleware: Upstream store structure changed, persistence disabled');
      return;
    }

    console.log('QuotaPersistenceMiddleware: Starting...');

    // Preload cache first
    this.ensureFresh().then(() => {
      console.log('QuotaPersistenceMiddleware: Cache preloaded');
    });

    this.unsubscribe = useQuotaStore.subscribe((state) => {
      if (this.isPreloading) return;

      PRO_QUOTA_PROVIDER_TYPES.forEach((provider) => {
        const quotaMap = this.getQuotaMap(state, provider);
        if (!quotaMap || this.lastQuotaMaps.get(provider) === quotaMap) return;
        this.lastQuotaMaps.set(provider, quotaMap);
        this.syncProvider(provider, quotaMap);
      });
    });

    console.log('QuotaPersistenceMiddleware: Started successfully');
  }

  /**
   * Stop the middleware
   */
  stop() {
    if (this.unsubscribe) {
      this.unsubscribe();
      this.unsubscribe = null;
    }
    this.lastQuotaMaps.clear();
    this.syncedVersions.clear();
    this.hydratedKeys.clear();
    if (this.retryTimer) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    void this.flushSyncQueue();
    console.log('QuotaPersistenceMiddleware: Stopped');
  }

  /**
   * Check if upstream store structure is compatible
   */
  private checkCompatibility(): boolean {
    const state = useQuotaStore.getState();
    const requiredFields = [
      ...PRO_QUOTA_PROVIDER_TYPES.map(getQuotaProviderMapName),
      ...PRO_QUOTA_PROVIDER_TYPES.map(getQuotaProviderSetterName),
      'clearQuotaCache',
    ];

    const missing = requiredFields.filter((field) => !(field in state));
    if (missing.length > 0) {
      console.error(`QuotaPersistenceMiddleware: Missing fields: ${missing.join(', ')}`);
      return false;
    }

    return true;
  }

  /**
   * Sync provider quota to SQLite quota cache.
   */
  private syncProvider(
    provider: ProQuotaProviderType,
    quotaMap: Record<string, QuotaStatusState>
  ) {
    let changed = false;
    const activeKeys = new Set<string>();
    Object.entries(quotaMap).forEach(([fileName, state]) => {
      const key = `${provider}:${fileName}`;
      activeKeys.add(key);
      if (state.status !== 'success') return;
      if (provider === 'gemini-cli' && state.quotaProviderSnapshot) return;

      const version = this.getSyncVersion(state);
      if (this.syncedVersions.get(key) === version) return;
      this.syncQueue.add(key);
      changed = true;
    });

    this.pruneSyncedVersions(provider, activeKeys);
    if (changed) void this.flushSyncQueue();
  }

  private getSyncVersion(state: unknown) {
    if (state && typeof state === 'object' && 'cachedAt' in state) {
      const cachedAt = (state as QuotaStatusState).cachedAt;
      if (cachedAt !== undefined) return String(cachedAt);
    }
    return JSON.stringify(state);
  }

  private pruneSyncedVersions(provider: ProQuotaProviderType, activeKeys: Set<string>) {
    const prefix = `${provider}:`;
    Array.from(this.syncedVersions.keys()).forEach((key) => {
      if (key.startsWith(prefix) && !activeKeys.has(key)) {
        this.syncedVersions.delete(key);
      }
    });
  }

  /**
   * Flush sync queue to SQLite quota cache
   */
  private async flushSyncQueue() {
    if (this.isFlushing) return;
    this.isFlushing = true;

    try {
      while (this.syncQueue.size > 0) {
        const key = this.syncQueue.values().next().value as string | undefined;
        if (!key) break;
        this.syncQueue.delete(key);

        const separatorIndex = key.indexOf(':');
        if (separatorIndex <= 0) continue;

        const provider = key.slice(0, separatorIndex) as ProQuotaProviderType;
        const fileName = key.slice(separatorIndex + 1);
        const state = useQuotaStore.getState();
        const quotaMap = this.getQuotaMap(state, provider);
        const quotaState = quotaMap?.[fileName];

        if (quotaState?.status !== 'success') continue;
        if (provider === 'gemini-cli' && quotaState.quotaProviderSnapshot) continue;

        const version = this.getSyncVersion(quotaState);
        const cachedAt = quotaState.cachedAt ?? Date.now();
        const synced = await sqliteQuotaCache.set(provider, fileName, { ...quotaState, cachedAt }, cachedAt);
        if (synced) {
          this.syncedVersions.set(key, version);
          this.retryDelayMs = 1_000;
        } else {
          this.syncQueue.add(key);
          this.scheduleRetry();
          break;
        }
      }
    } catch (err) {
      console.error('QuotaPersistenceMiddleware: Failed to sync to SQLite quota cache:', err);
    } finally {
      this.isFlushing = false;
      if (this.syncQueue.size > 0 && !this.retryTimer) {
        void this.flushSyncQueue();
      }
    }
  }

  private scheduleRetry() {
    if (this.retryTimer) return;
    const delay = this.retryDelayMs;
    this.retryDelayMs = Math.min(this.retryDelayMs * 2, 30_000);
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null;
      void this.flushSyncQueue();
    }, delay);
  }

  async ensureFresh() {
    if (this.ensureFreshPromise) return this.ensureFreshPromise;

    this.ensureFreshPromise = (async () => {
      const stats = await sqliteQuotaCache.getStats();
      if (!this.reloadRequested && stats.generation > 0 && stats.generation <= this.loadedGeneration) return;
      await this.runPreload(stats.generation);
      this.reloadRequested = false;
    })().finally(() => {
      this.ensureFreshPromise = null;
    });

    return this.ensureFreshPromise;
  }

  private runPreload(generation = 0) {
    if (this.preloadPromise) return this.preloadPromise;

    this.preloadPromise = this.preloadCache(generation).finally(() => {
      this.preloadPromise = null;
    });

    return this.preloadPromise;
  }

  markStale() {
    this.reloadRequested = true;
  }

  /**
   * Preload cache from SQLite quota cache to Zustand store
   */
  private async preloadCache(generation = 0) {
    this.isPreloading = true;

    try {
      const cachedEntries = await sqliteQuotaCache.getAll();
      const entriesByProvider = new Map<ProQuotaProviderType, QuotaCacheEntry[]>();
      cachedEntries.forEach((entry) => {
        if (!isProQuotaProviderType(entry.provider)) return;
        const provider = entry.provider;
        const entries = entriesByProvider.get(provider) ?? [];
        entries.push(entry);
        entriesByProvider.set(provider, entries);
      });

      PRO_QUOTA_PROVIDER_TYPES.forEach((provider) => {
        this.preloadProvider(provider, entriesByProvider.get(provider) ?? []);
      });
      this.loadedGeneration = Math.max(this.loadedGeneration, generation);
    } catch (err) {
      console.error('QuotaPersistenceMiddleware: Failed to preload cache:', err);
    } finally {
      this.isPreloading = false;
    }
  }

  /**
   * Preload single provider from SQLite quota cache
   */
  private preloadProvider(provider: ProQuotaProviderType, cachedEntries: QuotaCacheEntry[]) {
    const cached = selectPreferredQuotaCacheEntries(provider, cachedEntries);
    const previouslyHydrated = this.hydratedKeys.get(provider) ?? new Set<string>();

    const setterName = getQuotaProviderSetterName(provider);
    const storeState = useQuotaStore.getState();
    const setter = storeState[setterName] as unknown as (updater: QuotaMapUpdater) => void;

    if (typeof setter === 'function') {
      setter((prev) => {
        let changed = false;
        const next = { ...prev };
        previouslyHydrated.forEach((fileName) => {
          if (cached.has(fileName) || !(fileName in next)) return;
          delete next[fileName];
          this.syncedVersions.delete(`${provider}:${fileName}`);
          changed = true;
        });
        cached.forEach((entry, fileName) => {
          const data = normalizePersistedQuotaState(provider, entry.data, entry.cachedAt);
          if (!isAuthCardQuotaCacheDataCompatible(provider, data)) return;
          const quotaState = data as QuotaStatusState;
          this.syncedVersions.set(`${provider}:${fileName}`, this.getSyncVersion(quotaState));
          if (next[fileName] === quotaState) return;
          next[fileName] = quotaState;
          changed = true;
        });
        return changed ? next : prev;
      });

      this.hydratedKeys.set(provider, new Set(cached.keys()));

      console.log(`QuotaPersistenceMiddleware: Preloaded ${cached.size} entries for ${provider}`);
    }
  }

  /**
   * Get quota map from state by provider
   */
  private getQuotaMap(
    state: QuotaStoreState,
    provider: ProQuotaProviderType
  ): Record<string, QuotaStatusState> | null {
    const mapName = getQuotaProviderMapName(provider);
    return state[mapName] || null;
  }

  /**
   * Get cache statistics
   */
  async getStats() {
    return await sqliteQuotaCache.getStats();
  }

  /**
   * Clear all cache
   */
  async clearCache() {
    await sqliteQuotaCache.clear();
    this.syncedVersions.clear();
    this.syncQueue.clear();
    this.hydratedKeys.clear();
    this.loadedGeneration = 0;
    this.reloadRequested = true;
    console.log('QuotaPersistenceMiddleware: Cache cleared');
  }
}

export const quotaPersistenceMiddleware = new QuotaPersistenceMiddleware();
