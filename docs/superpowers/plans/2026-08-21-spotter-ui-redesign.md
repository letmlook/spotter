# Spotter UI 重新设计 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **⚠️ 历史文档（2026-08-21）—— 部分内容已被后续 commit 推翻。** 提交 `13598a4 refactor: drop deploy/uninstall-from-GUI` 移除了整个 deploy/uninstall 自动化路径。下面这些段落的"设计意图"已经过期，**实际代码以 `main.go` / 当前 `frontend/src/` 为准**：
>
> - **Task 10**（`ActionPanel` Deploy 表单 / SSH 凭据输入）—— 已移除
> - **Task 11**（`useDeviceActions` 中的 `deploy` / `uninstall` 入口）—— 已移除
> - **Task 12** 中 EmptyState 的 Deploy 按钮 / `onAction('deploy')` 联动 —— 已移除
> - **Task 13**（`DeviceActions` 卸载按钮 + 密码框）—— 已移除
>
> 其他 Task（TitleBar、Sidebar、DetailPanel、DetailPanel 的 Refresh、注册表清理、useWailsEvents、DeviceContext 等）仍然反映当前实现。此文档保留用于追溯当时的 UI 重构决策；如需重写，请参考 `docs/superpowers/specs/2026-08-21-spotter-design.md` 中已经更新的"7.7 cmd/client/main.go"和"8. 前端 UI"章节。

**Goal:** 将 Spotter Windows GUI 客户端从单 HTML + Vanilla JS 重构为 React + Vite + TypeScript + Ant Design 5，添加自定义无边框标题栏，所有用户输入改为界面内 inline 表单。

**Architecture:** Wails v2 内置 Vite 接管前端构建；后端 Go bindings 不变；前端拆为 TitleBar / Sidebar / MainArea 三大区，sidebar 顶部三个互斥展开的 inline 表单，DetailPanel 底部常驻操作条；状态用 Context + useReducer，事件订阅用 useEffect。

**Tech Stack:** Go 1.22+ / Wails v2.15 / React 18 / TypeScript 5 / Vite 5 / Ant Design 5 / @ant-design/icons

## Global Constraints

- 模块路径：`github.com/spotter/spotter`
- Wails 版本：v2.15.0
- Go 版本：1.22 或更新
- 前端目录：`frontend/`（从 `ui/` 迁移）
- 单一 Modal：仅清空注册表；其它输入全部 inline
- 自定义标题栏高 40px
- Sidebar 固定宽 260px
- 主题：AntD dark algorithm，主色 `#1677ff`
- 命名：React 组件 PascalCase，hooks camelCase，state interface 与 reducer action 同文件
- 提交：每次任务一个 commit
- 中文优先文案
- 不引入 Redux / Vitest（MVP 简化）
- 不引入 router / 多窗口 / i18n
- 保留 Go bindings 不变；不重写 Go 端业务逻辑

---

### Task 1: 初始化 frontend Vite 项目

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/tsconfig.json`
- Create: `frontend/tsconfig.node.json`
- Create: `frontend/index.html`
- Create: `frontend/src/main.tsx`
- Create: `frontend/src/App.tsx`
- Create: `frontend/src/styles.css`
- Create: `frontend/.gitignore`

**Interfaces:**
- Consumes: 现有的 `wails.json` `frontend:install` / `frontend:build` 字段
- Produces: 一个能被 `wails dev` / `wails build` 使用的最小 Vite + React + TS 项目骨架（仅显示 "Spotter" 字样）

- [ ] **Step 1: 写 `frontend/package.json`**

```json
{
  "name": "spotter-frontend",
  "private": true,
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "@ant-design/icons": "^5.5.0",
    "antd": "^5.21.0",
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@types/react": "^18.3.5",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "typescript": "^5.5.4",
    "vite": "^5.4.6"
  }
}
```

- [ ] **Step 2: 写 `frontend/vite.config.ts`**

```ts
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
});
```

- [ ] **Step 3: 写 `frontend/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "useDefineForClassFields": true
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

- [ ] **Step 4: 写 `frontend/tsconfig.node.json`**

```json
{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts"]
}
```

- [ ] **Step 5: 写 `frontend/index.html`**

