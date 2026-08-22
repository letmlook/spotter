import { createContext, useCallback, useContext, useEffect, useReducer } from 'react';
import type { ReactNode } from 'react';
import { ListDevices } from '../../wailsjs/go/main/App';

export interface DeviceInfo {
  schema_version?: number;
  device_id?: string;
  collected_at?: string;
  agent_version?: string;
  basic?: {
    hostname?: string;
    username?: string;
    os?: { pretty_name?: string; id?: string; version_id?: string };
    kernel?: string;
    arch?: string;
    uptime_seconds?: number;
  };
  network?: {
    primary_ip?: string;
    interfaces?: Array<{
      name?: string;
      mac?: string;
      addrs?: string[];
    }>;
  };
  jetson?: {
    model?: string;
    jetpack?: string;
    l4t?: string;
    cuda?: string;
    cudnn?: string;
    tensorrt?: string;
    python?: string;
    serial?: string;
  } | null;
}

export interface RegistryEntry {
  device_id: string;
  ip: string;
  port: number;
  username: string;
  deployed_at?: string;
  last_seen_at?: string;
  last_source?: string;
  online: boolean;
  last_info?: DeviceInfo;
}

interface DeviceState {
  devices: RegistryEntry[];
  selectedId: string | null;
  searchQuery: string;
  loading: boolean;
}

type Action =
  | { type: 'SET_DEVICES'; devices: RegistryEntry[] }
  | { type: 'SELECT'; id: string | null }
  | { type: 'SEARCH'; query: string }
  | { type: 'SET_LOADING'; loading: boolean };

function reducer(state: DeviceState, action: Action): DeviceState {
  switch (action.type) {
    case 'SET_DEVICES': {
      // Auto-select the first device whenever the registry snapshot
      // refreshes and no selection is set. This makes the GUI drop
      // directly into BasicCard + NetworkCard + LogSection for
      // single-device deployments without requiring the user to
      // click a row first.
      let nextSelected = state.selectedId
      if (!nextSelected && action.devices.length > 0) {
        nextSelected = action.devices[0].device_id
      }
      return { ...state, devices: action.devices, selectedId: nextSelected }
    }
    case 'SELECT': {
      // Drop selection if the previously selected device is gone.
      if (action.id && !state.devices.some((d) => d.device_id === action.id)) {
        return { ...state, selectedId: null };
      }
      return { ...state, selectedId: action.id };
    }
    case 'SEARCH':
      return { ...state, searchQuery: action.query };
    case 'SET_LOADING':
      return { ...state, loading: action.loading };
  }
}

const initial: DeviceState = { devices: [], selectedId: null, searchQuery: '', loading: false };

const DeviceContext = createContext<{
  state: DeviceState;
  dispatch: React.Dispatch<Action>;
  refresh: () => Promise<void>;
} | null>(null);

export function DeviceProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initial);

  const refresh = useCallback(async () => {
    dispatch({ type: 'SET_LOADING', loading: true });
    try {
      const devices = (await ListDevices()) as unknown as RegistryEntry[];
      dispatch({ type: 'SET_DEVICES', devices });
    } finally {
      dispatch({ type: 'SET_LOADING', loading: false });
    }
  }, []);

  // Initial fetch.
  useEffect(() => { refresh(); }, [refresh]);

  return (
    <DeviceContext.Provider value={{ state, dispatch, refresh }}>
      {children}
    </DeviceContext.Provider>
  );
}

export function useDevices() {
  const ctx = useContext(DeviceContext);
  if (!ctx) throw new Error('useDevices must be used inside DeviceProvider');
  return ctx;
}
