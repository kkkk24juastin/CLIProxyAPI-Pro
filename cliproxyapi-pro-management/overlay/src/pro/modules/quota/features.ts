/**
 * Feature flags for optional enhancements.
 */

export const FEATURES = {
  /**
   * Enable SQLite-backed persistence for quota data.
   * When enabled, quota data survives page refresh and browser changes.
   */
  QUOTA_PERSISTENCE: true,

  /**
   * Show cached timestamp on quota cards.
   * Requires QUOTA_PERSISTENCE to be enabled.
   */
  QUOTA_CACHE_TIMESTAMP: true,
} as const;