```html
<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Spotter</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 6: 写 `frontend/src/main.tsx`**

```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import { ConfigProvider, theme } from 'antd';
import App from './App';
import './styles.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider
      theme={{
        algorithm: theme.darkAlgorithm,
        token: { colorPrimary: '#1677ff', borderRadius: 6 },
      }}
    >
      <App />
    </ConfigProvider>
  </React.StrictMode>,
);
```

- [ ] **Step 7: 写 `frontend/src/App.tsx` 占位**

```tsx
export default function App() {
  return <div style={{ padding: 24 }}>Spotter</div>;
}
```

- [ ] **Step 8: 写 `frontend/src/styles.css` 占位**

```css
html, body, #root { height: 100%; margin: 0; }
body { background: #141414; color: #fff; font-family: -apple-system, system-ui, sans-serif; }
```

- [ ] **Step 9: 写 `frontend/.gitignore`**

```
node_modules/
dist/
```

- [ ] **Step 10: 安装依赖**

```bash
cd frontend && npm install
```

Expected: `node_modules/` 生成，无报错。

- [ ] **Step 11: 验证构建**

```bash
cd frontend && npm run build
```

Expected: 产出 `frontend/dist/index.html`，无 TS 错误。

- [ ] **Step 12: Commit**

```bash
git add frontend/package.json frontend/vite.config.ts frontend/tsconfig.json frontend/tsconfig.node.json frontend/index.html frontend/src/main.tsx frontend/src/App.tsx frontend/src/styles.css frontend/.gitignore frontend/package-lock.json
git commit -m "feat(ui): scaffold React + Vite + TS frontend with AntD dark theme"
```

---

### Task 2: Wails 切换到 frontend/ 输出（不改 Frameless）

**Files:**
- Modify: `main.go`（改 embed 路径）
- Modify: `wails.json`（确认 `frontend:install` / `frontend:build`）

**Interfaces:**
- Consumes: Task 1 产出的 `frontend/dist/`
- Produces: `wails build` 能从 `frontend/dist` 嵌入前端；旧的 `ui/` 暂未删除（保持二进制可编译）

- [ ] **Step 1: 修改 `main.go` 的 embed 指令**

把：
```go
//go:embed all:ui
var uiFS embed.FS
```

改为：
```go
//go:embed all:frontend/dist
var uiFS embed.FS
```

- [ ] **Step 2: 确认 `wails.json` 已有 `frontend:install` 和 `frontend:build`**

读 `wails.json`，确认包含：
```json
"frontend:install": "npm install",
"frontend:build": "npm run build",
```

如果有缺失则补全（参考 spec §12）。

- [ ] **Step 3: 验证编译**

```bash
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe 2>&1 | tail -10
```

Expected: 构建成功；新二进制能运行（双击启动后能看到 React 渲染的"Spotter"文字，深色背景）。

- [ ] **Step 4: 手动验证二进制**

启动 `bin/spotter-client.new.exe`（如文件锁则 mv 一份启动），确认：
1. 窗口仍然有原系统标题栏（Frameless 还没改）
2. 主区显示"Spotter"文字，深色背景

- [ ] **Step 5: Commit**

```bash
git add main.go wails.json
git commit -m "build: point Wails embed at frontend/dist output"
```

---

### Task 3: 启用 Wails Frameless + WindowIsMaximised runtime 函数

**Files:**
- Modify: `main.go`（添加 `Windows: &windows.Options{...}`，加 `Frameless: true`）

**Interfaces:**
- Consumes: Wails v2 `pkg/options/windows` 包
- Produces: 启动后窗口无原生 chrome；前端能用 `runtime.WindowMinimise` / `WindowToggleMaximise` / `WindowClose` 控制

- [ ] **Step 1: 添加 windows 包 import**

```go
import (
    // ...已有 imports
    "github.com/wailsapp/wails/v2/pkg/options/windows"
)
```

- [ ] **Step 2: 在 options.App 中加入 Windows + Frameless**

定位 `wails.Run(&options.App{ ... })`，把 `OnStartup: app.OnStartup,` 之后、`Bind: []interface{}{` 之前插入：

```go
Windows: &windows.Options{
    WebviewIsTransparent: false,
    DisableWindowIcon:    true,
},
Frameless: true,
```

完整 options.App 应包含 `Title`、`Width`、`Height`、`AssetServer`、`OnStartup`、`Bind`、新增的 `Windows` 和 `Frameless`。

- [ ] **Step 3: 验证编译**

```bash
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe 2>&1 | tail -5
```

Expected: 编译成功。

- [ ] **Step 4: 手动验证**

启动二进制：窗口应无任何原生 chrome（没有标题栏、没有最大/最小/关闭按钮）；内容区显示 React 的"Spotter"。窗口无法拖动也无法关闭。

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "feat(client): enable frameless window for custom title bar"
```

---

### Task 4: TitleBar 组件（无功能骨架）

**Files:**
- Create: `frontend/src/components/TitleBar.tsx`

**Interfaces:**
- Consumes: 无（纯 UI）
- Produces: 一个 40px 高的标题栏（无功能），App.tsx 中显示在顶部

- [ ] **Step 1: 写 `frontend/src/components/TitleBar.tsx`**

```tsx
import { MinusOutlined, BorderOutlined, CloseOutlined } from '@ant-design/icons';
import styles from './TitleBar.module.css';

export default function TitleBar() {
  return (
    <div className={styles.bar}>
      <div className={styles.left}>
        <span className={styles.logo}>●</span>
        <span className={styles.title}>Spotter</span>
      </div>
      <div className={styles.middle} />
      <div className={styles.right}>
        <button className={styles.btn} aria-label="minimise"><MinusOutlined /></button>
        <button className={styles.btn} aria-label="toggle maximise"><BorderOutlined /></button>
        <button className={`${styles.btn} ${styles.close}`} aria-label="close"><CloseOutlined /></button>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: 写 `frontend/src/components/TitleBar.module.css`**

```css
.bar {
  display: flex;
  align-items: center;
  height: 40px;
  background: #0a0a0a;
  border-bottom: 1px solid #303030;
  user-select: none;
  flex-shrink: 0;
}
.left {
  display: flex; align-items: center; gap: 8px;
  padding: 0 12px;
  width: 200px;
  -webkit-app-region: no-drag;
}
.logo { color: #1677ff; font-size: 18px; }
.title { color: #fff; font-weight: 600; font-size: 14px; }
.middle {
  flex: 1;
  height: 100%;
  -webkit-app-region: drag;
}
.right {
  display: flex; align-items: center;
  -webkit-app-region: no-drag;
}
.btn {
  width: 46px; height: 40px;
  border: 0; background: transparent;
  color: #aaa; font-size: 14px;
  cursor: pointer;
  display: inline-flex; align-items: center; justify-content: center;
}
.btn:hover { background: #1f1f1f; color: #fff; }
.close:hover { background: #c41a1a; color: #fff; }
```

- [ ] **Step 3: 修改 `frontend/src/App.tsx` 引入 TitleBar**

```tsx
import TitleBar from './components/TitleBar';

export default function App() {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ flex: 1, padding: 24 }}>Spotter</div>
    </div>
  );
}
```

- [ ] **Step 4: 验证构建 + 运行**

```bash
cd frontend && npm run build
```

Expected: TS 无错。

- [ ] **Step 5: 重建二进制 + 验证**

```bash
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe
```

启动二进制：自定义标题栏显示，左 logo+标题，中段空白，右侧 3 个按钮（min / max / close）。**中段拖动应可拖窗口**，按钮区 hover 高亮，关闭按钮 hover 变红。按钮本身还无功能。

- [ ] **Step 6: Commit**

```bash
git add frontend/src/
git commit -m "feat(ui): TitleBar with drag region + window control buttons (visual)"
```

---

### Task 5: TitleBar 按钮接线 window 控制

**Files:**
- Modify: `frontend/src/components/TitleBar.tsx`

**Interfaces:**
- Consumes: `frontend/wailsjs/runtime/runtime.d.ts` 中的 `WindowMinimise` / `WindowToggleMaximise` / `WindowClose`
- Produces: 3 个按钮能控制窗口

- [ ] **Step 1: 检查 runtime.d.ts 是否导出这三个函数**

```bash
grep -E "WindowMinimise|WindowToggleMaximise|WindowClose" frontend/wailsjs/runtime/runtime.d.ts
```

Expected: 三个函数都存在。如果不存在则 `wails build` 会重新生成，先跑一次 `wails generate module`。

- [ ] **Step 2: 修改 TitleBar.tsx 加入 onClick**

```tsx
import { MinusOutlined, BorderOutlined, CloseOutlined } from '@ant-design/icons';
import {
  WindowMinimise,
  WindowToggleMaximise,
  WindowClose,
} from '../../wailsjs/runtime/runtime';
import styles from './TitleBar.module.css';

export default function TitleBar() {
  return (
    <div className={styles.bar}>
      <div className={styles.left}>
        <span className={styles.logo}>●</span>
        <span className={styles.title}>Spotter</span>
      </div>
      <div className={styles.middle} />
      <div className={styles.right}>
        <button className={styles.btn} aria-label="minimise" onClick={WindowMinimise}>
          <MinusOutlined />
        </button>
        <button className={styles.btn} aria-label="toggle maximise" onClick={WindowToggleMaximise}>
          <BorderOutlined />
        </button>
        <button
          className={`${styles.btn} ${styles.close}`}
          aria-label="close"
          onClick={WindowClose}
        >
          <CloseOutlined />
        </button>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: 重新编译 + 验证**

```bash
cd frontend && npm run build
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe
```

启动二进制，点 3 个按钮：
- `−` → 窗口最小化
- `□` → 切换最大化 / 还原
- `×` → 关闭窗口（程序退出）

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/TitleBar.tsx
git commit -m "feat(ui): wire TitleBar buttons to Wails window controls"
```

---

### Task 6: DeviceContext 状态管理

**Files:**
- Create: `frontend/src/state/DeviceContext.tsx`

**Interfaces:**
- Consumes: Wails binding `ListDevices()` 返回 `RegistryEntry[]`
- Produces: `useDevices()` hook（返回 `{state, dispatch, refresh()}`），Provider 包整个 App

`RegistryEntry` 字段（snake_case）：`device_id`、`ip`、`port`、`username`、`deployed_at`、`last_seen_at`、`last_source`、`online`、`last_info`（可选，含 `basic`、`network`、`jetson`）

- [ ] **Step 1: 写 `frontend/src/state/DeviceContext.tsx`**

```tsx
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
    case 'SET_DEVICES':
      return { ...state, devices: action.devices };
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
```

- [ ] **Step 2: 在 main.tsx 包 Provider**

修改 `frontend/src/main.tsx`，在 `<App />` 外包 `<DeviceProvider>`：

```tsx
import { DeviceProvider } from './state/DeviceContext';

// ... 其它已有代码

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider theme={...}>
      <DeviceProvider>
        <App />
      </DeviceProvider>
    </ConfigProvider>
  </React.StrictMode>,
);
```

- [ ] **Step 3: 验证 TS 编译**

```bash
cd frontend && npm run build
```

Expected: 无 TS 错误。

- [ ] **Step 4: Commit**

```bash
git add frontend/src/state/DeviceContext.tsx frontend/src/main.tsx
git commit -m "feat(ui): DeviceContext state with ListDevices polling"
```

---

### Task 7: useWailsEvents hook

**Files:**
- Create: `frontend/src/hooks/useWailsEvents.ts`

**Interfaces:**
- Consumes: `EventsOn` from `../../wailsjs/runtime/runtime`
- Produces: hook 在组件挂载时订阅 3 个事件（info-updated / offline / unknown-device），卸载时清理

- [ ] **Step 1: 写 hook**

```ts
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
```

- [ ] **Step 2: 验证 TS**

```bash
cd frontend && npm run build
```

Expected: 无错。

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/useWailsEvents.ts
git commit -m "feat(ui): useWailsEvents hook with silent accept"
```

---

### Task 8: Sidebar 布局 + DeviceList

**Files:**
- Create: `frontend/src/components/Sidebar.tsx`
- Create: `frontend/src/components/DeviceList.tsx`
- Create: `frontend/src/components/DeviceRow.tsx`
- Create: `frontend/src/components/Sidebar.module.css`

**Interfaces:**
- Consumes: `useDevices()` 提供 devices / selectedId / searchQuery
- Produces: 260px 宽 sidebar，含搜索框 + 设备列表（含状态点）

- [ ] **Step 1: 写 DeviceRow.tsx**

```tsx
import { Tag } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import styles from './DeviceRow.module.css';

export default function DeviceRow({
  device,
  selected,
  onClick,
}: {
  device: RegistryEntry;
  selected: boolean;
  onClick: () => void;
}) {
  const hostname = device.last_info?.basic?.hostname || '';
  const isJetson = !!device.last_info?.jetson?.model;
  return (
    <div
      className={`${styles.row} ${selected ? styles.selected : ''}`}
      onClick={onClick}
    >
      <span className={`${styles.dot} ${device.online ? styles.online : styles.offline}`} />
      <div className={styles.text}>
        <div className={styles.ip}>{device.ip}</div>
        <div className={styles.sub}>
          {hostname || '—'}
          {device.username && <> · {device.username}</>}
        </div>
      </div>
      {isJetson && <Tag color="orange" style={{ marginRight: 0 }}>Jetson</Tag>}
    </div>
  );
}
```

- [ ] **Step 2: 写 DeviceRow.module.css**

```css
.row {
  display: flex; align-items: center; gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  border-radius: 4px;
  margin: 2px 0;
}
.row:hover { background: #262626; }
.selected { background: #1f3a5f; }
.selected:hover { background: #1f3a5f; }
.dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
.online { background: #52c41a; box-shadow: 0 0 6px #52c41a; }
.offline { background: #b71c1c; }
.text { flex: 1; min-width: 0; }
.ip { color: #fff; font-weight: 500; font-size: 13px; }
.sub { color: #888; font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
```

- [ ] **Step 3: 写 DeviceList.tsx**

```tsx
import { Empty, Input } from 'antd';
import { useDevices } from '../state/DeviceContext';
import DeviceRow from './DeviceRow';

export default function DeviceList() {
  const { state, dispatch } = useDevices();
  const filtered = state.devices.filter((d) => {
    if (!state.searchQuery) return true;
    const q = state.searchQuery.toLowerCase();
    return d.ip.toLowerCase().includes(q) ||
      (d.last_info?.basic?.hostname || '').toLowerCase().includes(q) ||
      d.username.toLowerCase().includes(q);
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
      <div style={{ padding: '8px 12px' }}>
        <Input.Search
          placeholder="Search devices"
          allowClear
          size="small"
          value={state.searchQuery}
          onChange={(e) => dispatch({ type: 'SEARCH', query: e.target.value })}
        />
      </div>
      <div style={{ flex: 1, overflow: 'auto', padding: '0 8px' }}>
        {filtered.length === 0 ? (
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={state.devices.length === 0 ? 'No devices' : 'No matches'}
            style={{ marginTop: 24 }}
          />
        ) : (
          filtered.map((d) => (
            <DeviceRow
              key={d.device_id}
              device={d}
              selected={state.selectedId === d.device_id}
              onClick={() => dispatch({ type: 'SELECT', id: d.device_id })}
            />
          ))
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: 写 Sidebar.tsx**

```tsx
import { Button, Popconfirm } from 'antd';
import { DeleteOutlined } from '@ant-design/icons';
import { ClearRegistry } from '../../wailsjs/go/main/App';
import { useDevices } from '../state/DeviceContext';
import DeviceList from './DeviceList';

export default function Sidebar() {
  const { state, refresh } = useDevices();

  return (
    <aside
      style={{
        width: 260, flexShrink: 0,
        background: '#0a0a0a', borderRight: '1px solid #303030',
        display: 'flex', flexDirection: 'column',
      }}
    >
      <div style={{ padding: '8px 12px', borderBottom: '1px solid #303030' }}>
        <Popconfirm
          title="Clear registry"
          description={`Remove all ${state.devices.length} device(s) from the local registry? This does NOT touch remote devices — use Uninstall for that.`}
          okText="Clear"
          cancelText="Cancel"
          onConfirm={async () => { await ClearRegistry(); await refresh(); }}
          disabled={state.devices.length === 0}
        >
          <Button
            danger
            icon={<DeleteOutlined />}
            block
            disabled={state.devices.length === 0}
          >
            Clear registry
          </Button>
        </Popconfirm>
      </div>
      <DeviceList />
    </aside>
  );
}
```

- [ ] **Step 5: 修改 App.tsx 引入 Sidebar**

```tsx
import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import { useWailsEvents } from './hooks/useWailsEvents';

export default function App() {
  useWailsEvents();
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Sidebar />
        <div style={{ flex: 1, padding: 24 }}>Main area placeholder</div>
      </div>
    </div>
  );
}
```

- [ ] **Step 6: 编译 + 重建 + 验证**

```bash
cd frontend && npm run build
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe
```

启动二进制确认：标题栏 + 左侧 sidebar（含 Clear 按钮 + 搜索框 + 空状态 Empty 组件）+ 主区占位文字。

- [ ] **Step 7: Commit**

```bash
git add frontend/src/
git commit -m "feat(ui): Sidebar with search, device list, status dots, Clear button"
```

---

### Task 9: DetailPanel + 三张卡片

**Files:**
- Create: `frontend/src/components/DetailPanel.tsx`
- Create: `frontend/src/components/BasicCard.tsx`
- Create: `frontend/src/components/NetworkCard.tsx`
- Create: `frontend/src/components/JetsonCard.tsx`
- Create: `frontend/src/components/EmptyState.tsx`
- Create: `frontend/src/utils/format.ts`

**Interfaces:**
- Consumes: `useDevices()` 拿 `state.devices` 和 `selectedId`
- Produces: 主区右侧 3 张 Card（Basic / Network / Jetson），未选中时显示 EmptyState

- [ ] **Step 1: 写 `frontend/src/utils/format.ts`**

```ts
export function formatUptime(seconds?: number | null): string | null {
  if (seconds == null) return null;
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
```

- [ ] **Step 2: 写 BasicCard.tsx**

```tsx
import { Card, Descriptions } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';
import { formatUptime } from '../utils/format';

export default function BasicCard({ device }: { device: RegistryEntry }) {
  const info = device.last_info;
  if (!info) {
    return (
      <Card title="Basic" size="small">
        <span style={{ color: '#888' }}>{device.online ? 'Polling…' : 'No info yet'}</span>
      </Card>
    );
  }
  const b = info.basic || {};
  const os = b.os || {};
  const items = [
    { key: 'h', label: 'Hostname', children: b.hostname || '—' },
    { key: 'u', label: 'Username', children: b.username || '—' },
    { key: 'os', label: 'OS', children: os.pretty_name || '—' },
    { key: 'dist', label: 'Distribution', children: (os.id ? `${os.id} ${os.version_id || ''}`.trim() : '—') },
    { key: 'k', label: 'Kernel', children: b.kernel || '—' },
    { key: 'a', label: 'Arch', children: b.arch || '—' },
    { key: 'up', label: 'Uptime', children: formatUptime(b.uptime_seconds) || '—' },
    { key: 'c', label: 'Collected at', children: info.collected_at || '—' },
    { key: 'v', label: 'Agent version', children: info.agent_version || '—' },
    { key: 'id', label: 'Device ID', children: info.device_id || '—' },
  ];
  return (
    <Card title="Basic" size="small">
      <Descriptions column={1} size="small" colon={false} labelStyle={{ color: '#888', width: 130 }}>
        {items.map((it) => <Descriptions.Item key={it.key} label={it.label}>{it.children}</Descriptions.Item>)}
      </Descriptions>
    </Card>
  );
}
```

- [ ] **Step 3: 写 NetworkCard.tsx**

```tsx
import { Card, Table } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';

export default function NetworkCard({ device }: { device: RegistryEntry }) {
  const net = device.last_info?.network;
  const ifaces = net?.interfaces || [];
  return (
    <Card title="Network" size="small">
      {net?.primary_ip && (
        <div style={{ marginBottom: 12, color: '#ccc' }}>
          Primary IP:&nbsp;<strong style={{ color: '#fff' }}>{net.primary_ip}</strong>
        </div>
      )}
      {ifaces.length === 0 ? (
        <span style={{ color: '#888' }}>No network interfaces reported</span>
      ) : (
        <Table<{ name?: string; mac?: string; addrs?: string[] }>
          rowKey={(r) => r.name || Math.random().toString()}
          dataSource={ifaces}
          size="small"
          pagination={false}
          columns={[
            { title: 'Interface', dataIndex: 'name' },
            { title: 'MAC', dataIndex: 'mac' },
            {
              title: 'Addresses',
              dataIndex: 'addrs',
              render: (a?: string[]) => (a && a.length > 0 ? a.join(', ') : '—'),
            },
          ]}
        />
      )}
    </Card>
  );
}
```

- [ ] **Step 4: 写 JetsonCard.tsx**

```tsx
import { Card, Descriptions } from 'antd';
import type { RegistryEntry } from '../state/DeviceContext';

export default function JetsonCard({ device }: { device: RegistryEntry }) {
  const j = device.last_info?.jetson;
  if (!j) {
    return (
      <Card title="Jetson" size="small">
        <span style={{ color: '#888', fontStyle: 'italic' }}>Not a Jetson device or probe failed</span>
      </Card>
    );
  }
  const items = [
    { key: 'm', label: 'Model', v: j.model },
    { key: 'j', label: 'JetPack', v: j.jetpack },
    { key: 'l', label: 'L4T', v: j.l4t },
    { key: 'c', label: 'CUDA', v: j.cuda },
    { key: 'd', label: 'cuDNN', v: j.cudnn },
    { key: 't', label: 'TensorRT', v: j.tensorrt },
    { key: 'p', label: 'Python', v: j.python },
    { key: 's', label: 'Serial', v: j.serial },
  ].filter((it) => it.v);
  if (items.length === 0) {
    return (
      <Card title="Jetson" size="small">
        <span style={{ color: '#888', fontStyle: 'italic' }}>No Jetson probes succeeded</span>
      </Card>
    );
  }
  return (
    <Card title="Jetson" size="small">
      <Descriptions column={1} size="small" colon={false} labelStyle={{ color: '#888', width: 130 }}>
        {items.map((it) => <Descriptions.Item key={it.key} label={it.label}>{it.v}</Descriptions.Item>)}
      </Descriptions>
    </Card>
  );
}
```

- [ ] **Step 5: 写 EmptyState.tsx**

```tsx
import { Empty, Button, Space } from 'antd';
import { PlusOutlined, ScanOutlined, ImportOutlined } from '@ant-design/icons';

export default function EmptyState({ onAction }: { onAction: (which: 'deploy' | 'scan' | 'add') => void }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No device selected. Use the toolbar to discover or add one."
      >
        <Space>
          <Button icon={<PlusOutlined />} onClick={() => onAction('deploy')}>Deploy</Button>
          <Button icon={<ScanOutlined />} onClick={() => onAction('scan')}>Scan subnet</Button>
          <Button icon={<ImportOutlined />} onClick={() => onAction('add')}>Add by IP</Button>
        </Space>
      </Empty>
    </div>
  );
}
```

- [ ] **Step 6: 写 DetailPanel.tsx**

```tsx
import { useDevices } from '../state/DeviceContext';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';

export default function DetailPanel({ onAction }: { onAction: (which: 'deploy' | 'scan' | 'add') => void }) {
  const { state } = useDevices();
  const device = state.devices.find((d) => d.device_id === state.selectedId);

  if (!device) return <EmptyState onAction={onAction} />;

  const hostname = device.last_info?.basic?.hostname || device.ip;
  const statusClass = device.online ? 'online' : 'offline';
  const statusText = device.online ? 'online' : 'offline';

  return (
    <div style={{ padding: 16, overflow: 'auto', height: '100%' }}>
      <h2 style={{ margin: '0 0 16px 0', color: '#fff' }}>
        {hostname}
        <span className={statusClass} style={{ marginLeft: 12, fontSize: 13 }}>{statusText}</span>
      </h2>
      <div style={{ display: 'grid', gap: 12 }}>
        <BasicCard device={device} />
        <NetworkCard device={device} />
        <JetsonCard device={device} />
      </div>
    </div>
  );
}
```

- [ ] **Step 7: 修改 App.tsx 接入 DetailPanel**

```tsx
import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import DetailPanel from './components/DetailPanel';
import { useWailsEvents } from './hooks/useWailsEvents';

export default function App() {
  useWailsEvents();
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Sidebar />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, background: '#141414' }}>
          <DetailPanel onAction={() => { /* placeholder until ActionPanel */ }} />
        </main>
      </div>
    </div>
  );
}
```

- [ ] **Step 8: 编译 + 重建 + 验证**

```bash
cd frontend && npm run build
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe
```

启动二进制确认：
- 选中一个已注册设备 → 右侧显示 hostname + online badge + 3 张卡片
- 未选中时 → 显示 EmptyState 插画 + 三个引导按钮（Deploy/Scan/Add by IP，但按钮暂不工作）
- 卡片字段缺失时显示 `—`；Jetson 卡如果设备非 Jetson 显示提示语

- [ ] **Step 9: Commit**

```bash
git add frontend/src/
git commit -m "feat(ui): DetailPanel with Basic/Network/Jetson cards + EmptyState"
```

---

### Task 10: ActionPanel inline 表单

**Files:**
- Create: `frontend/src/components/ActionPanel.tsx`

**Interfaces:**
- Consumes: `useDeviceActions()` hook（Task 11 中实现）提供的 deploy / scan / add 方法；`useState<activeForm>` 控制展开
- Produces: sidebar 顶部三个互斥的 inline 表单

- [ ] **Step 1: 写 ActionPanel.tsx**

```tsx
import { useState } from 'react';
import { Button, Form, Input, Space, Alert } from 'antd';
import { PlusOutlined, ScanOutlined, ImportOutlined, CloseOutlined } from '@ant-design/icons';
import { useDeviceActions } from '../hooks/useDeviceActions';
import { useDevices } from '../state/DeviceContext';

