import { create } from 'zustand';

type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'error';
type AuthState = {
  apiBase: string;
  managementKey: string;
  connectionStatus: ConnectionStatus;
  supportsPlugin: boolean;
};

export const useAuthStore = create<AuthState>(() => ({
  apiBase: 'bridge://management',
  managementKey: '__host_managed__',
  connectionStatus: 'connected',
  supportsPlugin: true,
}));
