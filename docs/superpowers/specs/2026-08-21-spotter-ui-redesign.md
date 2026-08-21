# Spotter 客户端 UI 重新设计

| 项 | 值 |
|---|---|
| 项目名 | **Spotter** |
| 文档日期 | 2026-08-21 |
| 范围 | 仅 UI 层（前端 + Wails 窗口选项） |
| 后端 | 不变（Go bindings 复用） |

---

## 1. 目标

现有 UI 是单 HTML 文件 + 原生 DOM + 系统标题栏，可用但**不够美观、扩展性差**。本次重设计：

- 去掉 Wails 系统标题栏，改为自定义可拖动标题栏（窗口控制按钮内置）
- 引入 React + Vite + TypeScript + Ant Design 5 重构前端
- 减少 modal 弹出，**所有需要输入的内容常驻在主界面（inline 表单 / 操作条）**
- 整体风格现代化、护眼（深色主题）

## 2. 技术栈

| 层 | 选择 | 理由 |
|----|------|------|
| 框架 | React 18 + TypeScript | Wails 官方推荐，生态成熟 |
| 构建 | Vite 5 | 启动快，HMR 好 |
| UI 组件库 | Ant Design 5 (`antd`) | 企业级，组件齐全，中文友好 |
| 图标 | `@ant-design/icons` | 配套 |
| 状态管理 | React Context + useReducer | MVP 规模无需 Redux；事件流直接进 reducer |
| 窗口控制 | Wails runtime `WindowMinimise` / `WindowToggleMaximise` / `WindowClose` | 不引入额外库 |

Wails v2 内置 Vite（`wails.json` 配 `frontend:install` / `frontend:build`），构建产物在 `frontend/dist`，Go 端 `//go:embed all:frontend/dist` 嵌入。

## 3. 窗口与标题栏

### 3.1 Wails 选项修改（`main.go`）

```go
Windows: &windows.Options{
    WebviewIsTransparent: false,
    DisableWindowIcon:    true,
    // Frameless + 自绘标题栏：
    Frameless: true,
},
```

启动后窗口无任何原生 chrome。

### 3.2 自定义标题栏（`<TitleBar />`）

- 高度 40px，固定在窗口顶部
- 三段式：
  - **左**（180px）：Logo（24×24）+ 产品名"Spotter"
  - **中**（flex:1）：空白区域 + `style={{ WebkitAppRegion: 'drag' }}`，用于拖动窗口
  - **右**（140px）：三个图标按钮（min / max / close），`WebkitAppRegion: 'no-drag'`
- 双击中段 = 最大化 / 还原
- 关闭按钮 hover 显示红色背景

### 3.3 窗口控制 API 调用

```ts
import { WindowMinimise, WindowToggleMaximise, WindowClose } from '../../wailsjs/runtime/runtime';
```

按钮 onClick 调用对应函数。最大化状态需要本地 useState 跟踪（通过 `runtime.WindowIsMaximised` 轮询或事件）。

## 4. 整体布局

```
┌─ TitleBar (40px) ─────────────────────────────────────────┐
├──────────────┬─────────────────────────────────────────────┤
│ Sidebar      │ MainArea                                    │
│ (260px)      │ ┌─────────────────────────────────────────┐ │
│              │ │ Toolbar (48px)                           │ │
│ [actions]    │ ├─────────────────────────────────────────┤ │
│ ┌──────────┐ │ │                                         │ │
│ │ device   │ │ │  DetailPanel                            │ │
│ │ list     │ │ │  - 空状态：Empty 组件 + 引导按钮         │ │
│ │ with     │ │ │  - 有选中：3 张 Cards + 底部 Actions    │ │
│ │ search   │ │ │                                         │ │
│ └──────────┘ │ ├─────────────────────────────────────────┤ │
│              │ │ StatusBar (24px)                         │ │
│              │ └─────────────────────────────────────────┘ │
└──────────────┴─────────────────────────────────────────────┘
```

- **Sidebar**：固定 260px，不可拖动改变宽度（MVP 简化）
- **MainArea**：flex:1，自适应剩余空间
- **Toolbar**：48px，主区顶部工具条（用于状态显示 + Refresh）
- **DetailPanel**：flex:1，可滚动
- **StatusBar**：24px，底部固定

## 5. 组件树

```
App
├── ConfigProvider (Antd dark theme)
└── Layout
    ├── TitleBar (40px)
    ├── Sidebar (260px)
    │   ├── BrandHeader
    │   ├── ActionPanel (deploy/scan/add inline forms)
    │   ├── SearchBox
    │   ├── DeviceList
    │   │   └── DeviceRow × N
    │   └── FooterActions (Clear registry)
    └── MainArea
        ├── Toolbar (refresh button + last updated)
        ├── DetailPanel
        │   ├── EmptyState (no selection)
        │   ├── BasicCard
        │   ├── NetworkCard (含接口 Table)
        │   ├── JetsonCard
        │   └── DeviceActions (password + uninstall/refresh)
        └── StatusBar (online count / total)
```