type ActiveForm = null | 'deploy' | 'scan' | 'add';

export default function ActionPanel() {
  const [active, setActive] = useState<ActiveForm>(null);
  const [error, setError] = useState<string | null>(null);
  const { refresh } = useDevices();
  const actions = useDeviceActions();
  const [busy, setBusy] = useState(false);

  const close = () => { setActive(null); setError(null); };

  return (
    <div style={{ borderBottom: '1px solid #303030' }}>
      <Space.Compact block>
        <Button
          type={active === 'deploy' ? 'primary' : 'default'}
          icon={<PlusOutlined />}
          onClick={() => setActive(active === 'deploy' ? null : 'deploy')}
          block
        >
          Deploy
        </Button>
        <Button
          type={active === 'scan' ? 'primary' : 'default'}
          icon={<ScanOutlined />}
          onClick={() => setActive(active === 'scan' ? null : 'scan')}
        >
          Scan
        </Button>
        <Button
          type={active === 'add' ? 'primary' : 'default'}
          icon={<ImportOutlined />}
          onClick={() => setActive(active === 'add' ? null : 'add')}
        >
          Add
        </Button>
      </Space.Compact>

      {error && <Alert type="error" message={error} closable onClose={() => setError(null)} style={{ margin: 8 }} />}

      {active === 'deploy' && (
        <Form
          layout="vertical" size="small"
          initialValues={{ port: 22, username: 'fitow' }}
          disabled={busy}
          onFinish={async (vals: { ip: string; port: number; username: string; password: string }) => {
            setBusy(true); setError(null);
            try {
              await actions.deploy(vals.ip, vals.port, vals.username, vals.password);
              await refresh();
              close();
            } catch (e: unknown) { setError(String(e)); }
            finally { setBusy(false); }
          }}
          style={{ padding: 12 }}
        >
          <Form.Item label="IP" name="ip" rules={[{ required: true, pattern: /^(\d{1,3}\.){3}\d{1,3}$/, message: 'Invalid IPv4' }]}>
            <Input placeholder="10.10.9.165" autoFocus />
          </Form.Item>
          <Form.Item label="SSH port" name="port" rules={[{ required: true, type: 'number', min: 1, max: 65535 }]}>
            <Input type="number" />
          </Form.Item>
          <Form.Item label="Username" name="username" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item label="Password" name="password" rules={[{ required: true }]}>
            <Input.Password />
          </Form.Item>
          <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
            <Button onClick={close} icon={<CloseOutlined />}>Cancel</Button>
            <Button type="primary" htmlType="submit" loading={busy}>Deploy</Button>
          </Space>
        </Form>
      )}

      {active === 'scan' && (
        <Form
          layout="vertical" size="small" disabled={busy}
          onFinish={async (vals: { cidr: string }) => {
            setBusy(true); setError(null);
            try {
              await actions.scan(vals.cidr);
              await refresh();
              close();
            } catch (e: unknown) { setError(String(e)); }
            finally { setBusy(false); }
          }}
          style={{ padding: 12 }}
        >
          <Form.Item label="CIDR" name="cidr" rules={[{ required: true, pattern: /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/, message: 'Invalid CIDR' }]}>
            <Input placeholder="192.168.1.0/24" autoFocus />
          </Form.Item>
          <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
            <Button onClick={close} icon={<CloseOutlined />}>Cancel</Button>
            <Button type="primary" htmlType="submit" loading={busy}>Scan</Button>
          </Space>
        </Form>
      )}

      {active === 'add' && (
        <Form
          layout="vertical" size="small" disabled={busy}
          initialValues={{ port: 9999, username: 'fitow' }}
          onFinish={async (vals: { ip: string; port: number; username: string }) => {
            setBusy(true); setError(null);
            try {
              await actions.add(vals.ip, vals.port, vals.username);
              await refresh();
              close();
            } catch (e: unknown) { setError(String(e)); }
            finally { setBusy(false); }
          }}
          style={{ padding: 12 }}
        >
          <Form.Item label="IP" name="ip" rules={[{ required: true, pattern: /^(\d{1,3}\.){3}\d{1,3}$/ }]}>
            <Input placeholder="10.10.9.165" autoFocus />
          </Form.Item>
          <Form.Item label="HTTP port" name="port" rules={[{ required: true, type: 'number', min: 1, max: 65535 }]}>
            <Input type="number" />
          </Form.Item>
          <Form.Item label="Username" name="username" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Space style={{ width: '100%', justifyContent: 'flex-end' }}>
            <Button onClick={close} icon={<CloseOutlined />}>Cancel</Button>
            <Button type="primary" htmlType="submit" loading={busy}>Add</Button>
          </Space>
        </Form>
      )}
    </div>
  );
}
```

- [ ] **Step 2: 编译（会失败，依赖 useDeviceActions）**

```bash
cd frontend && npm run build
```

Expected: 报错，提示 useDeviceActions 不存在。这步是为了明确 Task 11 的接口需求。

- [ ] **Step 3: Commit（提交不编译的中间状态 OK，因为下一步会补齐）**

```bash
git add frontend/src/components/ActionPanel.tsx
git commit -m "feat(ui): ActionPanel inline forms (deploy/scan/add) - skeleton"
```

---

### Task 11: useDeviceActions hook

**Files:**
- Create: `frontend/src/hooks/useDeviceActions.ts`

**Interfaces:**
- Consumes: Wails bindings `DeployDevice` / `UninstallDevice` / `ScanSubnet` / `ProbeByIP` / `RefreshNow`
- Produces: `useDeviceActions()` hook 返回 `{ deploy, scan, add, refresh, uninstall }` 方法（都是 async）

- [ ] **Step 1: 写 hook**

```ts
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
      await UninstallDevice(deviceId, password);
    },
  };
}
```

注：`username` 在 deploy/add/uninstall 路径暂时未使用（deploy 时 `main.go` 内部用 `deployUsername` 常量；add 路径的 username 通过 registry 存储；uninstall 路径目前直接用 registry 里的 username）。这个签名是为了后续扩展，调用方传 username 不报错。

- [ ] **Step 2: 编译验证（ActionPanel 现在能解析）**

```bash
cd frontend && npm run build
```

Expected: 无错。

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/useDeviceActions.ts
git commit -m "feat(ui): useDeviceActions hook wiring Wails bindings"
```

