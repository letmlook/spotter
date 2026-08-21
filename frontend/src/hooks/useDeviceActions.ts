import {
  DeployDevice,
  UninstallDevice,
  ScanSubnet,
  ProbeByIP,
  RefreshNow,
} from '../../wailsjs/go/main/App';

export interface DeviceActions {
  deploy: (ip: string, port: number, username: string, password: string) => Promise<void>;
  scan: (cidr: string) => Promise<void>;
  add: (ip: string, port: number, username: string) => Promise<void>;
  refresh: () => Promise<void>;
  uninstall: (deviceId: string, username: string, password: string) => Promise<void>;
}

export function useDeviceActions(): DeviceActions {
  return {
    deploy: async (ip, port, username, password) => {
      await DeployDevice(ip, port, password);
    },
    scan: async (cidr) => {
      await ScanSubnet(cidr);
    },
    add: async (ip, port, _username) => {
      await ProbeByIP(ip, port, _username);
    },
    refresh: async () => {
      await RefreshNow();
    },
    uninstall: async (deviceId, _username, password) => {
      await UninstallDevice(deviceId, _username, password);
    },
  };
}