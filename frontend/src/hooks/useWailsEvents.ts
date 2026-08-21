import { useEffect } from 'react';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { useDevices } from '../state/DeviceContext';

export function useWailsEvents(onUnknownDevice?: (payload: unknown) => void) {
  const { refresh } = useDevices();

  useEffect(() => {
    const off1 = EventsOn('info-updated', () => { refresh(); });
    const off2 = EventsOn('offline', () => { refresh(); });
    const off3 = EventsOn('unknown-device', (payload: unknown) => {
      if (onUnknownDevice) onUnknownDevice(payload);
      // silent accept: assume the user wants tracked devices.
      const data = payload as { Info?: { device_id?: string }; IP?: string; Port?: number };
      const info = data?.Info || {};
      const ip = data?.IP || info?.device_id ? '' : '';
      const port = data?.Port || 9999;
      const deviceId = info?.device_id;
      if (!deviceId) return;
      import('../../wailsjs/go/main/App').then(({ AcceptUnknownDevice }) => {
        AcceptUnknownDevice(deviceId, ip, port, '').then(refresh).catch(() => {});
      });
    });
    return () => { off1(); off2(); off3(); };
  }, [refresh, onUnknownDevice]);
}