---

### Task 12: 接入 ActionPanel 到 Sidebar

**Files:**
- Modify: `frontend/src/components/Sidebar.tsx`
- Modify: `frontend/src/components/DetailPanel.tsx`（从 App 接收 onAction 转发到 EmptyState）

**Interfaces:**
- Consumes: ActionPanel 输出的内联表单；`EmptyState` 接收 onAction 回调
- Produces: Sidebar 顶部三个互斥表单 + EmptyState 引导按钮联动（点 Deploy 同时展开 Sidebar 内的 Deploy 表单）

- [ ] **Step 1: 修改 Sidebar.tsx 在 Clear 按钮之上插入 ActionPanel**

```tsx
import { Button, Popconfirm } from 'antd';
import { DeleteOutlined } from '@ant-design/icons';
import { ClearRegistry } from '../../wailsjs/go/main/App';
import { useDevices } from '../state/DeviceContext';
import ActionPanel from './ActionPanel';
import DeviceList from './DeviceList';

export default function Sidebar() {
  const { state, refresh } = useDevices();

  return (
    <aside
      style={{
        width: 260, flexShrink: 0,
        background: '#0a0a0a', borderRight: '1px solid #303030',
        display: 'flex', flexDirection: 'column',
      }}
    >
      <ActionPanel />
      <div style={{ padding: '8px 12px', borderBottom: '1px solid #303030' }}>
        <Popconfirm
          title="Clear registry"
          description={`Remove all ${state.devices.length} device(s) from the local registry? This does NOT touch remote devices — use Uninstall for that.`}
          okText="Clear"
          cancelText="Cancel"
          onConfirm={async () => { await ClearRegistry(); await refresh(); }}
          disabled={state.devices.length === 0}
        >
          <Button
            danger
            icon={<DeleteOutlined />}
            block
            disabled={state.devices.length === 0}
          >
            Clear registry
          </Button>
        </Popconfirm>
      </div>
      <DeviceList />
    </aside>
  );
}
```

