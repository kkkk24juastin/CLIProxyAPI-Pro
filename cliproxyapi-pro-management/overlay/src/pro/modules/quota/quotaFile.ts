import type { AuthFileItem } from '@/types';
import { resolveAuthProvider } from '@/utils/quota';

export const isGeminiCliFile = (file: AuthFileItem): boolean =>
  resolveAuthProvider(file) === 'gemini-cli';

export const isRuntimeOnlyAuthFile = (file: AuthFileItem): boolean => {
  const raw = file['runtime_only'] ?? file.runtimeOnly;
  if (typeof raw === 'boolean') return raw;
  if (typeof raw === 'string') return raw.trim().toLowerCase() === 'true';
  return false;
};