## 6. inline 表单设计（关键变更）

### 6.1 Sidebar 顶部 ActionPanel

**三个按钮**：`+ Deploy`、`Scan`、`Add by IP`

**点击行为**：
- 点击展开对应 inline 表单（高度增加 200-280px）
- **互斥**：展开 Deploy 自动收起 Scan / Add
- **状态保留**：表单字段值存在 Context/State 中，收起不丢失
- **Enter 键**：每个表单的输入框回车触发主按钮

**Deploy 表单**：
```
+ Deploy                              [× close]
─────────────────────────────────────
IP         [_________________]
SSH port   [22]
Username   [fitow]
Password   [_________________]
─────────────────────────────────────
                [Cancel]   [Deploy ▸]
```

**Scan 表单**：
```
+ Scan subnet                         [× close]
─────────────────────────────────────
CIDR       [_________________]
            (e.g. 192.168.1.0/24)
─────────────────────────────────────
                [Cancel]   [Scan ▸]
```

**Add by IP 表单**：
```
+ Add by IP                           [× close]
─────────────────────────────────────
IP         [_________________]
HTTP port  [9999]
Username   [fitow]
─────────────────────────────────────
                [Cancel]   [Add ▸]
```

### 6.2 DetailPanel 底部 DeviceActions

选中设备时，详情面板底部常驻：

```
─────────────────────────────────────────────
Device actions
─────────────────────────────────────────────
Password  [_________________]
[Refresh]              [Uninstall spotterd]
```

- Password 字段：type=password，不持久化
- Refresh：调 `App.RefreshNow()`
- Uninstall：点击直接执行（无二次确认，password 非空时立刻跑 SSH）
- 字段值随选择设备切换而清空

### 6.3 表单校验

- 使用 AntD `Form` 的 `rules`，错误信息**直接显示在字段下方**（不弹窗）
- IP 格式：正则校验 IPv4
- 端口：1-65535 整数
- 密码：非空
- 提交按钮 loading 状态，期间禁用其它表单

## 7. 唯一保留的 Modal

只保留两类弹窗：

1. **清空注册表确认**（`Modal.confirm`）：带设备数 + "此操作不影响远端设备" 说明
2. **全局错误通知**（`notification.error` / `notification.success`）：右下角角标，不阻塞

其它所有交互**全部 inline**，不弹窗。

## 8. 主题与配色

```ts
<ConfigProvider
  theme={{
    algorithm: theme.darkAlgorithm,
    token: {
      colorPrimary: '#1677ff',
      borderRadius: 6,
    },
  }}
>
```

| 用途 | 颜色 |
|------|------|
| 背景 | `#141414` |
| 卡片 | `#1f1f1f` |
| 文字主 | `#fff` |
| 文字次 | `#aaa` |
| 强调色 / 在线 | `#52c41a` |
| Jetson tag | `#fa8c16` |
| 离线 | `#ff4d4f` |
| 标题栏背景 | `#0a0a0a`（与主区区分） |

## 9. 状态管理

### 9.1 DeviceContext

```ts
interface DeviceState {
  devices: RegistryEntry[];        // from ListDevices()
  selectedId: string | null;
  searchQuery: string;
  loading: boolean;
}

type Action =
  | { type: 'SET_DEVICES'; devices: RegistryEntry[] }
  | { type: 'SELECT'; id: string | null }
  | { type: 'SEARCH'; query: string }
  | { type: 'SET_LOADING'; loading: boolean };
```

- `useReducer` 维护状态
- Context Provider 包 App 顶层
- 各组件用 `useDevices()` hook 访问

### 9.2 事件订阅

```ts
function useWailsEvents() {
  const dispatch = useDevicesDispatch();
  useEffect(() => {
    const off1 = EventsOn('info-updated', () => refresh(dispatch));
    const off2 = EventsOn('offline', () => refresh(dispatch));
    const off3 = EventsOn('unknown-device', (e) => {
      notification.info({ message: 'New device discovered', description: e.info?.basic?.hostname });
      // silent accept
      AcceptUnknownDevice(e.info.device_id, e.ip, e.port, '')
        .then(() => refresh(dispatch));
    });
    return () => { off1(); off2(); off3(); };
  }, []);
}
```

事件清理通过 `EventsOn` 返回的 unsubscribe 函数（useEffect cleanup）。

## 10. 文件结构