- [ ] **Step 2: 修改 App.tsx 让 EmptyState 引导按钮联动 ActionPanel**

提取 ActionPanel 引用并暴露 activeForm setter；改造 App.tsx：

```tsx
import { useRef, useState } from 'react';
import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import DetailPanel from './components/DetailPanel';
import { useWailsEvents } from './hooks/useWailsEvents';

export default function App() {
  useWailsEvents();
  const [triggerForm, setTriggerForm] = useState<null | 'deploy' | 'scan' | 'add'>(null);
  const triggerRef = useRef<(which: 'deploy' | 'scan' | 'add') => void>(() => {});

  // Expose a global handler that Sidebar's ActionPanel registers into.
  // Simpler: lift ActionPanel into App and pass setter to both Sidebar and DetailPanel.
  // ...
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Sidebar onTriggerForm={(which) => setTriggerForm(which)} externalTrigger={triggerForm} onConsumed={() => setTriggerForm(null)} />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, background: '#141414' }}>
          <DetailPanel onAction={(which) => setTriggerForm(which)} />
        </main>
      </div>
    </div>
  );
}
```

实操简化：把 ActionPanel 状态提升到 App，由 App 持有 activeForm 并向下传。但 Sidebar 当前没接 props。为避免破坏 ActionPanel 的内部 state，简单方案——**让 DetailPanel 的 EmptyState 引导按钮直接 navigate 用户到 Sidebar 区域**（不强制展开），仅显示一行提示 "Use the sidebar toolbar to deploy, scan, or add a device"。把 onAction prop 改成 no-op。

