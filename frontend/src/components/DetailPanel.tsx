import { useState } from 'react';
import { Button, Modal, Space, Tooltip, message } from 'antd';
import { PoweroffOutlined, CloseCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';
import { useDeviceActions } from '../hooks/useDeviceActions';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';
import LogSection from './LogSection';
import MetricSpark from './MetricSpark';
import { useEffect, useState } from 'react';
import { Tag as AntTag } from 'antd';

type PowerAction = 'reboot' | 'shutdown';

export default function DetailPanel() {
  const { state, refresh } = useDevices();
  const { t } = useI18n();
  const actions = useDeviceActions();
  const [busyAction, setBusyAction] = useState<PowerAction | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const device = state.devices.find((d) => d.device_id === state.selectedId);
  const hostname = device?.last_info?.basic?.hostname || device?.ip || '';
  // Rolling history of CPU seconds + CPU temp; the parent component
  // re-renders whenever the registry snapshot changes (every poll
  // cycle). We cap history at 80 samples (~6 minutes at 5s poll).
  const [cpuHist, setCpuHist] = useState<number[]>([]);
  const [tempHist, setTempHist] = useState<number[]>([]);
  const [memHist, setMemHist] = useState<number[]>([]);
  useEffect(() => {
    const mi = device?.last_info?.metrics;
    if (!mi) return;
    if (mi.cpu_seconds_total != null) {
      setCpuHist((prev) => [...prev.slice(-79), mi.cpu_seconds_total as number]);
    }
    if (mi.cpu_temp_c != null) {
      setTempHist((prev) => [...prev.slice(-79), mi.cpu_temp_c as number]);
    }
    if (mi.mem_total_bytes && mi.mem_available_bytes) {
      const used = 1 - (mi.mem_available_bytes as number) / (mi.mem_total_bytes as number);
      setMemHist((prev) => [...prev.slice(-79), used * 100]);
    }
  }, [device?.last_info?.metrics]);

  const onRefresh = async () => {
    if (!device) return;
    setRefreshing(true);
    try { await actions.refresh(); await refresh(); }
    finally { setRefreshing(false); }
  };

  const runPowerAction = (kind: PowerAction) => {
    if (!device) return;
    const titleKey =
      kind === 'reboot'
        ? 'detail.actions.power.reboot.confirmTitle'
        : 'detail.actions.power.shutdown.confirmTitle';
    const okKey =
      kind === 'reboot'
        ? 'detail.actions.power.reboot.confirmOk'
        : 'detail.actions.power.shutdown.confirmOk';
    const bodyKey =
      kind === 'shutdown'
        ? 'detail.actions.power.shutdown'
        : 'detail.actions.power.reboot';

    Modal.confirm({
      title: t(titleKey).replace('{hostname}', hostname),
      content: t(bodyKey),
      okText: t(okKey),
      okButtonProps: { danger: kind === 'shutdown' },
      cancelText: t('common.cancel'),
      onOk: async () => {
        setBusyAction(kind);
        try {
          if (kind === 'reboot') {
            await actions.reboot(device.device_id);
          } else {
            await actions.shutdown(device.device_id);
          }
          message.success(t('detail.actions.power.toast.success') ?? 'Command sent');
        } catch (e: unknown) {
          const err = e as { message?: string } | null;
          const msg = (err?.message ?? String(e)) as string;
          if (msg.includes('power actions disabled')) {
            message.error(t('detail.actions.power.disabledHint'));
          } else {
            message.error(msg);
          }
        } finally {
          setBusyAction(null);
        }
      },
    });
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {!device ? <EmptyState /> : (
          <>
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: 16,
              }}
            >
              <h2 style={{ margin: 0, color: 'var(--text-primary)' }}>
                {device.last_info?.basic?.hostname || device.ip}
                <span
                  style={{
                    marginLeft: 12,
                    fontSize: 13,
                    color: device.online ? '#52c41a' : '#b71c1c',
                  }}
                >
                  {device.online ? t('detail.status.online') : t('detail.status.offline')}
                </span>
              </h2>
              <Space>
                <Tooltip title={t('detail.refresh')}>
                  <Button
                    size="small"
                    icon={<ReloadOutlined />}
                    loading={refreshing}
                    onClick={onRefresh}
                  >
                    {t('detail.refresh')}
                  </Button>
                </Tooltip>
                <Button
                  size="small"
                  icon={<PoweroffOutlined />}
                  disabled={!device.online || busyAction !== null}
                  loading={busyAction === 'reboot'}
                  onClick={() => runPowerAction('reboot')}
                >
                  {t('detail.actions.power.reboot')}
                </Button>
                <Button
                  size="small"
                  danger
                  icon={<CloseCircleOutlined />}
                  disabled={!device.online || busyAction !== null}
                  loading={busyAction === 'shutdown'}
                  onClick={() => runPowerAction('shutdown')}
                >
                  {t('detail.actions.power.shutdown')}
                </Button>
              </Space>
            </div>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <BasicCard device={device} />
              <JetsonCard device={device} />
            </div>
            <div style={{ marginTop: 12 }}>
              <NetworkCard device={device} />
            </div>
            {(device.tags && device.tags.length > 0) && (
              <div style={{ marginTop: 8 }}>
                {device.tags.map((tag) => (
                  <AntTag key={tag} color="blue">{tag}</AntTag>
                ))}
              </div>
            )}
            <div style={{ marginTop: 12, display: 'flex', gap: 16 }}>
              <MetricSpark label={t('metrics.cpu')} unit="s" points={cpuHist} color="#69b1ff" decimals={1} />
              <MetricSpark label={t('metrics.cpuTemp')} unit="°C" points={tempHist} color="#ffc53d" decimals={0} />
              <MetricSpark label={t('metrics.memUsage')} unit="%" points={memHist} color="#95de64" decimals={1} />
            </div>
          </>
        )}
      </div>
      {device && (
        <div
          style={{
            borderTop: '1px solid var(--border)',
            padding: '12px 16px',
            background: 'var(--bg-app)',
          }}
        >
          <LogSection deviceID={device.device_id} online={device.online} />
        </div>
      )}
    </div>
  );
}