```
device_discovery/
├── cmd/agent/
├── internal/                  # Go 代码不变
├── scripts/
├── frontend/                  # Vite 项目根（新增）
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── styles.css
│       ├── components/
│       │   ├── TitleBar.tsx
│       │   ├── Sidebar.tsx
│       │   ├── ActionPanel.tsx
│       │   ├── DeviceList.tsx
│       │   ├── DeviceRow.tsx
│       │   ├── MainArea.tsx
│       │   ├── Toolbar.tsx
│       │   ├── DetailPanel.tsx
│       │   ├── BasicCard.tsx
│       │   ├── NetworkCard.tsx
│       │   ├── JetsonCard.tsx
│       │   ├── DeviceActions.tsx
│       │   ├── EmptyState.tsx
│       │   └── StatusBar.tsx
│       ├── state/
│       │   └── DeviceContext.tsx
│       ├── hooks/
│       │   ├── useWailsEvents.ts
│       │   └── useDeviceActions.ts
│       └── utils/
│           └── format.ts
├── ui/                        # 删除（迁移完成后）
├── docs/
├── main.go                    # Frameless 选项 + embed 路径变更
├── wails.json
├── go.mod / go.sum
└── README.md
```

## 11. 关键 Go 端变更（仅 `main.go`）

```go
//go:embed all:frontend/dist
var frontendFS embed.FS

//go:embed all:frontend/dist/assets
var assetsFS embed.FS   // for assetserver

// 在 options.App 中：
AssetServer: &assetserver.Options{
    Assets:  assetsFS,
    Handler: nil,
},
Windows: &windows.Options{
    WebviewIsTransparent: false,
    DisableWindowIcon:    true,
},
Frameless: true,
```

注：Wails 默认会自动从 `frontend/dist` 提供静态资源，Go 端 embed 路径必须与 Vite 输出一致。

## 12. Wails 配置文件（`wails.json`）

```json
{
  "name": "Spotter",
  "outputfilename": "spotter-client",
  "frontend:install": "npm install",
  "frontend:build": "npm run build",
  "frontend:dev:watcher": "npm run dev",
  "frontend:dev:serverUrl": "auto",
  "author": { "name": "Spotter Dev" },
  "info": {
    "productName": "Spotter",
    "productVersion": "0.2.0",
    "comments": "Device discovery tool"
  }
}
```

## 13. 验收清单（手工）

1. 启动后窗口**无系统标题栏**，自定义标题栏可拖动
2. min / max / close 三个按钮均工作
3. 双击标题栏切换最大化
4. Deploy / Scan / Add 三个 inline 表单互斥展开，字段保留
5. 每个表单回车提交
6. 选中设备后详情面板显示 Basic / Network / Jetson 三张卡
7. 底部 Actions 区域常驻，密码字段 + Refresh + Uninstall
8. 点 Uninstall 无二次弹窗，直接执行
9. Clear registry 唯一保留的 confirm 弹窗工作
10. 设备列表带搜索框，online 状态点正确显示
11. Jetson 设备列表行有 orange tag
12. 深色主题一致，无亮色透出
13. 网络 mcast 收到新设备时，右下角 notification 角标提示

## 14. 风险与回退

| 风险 | 缓解 |
|------|------|
| AntD 打包体积大 (~500KB) | 接受；Wails 应用本就内嵌资源，体积影响有限 |
| Vite + Wails 集成踩坑 | 跟随 Wails 官方模板逐步迁移，不跳步 |
| `embed` 路径变更导致旧 UI 残留 | 旧 `ui/` 在迁移完成后立即删除 |
| 自定义标题栏拖动在双屏异常 | 用 Wails 官方推荐 `-webkit-app-region: drag` CSS |

## 15. 实施步骤（粗略）

1. 备份当前 UI 行为（截图 + 现有 binding 列表）
2. 创建 `frontend/` 目录，初始化 package.json + vite + tsconfig
3. 安装 antd / @ant-design/icons
4. 搭建 TitleBar + Sidebar + MainArea 骨架（先无功能）
5. 实施 ActionPanel（deploy/scan/add inline 表单）
6. 实施 DeviceList + DeviceRow + 搜索
7. 实施 DetailPanel 三张卡片 + DeviceActions
8. 接线 Wails bindings + events
9. 美化（间距、动效、空状态、tag、status dot）
10. 修改 `main.go` 的 Frameless + embed 路径
11. 编译验证 `wails build`
12. 删除旧 `ui/`
13. 完整手工验收清单

## 16. 不在范围内

- 不做用户认证
- 不做远程命令执行
- 不做暗色 / 亮色主题切换（仅深色）
- 不做多窗口
- 不做国际化（中文文案）
- 不做可拖动调整侧栏宽度