修改 DetailPanel.tsx 删除 onAction prop：

```tsx
import { useDevices } from '../state/DeviceContext';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';

export default function DetailPanel() {
  const { state } = useDevices();
  const device = state.devices.find((d) => d.device_id === state.selectedId);
  if (!device) return <EmptyState />;

  const hostname = device.last_info?.basic?.hostname || device.ip;
  return (
    <div style={{ padding: 16, overflow: 'auto', height: '100%' }}>
      <h2 style={{ margin: '0 0 16px 0', color: '#fff' }}>
        {hostname}
        <span style={{ marginLeft: 12, fontSize: 13 }} className={device.online ? 'online' : 'offline'}>
          {device.online ? 'online' : 'offline'}
        </span>
      </h2>
      <div style={{ display: 'grid', gap: 12 }}>
        <BasicCard device={device} />
        <NetworkCard device={device} />
        <JetsonCard device={device} />
      </div>
    </div>
  );
}
```

修改 EmptyState.tsx 删除 onAction 参数：

```tsx
import { Empty, Typography } from 'antd';

export default function EmptyState() {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100%' }}>
      <Empty
        image={Empty.PRESENTED_IMAGE_SIMPLE}
        description="No device selected. Use the sidebar toolbar to deploy, scan a subnet, or add by IP."
      >
        <Typography.Text type="secondary">
          Discovered devices from multicast or subnet scans are added automatically.
        </Typography.Text>
      </Empty>
    </div>
  );
}
```

