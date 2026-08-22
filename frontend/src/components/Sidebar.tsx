import { Button, Tooltip, Popconfirm, Space, notification } from 'antd';
import { ScanOutlined, ReloadOutlined, DeleteOutlined } from '@ant-design/icons';
import { ClearRegistry } from '../../wailsjs/go/main/App';
import { useMenu } from '../state/MenuContext';
import { useDevices } from '../state/DeviceContext';
import { useI18n } from '../state/I18nContext';
import DeviceList from './DeviceList';
import { useState, useEffect, useRef } from 'react';

const SIDEBAR_MIN = 180;
const SIDEBAR_MAX = 480;
const SIDEBAR_DEFAULT = 260;
const SIDEBAR_KEY = 'spotter-sidebar-width';

export default function Sidebar() {
  const { state, refresh } = useDevices();
  const { triggerScan } = useMenu();
  const { t } = useI18n();
  const [refreshing, setRefreshing] = useState(false);
  const [width, setWidth] = useState<number>(() => {
    try {
      const v = localStorage.getItem(SIDEBAR_KEY);
      const n = v ? Number(v) : SIDEBAR_DEFAULT;
      return Number.isFinite(n) ? Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, n)) : SIDEBAR_DEFAULT;
    } catch { return SIDEBAR_DEFAULT; }
  });
  const draggingRef = useRef(false);

  useEffect(() => {
    try { localStorage.setItem(SIDEBAR_KEY, String(width)); } catch { /* ignore */ }
  }, [width]);

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!draggingRef.current) return;
      const next = Math.min(SIDEBAR_MAX, Math.max(SIDEBAR_MIN, e.clientX));
      setWidth(next);
    };
    const onUp = () => { draggingRef.current = false; document.body.style.cursor = ''; };
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
    };
  }, []);

  const onQuickScan = async () => {
    try {
      await triggerScan();
      await refresh();
      notification.success({ message: t('notif.scan.done'), placement: 'bottomRight', duration: 2 });
    } catch (e) {
      notification.error({ message: t('notif.scan.fail'), description: String(e), placement: 'bottomRight' });
    }
  };

  const onRefreshAll = async () => {
    setRefreshing(true);
    try {
      const { RefreshNow } = await import('../../wailsjs/go/main/App');
      await RefreshNow();
      await refresh();
      notification.success({ message: t('notif.refresh.done'), placement: 'bottomRight', duration: 1.5 });
    } catch (e) {
      notification.error({ message: t('notif.refresh.fail'), description: String(e), placement: 'bottomRight' });
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'row', flexShrink: 0 }}>
      <aside
        style={{
          width, flexShrink: 0,
          background: 'var(--bg-app)', borderRight: '1px solid var(--border)',
          display: 'flex', flexDirection: 'column',
        }}
      >
      <div style={{
        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
        padding: '10px 12px', borderBottom: '1px solid var(--border)',
      }}>
        <span style={{ color: 'var(--text-secondary)', fontSize: 12, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
          {t('sidebar.devices')}
        </span>
        <Space size={4}>
          <Tooltip title={t('sidebar.scan')}>
            <Button
              size="small"
              icon={<ScanOutlined />}
              onClick={onQuickScan}
              aria-label={t('sidebar.scan')}
            />
          </Tooltip>
          <Tooltip title={t('sidebar.refresh')}>
            <Button
              size="small"
              icon={<ReloadOutlined />}
              loading={refreshing}
              onClick={onRefreshAll}
              aria-label={t('sidebar.refresh')}
            />
          </Tooltip>
          <Popconfirm
            title={t('notif.clear.confirm.title')}
            description={t('notif.clear.confirm.body')}
            okText={t('notif.clear.confirm.ok')}
            cancelText={t('common.cancel')}
            onConfirm={async () => { await ClearRegistry(); await refresh(); }}
            disabled={state.devices.length === 0}
          >
            <Tooltip title={t('sidebar.clear')}>
              <Button
                size="small"
                danger
                icon={<DeleteOutlined />}
                disabled={state.devices.length === 0}
                aria-label={t('sidebar.clear')}
              />
            </Tooltip>
          </Popconfirm>
        </Space>
      </div>
      <DeviceList />
      </aside>
      <div
        role="separator"
        aria-orientation="vertical"
        onMouseDown={() => {
          draggingRef.current = true;
          document.body.style.cursor = 'col-resize';
        }}
        style={{
          width: 4,
          cursor: 'col-resize',
          background: 'transparent',
        }}
        data-testid="sidebar-resizer"
      />
    </div>
  );
}