import { useState } from 'react';
import { Dropdown, Modal } from 'antd';
import { useMenu } from '../state/MenuContext';
import { useDevices } from '../state/DeviceContext';
import { useDeviceActions } from '../hooks/useDeviceActions';
import { ClearRegistry } from '../../wailsjs/go/main/App';
import { Quit } from '../../wailsjs/runtime/runtime';

interface MenuItemSpec {
  key: string;
  label: string;
  shortcut?: string;
  onClick?: () => void;
  danger?: boolean;
  disabled?: boolean;
}

type AppRegionStyle = React.CSSProperties & { WebkitAppRegion?: string };

const menuBtnStyle: AppRegionStyle = {
  background: 'transparent',
  border: 'none',
  color: '#d9d9d9',
  padding: '0 10px',
  fontSize: 13,
  cursor: 'pointer',
  height: '100%',
  borderRadius: 0,
  WebkitAppRegion: 'no-drag',
};

const menuBtnStyleHover: AppRegionStyle = {
  ...menuBtnStyle,
  background: '#262626',
};

export default function MenuBar() {
  const { openModal, triggerScan } = useMenu();
  const { refresh } = useDevices();
  const actions = useDeviceActions();
  const [modalApi, contextHolder] = Modal.useModal();
  const [hovered, setHovered] = useState<string | null>(null);

  const onClearRegistry = () =>
    modalApi.confirm({
      title: 'Clear registry',
      content: 'Remove every device from the local registry? Devices already running spotterd will be rediscovered automatically.',
      okText: 'Clear',
      okButtonProps: { danger: true },
      onOk: async () => { await ClearRegistry(); await refresh(); },
    });

  const onScan = () => {
    (async () => {
      try { await triggerScan(); await refresh(); }
      catch (e) { modalApi.error({ title: 'Scan failed', content: String(e) }); }
    })();
  };

  const fileMenu: MenuItemSpec[] = [
    { key: 'quit', label: 'Quit', shortcut: '⌘Q', onClick: Quit },
  ];

  const viewMenu: MenuItemSpec[] = [
    { key: 'refresh', label: 'Refresh', shortcut: 'F5', onClick: () => { actions.refresh().then(refresh); } },
    { key: 'clear', label: 'Clear registry…', onClick: onClearRegistry, danger: true },
  ];

  const toolsMenu: MenuItemSpec[] = [
    { key: 'scan', label: 'Scan local subnet', shortcut: '⌘L', onClick: onScan },
    { key: 'add', label: 'Add device by IP…', shortcut: '⌘I', onClick: () => openModal('add-device') },
    { key: 'guide', label: 'Device setup guide…', shortcut: 'F1', onClick: () => openModal('setup-guide') },
  ];

  const helpMenu: MenuItemSpec[] = [
    { key: 'about', label: 'About Spotter…', onClick: () => openModal('about') },
  ];

  const toAntdItems = (items: MenuItemSpec[]) =>
    items.map((it) => ({
      key: it.key,
      label: (
        <span style={{ display: 'flex', justifyContent: 'space-between', gap: 24 }}>
          <span style={{ color: it.danger ? '#ff7875' : undefined }}>{it.label}</span>
          {it.shortcut && <span style={{ color: '#888', fontSize: 11 }}>{it.shortcut}</span>}
        </span>
      ),
      onClick: it.onClick,
      disabled: it.disabled,
    }));

  const makeTrigger = (label: string, id: string) => (
    <button
      type="button"
      style={hovered === id ? menuBtnStyleHover : menuBtnStyle}
      onMouseEnter={() => setHovered(id)}
      onMouseLeave={() => setHovered((h) => (h === id ? null : h))}
    >
      {label}
    </button>
  );

  return (
    <>
      {contextHolder}
      <div style={{
        display: 'flex', alignItems: 'stretch', height: '100%',
        WebkitAppRegion: 'no-drag',
      } as AppRegionStyle}>
        <Dropdown menu={{ items: toAntdItems(fileMenu) }} trigger={['click']} placement="bottomLeft">
          {makeTrigger('File', 'file')}
        </Dropdown>
        <Dropdown menu={{ items: toAntdItems(viewMenu) }} trigger={['click']} placement="bottomLeft">
          {makeTrigger('View', 'view')}
        </Dropdown>
        <Dropdown menu={{ items: toAntdItems(toolsMenu) }} trigger={['click']} placement="bottomLeft">
          {makeTrigger('Tools', 'tools')}
        </Dropdown>
        <Dropdown menu={{ items: toAntdItems(helpMenu) }} trigger={['click']} placement="bottomLeft">
          {makeTrigger('Help', 'help')}
        </Dropdown>
      </div>
    </>
  );
}