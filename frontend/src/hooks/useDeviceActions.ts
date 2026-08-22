import {
  ScanSubnet,
  ProbeByIP,
  RefreshNow,
  RebootDevice,
  ShutdownDevice,
} from '../../wailsjs/go/main/App';

export interface DeviceActions {
  scan: (cidr?: string) => Promise<void>;
  add: (ip: string, port: number, username: string) => Promise<void>;
  refresh: () => Promise<void>;
  reboot: (deviceID: string) => Promise<void>;
  shutdown: (deviceID: string) => Promise<void>;
}

export function useDeviceActions(): DeviceActions {
  return {
    scan: async (cidr) => {
      await ScanSubnet(cidr ?? '');
    },
    add: async (ip, port, _username) => {
      await ProbeByIP(ip, port, _username);
    },
    refresh: async () => {
      await RefreshNow();
    },
    reboot: async (deviceID) => {
      await RebootDevice(deviceID);
    },
    shutdown: async (deviceID) => {
      await ShutdownDevice(deviceID);
    },
  };
}