import { useState } from 'react';
import { Dropdown, Modal } from 'antd';
import { useMenu } from '../state/MenuContext';
import { useDevices } from '../state/DeviceContext';
import { useDeviceActions } from '../hooks/useDeviceActions';
import { useI18n } from '../state/I18nContext';
import { useTheme } from '../state/ThemeContext';
import { ClearRegistry } from '../../wailsjs/go/main/App';
import { Quit, WindowClose } from '../../wailsjs/runtime/runtime';
import type { Locale } from '../i18n/dictionaries';

interface MenuItemSpec {
  key: string;
  label: string;
  shortcut?: string;
  onClick?: () => void;
  danger?: boolean;
  disabled?: boolean;
  selected?: boolean;
  children?: MenuItemSpec[];
}

type AppRegionStyle = React.CSSProperties & { WebkitAppRegion?: string };

const menuBtnStyle: AppRegionStyle = {
  background: 'transparent',
  border: 'none',
  color: 'var(--text-primary)',
  padding: '0 10px',
  fontSize: 13,
  cursor: 'pointer',
  height: '100%',
  borderRadius: 0,
  WebkitAppRegion: 'no-drag',
};

const menuBtnStyleHover: AppRegionStyle = {
  ...menuBtnStyle,
  background: 'var(--bg-hover)',
};

export default function MenuBar() {
  const { openModal, triggerScan } = useMenu();
  const { refresh } = useDevices();
  const actions = useDeviceActions();
  const { t, locale, setLocale } = useI18n();
  const { mode, setMode } = useTheme();
  const [modalApi, contextHolder] = Modal.useModal();
  const [hovered, setHovered] = useState<string | null>(null);

  const onClearRegistry = () =>
    modalApi.confirm({
      title: t('notif.clear.confirm.title'),
      content: t('notif.clear.confirm.body'),
      okText: t('notif.clear.confirm.ok'),
      okButtonProps: { danger: true },
      onOk: async () => { await ClearRegistry(); await refresh(); },
    });

  const onScan = () => {
    (async () => {
      try { await triggerScan(); await refresh(); }
      catch (e) { modalApi.error({ title: t('notif.scan.fail'), content: String(e) }); }
    })();
  };

  const fileMenu: MenuItemSpec[] = [
    { key: 'close', label: t('menu.file.close'), shortcut: '⌘W', onClick: WindowClose },
    { key: 'quit', label: t('menu.file.quit'), shortcut: '⌘Q', onClick: Quit },
  ];

  const viewMenu: MenuItemSpec[] = [
    { key: 'refresh', label: t('menu.view.refresh'), shortcut: 'F5', onClick: () => { actions.refresh().then(refresh); } },
    { key: 'theme', label: t('menu.view.theme'), children: [
      { key: 'theme-dark', label: t('menu.view.theme.dark'), onClick: () => setMode('dark'), selected: mode === 'dark' },
      { key: 'theme-light', label: t('menu.view.theme.light'), onClick: () => setMode('light'), selected: mode === 'light' },
      { key: 'theme-system', label: t('menu.view.theme.system'), onClick: () => setMode('system'), selected: mode === 'system' },
    ]},
    { key: 'language', label: t('menu.view.language'), children: [
      { key: 'lang-en', label: t('menu.view.language.en'), onClick: () => setLocale('en'), selected: locale === 'en' },
      { key: 'lang-zh', label: t('menu.view.language.zh'), onClick: () => setLocale('zh'), selected: locale === 'zh' },
    ]},
  ];

  const toolsMenu: MenuItemSpec[] = [
    { key: 'scan', label: t('menu.tools.scan'), shortcut: '⌘L', onClick: onScan },
    { key: 'add', label: t('menu.tools.add'), shortcut: '⌘I', onClick: () => openModal('add-device') },
    { key: 'settings', label: t('menu.tools.settings'), shortcut: '⌘,', onClick: () => openModal('settings') },
    { key: 'clear', label: t('menu.tools.clear'), onClick: onClearRegistry, danger: true },
  ];

  const helpMenu: MenuItemSpec[] = [
    { key: 'guide', label: t('menu.help.guide'), shortcut: 'F1', onClick: () => openModal('setup-guide') },
    { key: 'about', label: t('menu.help.about'), onClick: () => openModal('about') },
  ];

  const toAntdItems = (items: MenuItemSpec[]): any[] =>
    items.map((it) => {
      const hasChildren = Array.isArray(it.children) && it.children.length > 0;
      const label = (
        <span style={{ display: 'flex', justifyContent: 'space-between', gap: 24, alignItems: 'center' }}>
          <span style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 6,
            color: it.danger ? '#ff7875' : undefined,
            fontWeight: it.selected ? 600 : 400,
          }}>
            <span style={{
              display: 'inline-block',
              width: 12,
              color: it.selected ? 'var(--color-primary, #1677ff)' : 'transparent',
              fontWeight: 700,
            }}>✓</span>
            {it.label}
          </span>
          {it.shortcut && <span style={{ color: 'var(--text-secondary)', fontSize: 11 }}>{it.shortcut}</span>}
        </span>
      );
      if (hasChildren) {
        return {
          key: it.key,
          label,
          children: toAntdItems(it.children!),
        };
      }
      return {
        key: it.key,
        label,
        onClick: it.onClick,
        disabled: it.disabled,
      };
    });

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
          {makeTrigger(t('menu.file'), 'file')}
        </Dropdown>
        <Dropdown menu={{ items: toAntdItems(viewMenu) }} trigger={['click']} placement="bottomLeft">
          {makeTrigger(t('menu.view'), 'view')}
        </Dropdown>
        <Dropdown menu={{ items: toAntdItems(toolsMenu) }} trigger={['click']} placement="bottomLeft">
          {makeTrigger(t('menu.tools'), 'tools')}
        </Dropdown>
        <Dropdown menu={{ items: toAntdItems(helpMenu) }} trigger={['click']} placement="bottomLeft">
          {makeTrigger(t('menu.help'), 'help')}
        </Dropdown>
      </div>
    </>
  );
}