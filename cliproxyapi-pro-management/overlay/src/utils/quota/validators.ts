/**
 * Validation and type checking functions for quota management.
 */

import type { AuthFileItem } from '@/types';
import { GEMINI_CLI_IGNORED_MODEL_PREFIXES } from './constants';
import { normalizeNumberValue } from './parsers';

export function isRecordValue(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function readStringValue(value: unknown): string {
  if (value === undefined || value === null) return '';
  return String(value).trim();
}

export function readBooleanValue(value: unknown, fallback = false): boolean {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (['true', '1', 'yes', 'on'].includes(normalized)) return true;
    if (['false', '0', 'no', 'off'].includes(normalized)) return false;
  }
  return fallback;
}

export function resolveAuthProvider(file: AuthFileItem): string {
  const raw = file.provider ?? file.type ?? file.typo ?? '';
  return String(raw).trim().toLowerCase();
}

export function isAntigravityFile(file: AuthFileItem): boolean {
  return resolveAuthProvider(file) === 'antigravity';
}

export function isClaudeFile(file: AuthFileItem): boolean {
  return resolveAuthProvider(file) === 'claude';
}

export function isClaudeOAuthFile(file: AuthFileItem): boolean {
  if (!isClaudeFile(file)) return false;
  const metadata =
    file && typeof file.metadata === 'object' && file.metadata !== null
      ? (file.metadata as Record<string, unknown>)
      : null;
  const accessToken =
    metadata && typeof metadata.access_token === 'string'
      ? metadata.access_token.trim()
      : '';
  return accessToken.includes('sk-ant-oat');
}

export function isCodexFile(file: AuthFileItem): boolean {
  return resolveAuthProvider(file) === 'codex';
}

export function isGeminiCliFile(file: AuthFileItem): boolean {
  return resolveAuthProvider(file) === 'gemini-cli';
}

export function isKimiFile(file: AuthFileItem): boolean {
  return resolveAuthProvider(file) === 'kimi';
}

export function isRuntimeOnlyAuthFile(file: AuthFileItem): boolean {
  const raw = file['runtime_only'] ?? file.runtimeOnly;
  if (typeof raw === 'boolean') return raw;
  if (typeof raw === 'string') return raw.trim().toLowerCase() === 'true';
  return false;
}

export function isDisabledAuthFile(file: AuthFileItem): boolean {
  const raw = (file as { disabled?: unknown }).disabled;
  const statusRaw = file.status ?? file.state;
  const normalizedStatus =
    typeof statusRaw === 'string' ? statusRaw.trim().toLowerCase() : '';
  if (normalizedStatus === 'disabled' || normalizedStatus === 'inactive') {
    return true;
  }
  return readBooleanValue(raw);
}

export function isQuotaLowState(quota: unknown): boolean {
  if (!quota || typeof quota !== 'object' || Array.isArray(quota)) return false;
  const quotaRecord = quota as Record<string, unknown>;
  if (quotaRecord.status !== 'success') return false;

  return ['windows', 'groups', 'buckets', 'rows'].some((key) => {
    const value = quotaRecord[key];
    return Array.isArray(value) && value.some(isQuotaLowWindow);
  });
}

function isQuotaLowWindow(window: unknown): boolean {
  if (!window || typeof window !== 'object' || Array.isArray(window)) return false;
  const windowRecord = window as Record<string, unknown>;
  const usedPercent = normalizeNumberValue(windowRecord.usedPercent ?? windowRecord.used_percent);
  if (usedPercent !== null && usedPercent >= 100) return true;
  const remainingFraction = normalizeNumberValue(windowRecord.remainingFraction ?? windowRecord.remaining_fraction);
  if (remainingFraction !== null && remainingFraction <= 0) return true;
  const remainingAmount = normalizeNumberValue(windowRecord.remainingAmount ?? windowRecord.remaining_amount ?? windowRecord.remaining);
  if (remainingAmount !== null && remainingAmount <= 0) return true;
  const limit = normalizeNumberValue(windowRecord.limit);
  const used = normalizeNumberValue(windowRecord.used);
  return limit !== null && limit > 0 && used !== null && used >= limit;
}

export function isIgnoredGeminiCliModel(modelId: string): boolean {
  return GEMINI_CLI_IGNORED_MODEL_PREFIXES.some(
    (prefix) => modelId === prefix || modelId.startsWith(`${prefix}-`)
  );
}
