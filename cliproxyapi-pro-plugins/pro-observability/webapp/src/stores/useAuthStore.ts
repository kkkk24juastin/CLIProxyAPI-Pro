import { create } from 'zustand';

type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'error';
type AuthState = {
  apiBase: string;
  managementKey: string;
  connectionStatus: ConnectionStatus;
};

export const useAuthStore = create<AuthState>(() => ({
  apiBase: 'bridge://management',
  managementKey: '__host_managed__',
  connectionStatus: 'connected',
}));