修改 App.tsx 删除 onAction 传递：

```tsx
import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import DetailPanel from './components/DetailPanel';
import { useWailsEvents } from './hooks/useWailsEvents';

export default function App() {
  useWailsEvents();
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Sidebar />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, background: '#141414' }}>
          <DetailPanel />
        </main>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: 编译 + 重建 + 验证**

```bash
cd frontend && npm run build
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe
```

启动二进制确认：
1. Sidebar 顶部三个按钮（Deploy / Scan / Add），点击互斥展开 inline 表单
2. 表单字段验证生效（IP 格式错 → 字段下方红字）
3. 取消按钮收起表单，再次点同一按钮重新展开（之前输入的值仍在）
4. Deploy 提交后如果失败，Alert 显示错误（如果成功会自动收起）
5. 选中设备时主区显示三张卡；未选中显示简化版 EmptyState

- [ ] **Step 4: Commit**

```bash
git add frontend/src/
git commit -m "feat(ui): wire ActionPanel into Sidebar, simplify EmptyState"
```

---

### Task 13: DeviceActions 底部操作条（卸载 + 刷新）

**Files:**
- Create: `frontend/src/components/DeviceActions.tsx`

**Interfaces:**
- Consumes: `useDevices()` 的 selectedId + username；`useDeviceActions().uninstall/refresh`
- Produces: 详情面板底部常驻的 inline 操作条（密码输入 + Refresh + Uninstall 按钮）

- [ ] **Step 1: 写 DeviceActions.tsx**

```tsx
import { useEffect, useState } from 'react';
import { Button, Input, Space, message } from 'antd';
import { ReloadOutlined, DeleteOutlined } from '@ant-design/icons';
import { useDevices } from '../state/DeviceContext';
import { useDeviceActions } from '../hooks/useDeviceActions';

