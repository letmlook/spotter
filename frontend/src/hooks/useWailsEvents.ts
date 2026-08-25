import { useEffect } from 'react';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { useDevices } from '../state/DeviceContext';

export function useWailsEvents(onUnknownDevice?: (payload: unknown) => void) {
  const { refresh } = useDevices();

  useEffect(() => {
    const off1 = EventsOn('info-updated', () => { refresh(); });
    const off2 = EventsOn('offline', () => { refresh(); });
    // Wails EventsOn callbacks receive variadic Go args as a JS
    // array. We Emit(ctx, name, value) so args is a single-element
    // array. Extract the event from args[0].
    const off3 = EventsOn('unknown-device', (...args: unknown[]) => {
      const arg = args[0] as { Info?: { device_id?: string; network?: { primary_ip?: string } }; IP?: string; Port?: number } | undefined;
      if (onUnknownDevice) onUnknownDevice(arg);
      const event = arg ?? {};
      const info = event.Info ?? {};
      const ip = event.IP ?? info.network?.primary_ip ?? '';
      const port = event.Port ?? 9999;
      const deviceId = info.device_id ?? '';
      if (!deviceId) return;
      import('../../wailsjs/go/main/RegistryApp').then(({ AcceptUnknownDevice }) => {
        AcceptUnknownDevice(deviceId, ip, port, '').then(refresh).catch((err: unknown) => {
          // eslint-disable-next-line no-console
          console.warn('AcceptUnknownDevice failed', deviceId, err);
        });
      });
    });
    return () => { off1(); off2(); off3(); };
  }, [refresh, onUnknownDevice]);
}
