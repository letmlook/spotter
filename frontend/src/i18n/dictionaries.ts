// Translation dictionaries for the Spotter GUI. Add new keys here
// when introducing new user-visible strings; missing translations
// fall back to English so a half-translated release still works.

export type Locale = 'en' | 'zh';

export const dictionaries: Record<Locale, Record<string, string>> = {
  en: {
    // Title bar / brand
    'app.title': 'Spotter',

    // Menu bar
    'menu.file': 'File',
    'menu.file.quit': 'Quit',
    'menu.view': 'View',
    'menu.view.refresh': 'Refresh',
    'menu.view.clear': 'Clear registry…',
    'menu.view.theme': 'Theme',
    'menu.view.theme.dark': 'Dark',
    'menu.view.theme.light': 'Light',
    'menu.view.language': 'Language',
    'menu.view.language.en': 'English',
    'menu.view.language.zh': '中文',
    'menu.tools': 'Tools',
    'menu.tools.scan': 'Scan local subnet',
    'menu.tools.add': 'Add device by IP…',
    'menu.tools.guide': 'Device setup guide…',
    'menu.help': 'Help',
    'menu.help.about': 'About Spotter…',

    // Sidebar
    'sidebar.devices': 'Devices',
    'sidebar.scan': 'Scan local subnet',
    'sidebar.clear': 'Clear registry',

    // Empty state
    'empty.title': 'No device selected.',
    'empty.body': 'Use {scan} to discover devices on your LAN, or {add} to register one manually. Devices already running spotterd are also discovered automatically via multicast.',
    'empty.cta': 'How to install spotterd on a device',
    'empty.scan.shortcut': 'Tools → Scan local subnet',
    'empty.add.shortcut': 'Tools → Add device by IP',

    // DetailPanel / device actions
    'detail.status.online': 'online',
    'detail.status.offline': 'offline',
    'detail.actions': 'Device actions',
    'detail.refresh': 'Refresh',

    // Modals
    'modal.about.title': 'About',
    'modal.about.tagline': 'LAN device discovery for Linux devices',
    'modal.about.tagline2': '(ARM64 SBCs such as Jetson, plus AMD64 servers and workstations).',
    'modal.about.client': 'Client',
    'modal.about.agent': 'Agent',
    'modal.about.copyright': '© 2026 Spotter Dev',
    'modal.add.title': 'Add device by IP',
    'modal.add.body': 'Manually register a device when auto-discovery (multicast / subnet scan) is blocked.',
    'modal.add.ip': 'IP',
    'modal.add.port': 'HTTP port',
    'modal.add.username': 'Username',
    'modal.add.username.placeholder': 'optional label',
    'modal.add.submit': 'Add',
    'modal.add.cancel': 'Cancel',
    'modal.guide.title': 'How to install spotterd on a device',
    'modal.guide.body': "spotterd must be installed on each target device manually — the GUI only discovers and displays devices, it does not deploy or manage the agent. Run the steps below from your development machine.",
    'modal.guide.tab.arm64': 'arm64 (Jetson / SBC)',
    'modal.guide.tab.amd64': 'amd64 (server / PC)',
    'modal.guide.tab.verify': 'Verify / troubleshoot',
    'modal.guide.prereq': 'Prerequisites',
    'modal.guide.prereq.1': 'Target device runs Linux with systemd (Ubuntu / Jetson / Debian / RHEL)',
    'modal.guide.prereq.2': 'Target device is on the same L2 network as the GUI (for UDP multicast)',
    'modal.guide.prereq.3': 'SSH access to the target as a sudo-capable user',

    // Common buttons
    'common.cancel': 'Cancel',
    'common.submit': 'Submit',

    // Notifications
    'notif.scan.done': 'Scan complete',
    'notif.scan.fail': 'Scan failed',
    'notif.clear.confirm.title': 'Clear registry',
    'notif.clear.confirm.body': 'Remove every device from the local registry? Devices already running spotterd will be rediscovered automatically.',
    'notif.clear.confirm.ok': 'Clear',
  },

  zh: {
    'app.title': 'Spotter',

    'menu.file': '文件',
    'menu.file.quit': '退出',
    'menu.view': '视图',
    'menu.view.refresh': '刷新',
    'menu.view.clear': '清空注册表…',
    'menu.view.theme': '主题',
    'menu.view.theme.dark': '深色',
    'menu.view.theme.light': '浅色',
    'menu.view.language': '语言',
    'menu.view.language.en': 'English',
    'menu.view.language.zh': '中文',
    'menu.tools': '工具',
    'menu.tools.scan': '扫描本机子网',
    'menu.tools.add': '按 IP 添加设备…',
    'menu.tools.guide': '设备部署教程…',
    'menu.help': '帮助',
    'menu.help.about': '关于 Spotter…',

    'sidebar.devices': '设备',
    'sidebar.scan': '扫描本机子网',
    'sidebar.clear': '清空注册表',

    'empty.title': '未选中设备。',
    'empty.body': '使用 {scan} 发现局域网设备，或使用 {add} 手动注册。已运行 spotterd 的设备也会通过组播自动发现。',
    'empty.cta': '如何把 spotterd 安装到一台设备',
    'empty.scan.shortcut': '工具 → 扫描本机子网',
    'empty.add.shortcut': '工具 → 按 IP 添加设备',

    'detail.status.online': '在线',
    'detail.status.offline': '离线',
    'detail.actions': '设备操作',
    'detail.refresh': '刷新',

    'modal.about.title': '关于',
    'modal.about.tagline': '局域网设备发现工具，面向 Linux 设备',
    'modal.about.tagline2': '（ARM64 单板机，例如 Jetson，以及 AMD64 服务器与工作站）。',
    'modal.about.client': '客户端',
    'modal.about.agent': '设备端',
    'modal.about.copyright': '© 2026 Spotter Dev',
    'modal.add.title': '按 IP 添加设备',
    'modal.add.body': '当自动发现（组播 / 子网扫描）不通时手动注册一台设备。',
    'modal.add.ip': 'IP',
    'modal.add.port': 'HTTP 端口',
    'modal.add.username': '用户名',
    'modal.add.username.placeholder': '可选标签',
    'modal.add.submit': '添加',
    'modal.add.cancel': '取消',
    'modal.guide.title': '如何把 spotterd 安装到一台设备',
    'modal.guide.body': 'spotterd 必须由你在每台目标设备上手动安装 —— 客户端只负责发现和展示，不部署 / 不管理 agent。从你的开发机执行下面的步骤。',
    'modal.guide.tab.arm64': 'arm64（Jetson / SBC）',
    'modal.guide.tab.amd64': 'amd64（服务器 / PC）',
    'modal.guide.tab.verify': '验证 / 故障排查',
    'modal.guide.prereq': '前置条件',
    'modal.guide.prereq.1': '目标设备运行 Linux 且启用 systemd（Ubuntu / Jetson / Debian / RHEL）',
    'modal.guide.prereq.2': '目标设备与客户端在同一 L2 网络（UDP 组播可达）',
    'modal.guide.prereq.3': '对目标设备有 sudo 能力的 SSH 账号',

    'common.cancel': '取消',
    'common.submit': '提交',

    'notif.scan.done': '扫描完成',
    'notif.scan.fail': '扫描失败',
    'notif.clear.confirm.title': '清空注册表',
    'notif.clear.confirm.body': '从本地注册表中移除所有设备？已运行 spotterd 的设备会自动重新发现。',
    'notif.clear.confirm.ok': '清空',
  },
};