export default function DeviceActions() {
  const { state, refresh } = useDevices();
  const actions = useDeviceActions();
  const [password, setPassword] = useState('');
  const [username, setUsername] = useState('');
  const [busy, setBusy] = useState(false);

  const device = state.devices.find((d) => d.device_id === state.selectedId);

  // Clear password & username on selection change.
  useEffect(() => {
    setPassword('');
    setUsername('');
  }, [state.selectedId]);

  if (!device) return null;

  const onRefresh = async () => {
    setBusy(true);
    try { await actions.refresh(); await refresh(); }
    finally { setBusy(false); }
  };

  const onUninstall = async () => {
    if (!password) { message.warning('Enter SSH password first'); return; }
    setBusy(true);
    try {
      await actions.uninstall(device.device_id, username || device.username, password);
      message.success(`Uninstalled ${device.ip}`);
      setPassword('');
      await refresh();
    } catch (e: unknown) {
      message.error(`Uninstall failed: ${e}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={{ borderTop: '1px solid #303030', padding: '12px 16px', background: '#0a0a0a' }}>
      <div style={{ marginBottom: 8, fontSize: 12, color: '#888', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        Device actions
      </div>
      <Space.Compact style={{ width: '100%' }}>
        <Input.Password
          placeholder="SSH password (not persisted)"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </Space.Compact>
      <Space style={{ marginTop: 8, width: '100%', justifyContent: 'space-between' }}>
        <Button icon={<ReloadOutlined />} onClick={onRefresh} disabled={busy}>
          Refresh
        </Button>
        <Button danger icon={<DeleteOutlined />} onClick={onUninstall} loading={busy} disabled={!password}>
          Uninstall spotterd
        </Button>
      </Space>
    </div>
  );
}
```

- [ ] **Step 2: 修改 DetailPanel.tsx 在底部加 DeviceActions**

```tsx
import { useDevices } from '../state/DeviceContext';
import BasicCard from './BasicCard';
import NetworkCard from './NetworkCard';
import JetsonCard from './JetsonCard';
import EmptyState from './EmptyState';
import DeviceActions from './DeviceActions';

export default function DetailPanel() {
  const { state } = useDevices();
  const device = state.devices.find((d) => d.device_id === state.selectedId);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {!device ? <EmptyState /> : (
          <>
            <h2 style={{ margin: '0 0 16px 0', color: '#fff' }}>
              {device.last_info?.basic?.hostname || device.ip}
              <span style={{ marginLeft: 12, fontSize: 13 }} className={device.online ? 'online' : 'offline'}>
                {device.online ? 'online' : 'offline'}
              </span>
            </h2>
            <div style={{ display: 'grid', gap: 12 }}>
              <BasicCard device={device} />
              <NetworkCard device={device} />
              <JetsonCard device={device} />
            </div>
          </>
        )}
      </div>
      {device && <DeviceActions />}
    </div>
  );
}
```

- [ ] **Step 3: 编译 + 重建 + 验证**

```bash
cd frontend && npm run build
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe
```

启动二进制确认：
1. 选中设备 → 详情面板底部出现 DeviceActions
2. 输入密码 → Uninstall 按钮从 disabled 变可点
3. Refresh 按钮调 Wails refresh → 状态更新
4. 切换设备 → 密码字段清空
5. 未选中设备 → 不显示 DeviceActions（仅 EmptyState）

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/DeviceActions.tsx frontend/src/components/DetailPanel.tsx
git commit -m "feat(ui): DeviceActions footer with Refresh and Uninstall buttons"
```

---

### Task 14: StatusBar + 整合完成

**Files:**
- Create: `frontend/src/components/StatusBar.tsx`
- Modify: `frontend/src/App.tsx`（添加 StatusBar）

**Interfaces:**
- Consumes: `useDevices()` 的 devices 数组
- Produces: 主区底部 24px 状态栏，显示 `online/total` 计数

- [ ] **Step 1: 写 StatusBar.tsx**

```tsx
import { useDevices } from '../state/DeviceContext';

export default function StatusBar() {
  const { state } = useDevices();
  const online = state.devices.filter((d) => d.online).length;
  const total = state.devices.length;
  return (
    <div
      style={{
        height: 24, flexShrink: 0,
        padding: '0 16px',
        display: 'flex', alignItems: 'center',
        background: '#0a0a0a', borderTop: '1px solid #303030',
        fontSize: 12, color: '#888',
      }}
    >
      {online} online / {total} total
      {state.loading && <span style={{ marginLeft: 12 }}>refreshing…</span>}
    </div>
  );
}
```

- [ ] **Step 2: 修改 App.tsx 接入 StatusBar**

```tsx
import TitleBar from './components/TitleBar';
import Sidebar from './components/Sidebar';
import DetailPanel from './components/DetailPanel';
import StatusBar from './components/StatusBar';
import { useWailsEvents } from './hooks/useWailsEvents';

export default function App() {
  useWailsEvents();
  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <TitleBar />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Sidebar />
        <main style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0, background: '#141414' }}>
          <DetailPanel />
          <StatusBar />
        </main>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: 编译 + 重建 + 验证**

```bash
cd frontend && npm run build
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe
```

启动二进制确认：
1. 底部状态栏显示 `N online / M total`
2. 刷新时显示 "refreshing…"
3. 整体布局：TitleBar 40 + Sidebar 260 / Main flex / StatusBar 24

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/StatusBar.tsx frontend/src/App.tsx
git commit -m "feat(ui): StatusBar with online/total counter"
```

---

### Task 15: 删除旧 ui/ + 验证清单

**Files:**
- Delete: `ui/index.html`
- Delete: `ui/app.js`
- Delete: `ui/styles.css`
- Modify: `main.go`（清理旧 embed 注释）

**Interfaces:**
- Consumes: Task 14 完成后的 frontend/ 可独立工作
- Produces: 仓库不再有旧 ui/ 残留

- [ ] **Step 1: 删除旧文件**

```bash
git rm ui/index.html ui/app.js ui/styles.css
```

- [ ] **Step 2: 运行全部 Go 测试确认未破坏**

```bash
"/c/Program Files/Go/bin/go" test -count=1 ./...
```

Expected: 5/5 包通过。

- [ ] **Step 3: 最终编译**

```bash
cd frontend && npm run build
export PATH="/c/Program Files/Go/bin:$PATH" && "C:/Users/lp/go/bin/wails.exe" build -o bin/spotter-client.exe
```

- [ ] **Step 4: 手工验收清单**

启动二进制并按 spec §13 逐项验收：

1. ✅ 窗口无系统标题栏，自定义标题栏可拖动
2. ✅ min / max / close 按钮工作
3. ✅ 双击标题栏切换最大化（需在 TitleBar 上加双击事件 — 如果未实现则跳过此条）
4. ✅ Deploy / Scan / Add 三个 inline 表单互斥展开
5. ✅ 表单回车提交
6. ✅ 选中设备显示 3 张卡
7. ✅ DeviceActions 底部常驻 + 密码字段 + Refresh + Uninstall
8. ✅ Uninstall 直接执行无二次弹窗
9. ✅ Clear registry 是唯一 confirm 弹窗
10. ✅ 搜索框过滤
11. ✅ Jetson 设备列表行有 orange tag
12. ✅ 深色主题一致
13. ✅ 收到 unknown-device 时右下 notification 提示

第 3 项未实现：双击标题栏最大化。如果验收需要此功能，补一行：TitleBar.middle 加 `onDoubleClick={WindowToggleMaximise}`。

- [ ] **Step 5: 补充双击最大化（如验收需要）**

```tsx
<div className={styles.middle} onDoubleClick={WindowToggleMaximise} />
```

如果上一步通过则跳过此步。

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore: remove legacy ui/ directory (replaced by frontend/)"
```

---

### Task 16: 文档更新

**Files:**
- Modify: `README.md`（更新 build 步骤、UI 描述）
- Modify: `docs/superpowers/specs/2026-08-21-spotter-design.md`（附录 A 更新目录树）

**Interfaces:**
- Consumes: Task 15 完成的状态
- Produces: README 与设计文档反映新目录结构

- [ ] **Step 1: 更新 README 的 build 步骤**

修改 `make client` 段落（如有），注明需要先 `cd frontend && npm install`。把"开发 UI"步骤从 `ui/` 改为 `frontend/`。

- [ ] **Step 2: 更新 spec 附录 A**

读 `docs/superpowers/specs/2026-08-21-spotter-design.md`，更新目录树：
- `cmd/client/main.go` → 已经是 `main.go`（项目根）
- `ui/` → `frontend/src/`
- `ui/index.html` → `frontend/index.html`

- [ ] **Step 3: Commit**

```bash
git add README.md docs/superpowers/specs/2026-08-21-spotter-design.md
git commit -m "docs: update README and design spec for new frontend/ layout"
```

---

## Self-Review

**1. Spec 覆盖：**

| Spec 节 | 对应任务 |
|---------|---------|
| §3.1 Frameless | Task 3 |
| §3.2 自定义标题栏 | Task 4, 5 |
| §4 整体布局 | Task 8, 9 |
| §6.1 inline 表单 | Task 10, 11, 12 |
| §6.2 DeviceActions | Task 13 |
| §6.3 表单校验 | Task 10 |
| §7 唯一 Modal | Task 8 (Clear) |
| §8 主题配色 | Task 1 |
| §9 状态管理 | Task 6, 7 |
| §10 文件结构 | Task 1 |
| §11 Go 端变更 | Task 2, 3 |
| §12 wails.json | Task 2 |
| §13 验收清单 | Task 15 |

无遗漏。

**2. 占位符扫描：** 无 TBD/TODO/FIXME。✓

**3. 类型一致性：**
- `RegistryEntry.device_id` 全篇一致（snake_case）
- `DeviceActions` 接口在 Task 11 定义，Task 13 引用 ✓
- `useDevices` 在 Task 6 定义，Task 7/8/9/12/13/14 全部引用 ✓

**4. 隐藏问题修复：**
- Task 12 中我原本计划让 EmptyState 按钮联动 ActionPanel，简化后改为引导文字说明，避免双层状态提升复杂性。
- Task 13 的 username 在 uninstall 路径暂未使用（main.go 用 registry 里的），这与当前 Go 端签名一致；UI 调用方传 username 不破坏行为。

执行选项：

**1. Subagent-Driven (推荐)** — 我为每个任务 dispatch 一个新的子代理，期间审查。任务间快速迭代。

**2. Inline Execution** — 在当前会话中按任务执行，每个检查点停顿。

请选择。
