# hfp - all in one 设计系统规范 (Design System Specification)

**hfp - all in one** 的视觉与交互语言参考 **「藏书阁 (Ready Library)」**，以纯色桌面、液态玻璃书架、精装布面书脊与信号橙（Print Signal）指示灯，构成面向 AI 时代的电影感放映室与无限画布创作工作台。

应用名称统一显示为 `hfp - all in one`。图标复用 HFP All-in-One Tauri 项目的正式 1024×1024 PNG：`assets/app-icon.png` 用于桌面构建，`web/public/app-icon.png` 用于导航、加载页和页签；`web/public/favicon.ico` 为同源 ICO。图标保留原有白底与橙色手写标识，不做主题染色。代码标识和仓库地址不随显示名称改写。

---

## 1. 栅格与空间纪律 (Grid & Spatial Discipline)

hfp - all in one 全局严格遵循 **12 列响应式栅格系统** 与 **4pt 基线空间尺度**（定义于 `web/src/styles/hfp-design-system.css`）：

### 1.1 栅格 Token
| Token | 默认值（桌面） | 窄屏 (`< 900px`) | 宽屏/TV (`>= 1800px`) | 说明 |
| --- | --- | --- | --- | --- |
| `--grid-cols` | `12` | `12`（折算为单列/双列流式） | `12` | 标准列数 |
| `--grid-gutter` | `20px` | `14px` | `24px` | 列间距 |
| `--grid-row-gap` | `24px` | `16px` | `28px` | 行间距 |
| `--grid-margin` | `40px` | `18px` | `48px` | 页面两侧安全边距 |
| `--grid-max-width` | `1320px` | `100%` | `1560px` | 内容容器最大宽度 |
| `--sidebar-width` | `244px` | 抽屉模式 | `244px` | 液态玻璃侧边导航宽度 |
| `--sidebar-collapsed-width` | `68px` | — | `68px` | 折叠态导轨宽度 |

### 1.2 4pt 间距与圆角尺度
- **4pt 间距尺度**：
  - `--s1: 4px` · `--s2: 8px` · `--s3: 12px` · `--s4: 16px` · `--s5: 20px` · `--s6: 24px` · `--s8: 32px` · `--s10: 40px` · `--s14: 56px`
- **圆角体系**：
  - `--r-xs: 6px`（徽章/快捷键 `kbd`）
  - `--r-sm: 9px`（紧凑按钮/缩略图）
  - `--r-md: 12px`（常规控件/小卡片）
  - `--r-lg: 16px`（工程封面/播放器视窗）
  - `--r-xl: 22px`（液态玻璃卡片/侧栏模块）
  - `--r-2xl: 28px`（主舞台面板/大型输入舱）
  - `--r-pill: 999px`（胶囊导航/主行动按钮/分段控制器）

---

## 2. 字体与排版配对 (Typography Pairing)

全套开源字体随 `web/public/ui-fonts/` 与 `web/public/fonts/` 本地托管（许可证见 `licenses/ui/`）：

| Token | 字体栈 | 用途与排版特征 |
| --- | --- | --- |
| `--font-brand` | 水木明体 (Mizuki Ming) | 侧栏名称、首页品牌标题、加载字标和启动页标题中的 `hfp -` 使用常规字重；`all in one` 使用 Instrument Serif 斜体。水木明体随项目托管，遵循 IPA Font License 1.0。 |
| `--font-ui` | `'RL Sans'` (Inter Variable), `'PingFang SC'`, `'RL CJK'` (Noto Sans SC) | 界面正文、表单、设置与操作控件；启用 `'cv11', 'ss01', 'ss03'` OpenType 特性。 |
| `--font-display` | `'RL Display'` (Inter Tight), `'PingFang SC'`, `'RL CJK'` | 常规界面标题、胶囊主按钮（紧凑字距 `-0.02em` 至 `-0.045em`）。 |
| `--font-serif` / `--font-cloth` | `'RL Serif'` (Instrument Serif & Italic), `'RL Serif SC'` (Noto Serif SC) | 分区分镜标题、项目片名与人文叙事副标题，赋予数字工作台纸质出版物与电影片头质感。 |
| `--font-mono` | `'RL Mono'` (Geist Mono), `ui-monospace` | 帧率、时间码、分辨率、序号 (`01`–`07`)、版本号、快捷键 `<kbd>` 与状态行 `.caption`；启用 `tabular-nums` 与 `'zero', 'ss01'`。 |

- **字号梯度**：
  `--fs-2xs (10px)` → `--fs-xs (11px)` → `--fs-sm (12px)` → `--fs-md (13px)` → `--fs-lg (15px)` → `--fs-xl (17px)` → `--fs-2xl (22px)` → `--fs-3xl (28px)` → `--fs-4xl (40px)`。

---

## 3. 色彩与液态玻璃材质 (Color & Liquid Glass Material)

### 3.1 双主题语义色板
支持深色放映室模式（默认，`[data-theme="dark"]` / `.dark`）与浅色画廊桌面模式（`:root`）：

| 语义角色 | 浅色模式 (Gallery Desk) | 深色模式 (Screening Room) | 用途 |
| --- | --- | --- | --- |
| `--stage` / `--stage-2` | `#ebebee` / `#e3e3e8` | `#0b0b0d` / `#111114` | 工作区纯色背景与次级表面；页面背景不使用彩色晕染 |
| `--ink` / `--ink-2` / `--ink-3` | `#0b0b0e` / `66%` / `44%` | `#f4f4f6` / `68%` / `44%` | 主文本、次级说明与元数据文字 |
| `--hair` / `--hair-2` | `rgba(11,11,14,.08)` / `.14` | `rgba(255,255,255,.08)` / `.14` | 0.5px–1px 精细分割线与玻璃边框 |
| `--fill` / `--fill-2` / `--fill-3` | `4.5%` / `8%` / `13%` 墨色 | `5%` / `8.5%` / `13%` 亮色 | 控件常态、悬停与按下填充层 |
| `--rl-accent` | `#ff5a1f` | `#ff6a2c` | **Print / Broadcast Signal Orange**：唯一高饱和强调色（REC 信号灯、主操作按钮、激活指示点、进度条） |
| `--paper` / `--paper-ink` | `#f7f4ee` / `#1d1b18` | `#e9e4da` / `#1d1b18` | 剧本纸面与票签质感对比色 |
| `--ok` / `--warn` / `--bad` | `#1fae6b` / `#e2a400` / `#e5484d` | `#1fae6b` / `#e2a400` / `#e5484d` | 状态指示灯与诊断反馈 |

### 3.2 液态玻璃光学层 (`.lg` & `web/src/lib/hfp-glass.ts`)
复刻「藏书阁」的五层玻璃合成与指针追光：
1. **背景折射与磨砂**：`backdrop-filter: blur(var(--glass-blur)) saturate(var(--glass-sat))`。
2. **环境色调融合**：`rgb(var(--glass-tint) / var(--glass-alpha))` 叠加微弱的信号橙环境光晕 `--amb`。
3. **内高光与接触阴影**：顶部 `inset 0 1px 0 var(--glass-inner-hi)` 高光、底部 `inset 0 -1px 0 var(--glass-inner-lo)` 阴影与多级景深投影 `--shadow-1..4`。
4. **动态高光边缘 (`::before`)**：1px 渐变遮罩边框，其高光角度 `--la` 随鼠标指针位置实时通过 `requestAnimationFrame` 更新。
5. **程序化斜面焦散贴图 (`::after`)**：由 `hfp-glass.ts` 按元素尺寸与圆角在 Canvas2D 上实时计算的 `overlayMap`（源自 `Glass-HQ/liquid-glass` MIT 光学公式）。

首页采用无点阵、无暗角的纯色页面背景。首页玻璃卡片关闭橙色环境染色（`--amb-alpha: 0`），使用 `--glass-alpha: 0.08` 的轻透明表面，主要通过细边缘、折射高光与阴影区分层次。橙色继续用于操作强调和项目标记；降低透明度偏好仍使用不透明表面。

---

## 4. 动效、物理反馈与无障碍 (Motion, Physics & Accessibility)

- **缓动曲线**：
  - `--ease-out: cubic-bezier(.22, 1, .36, 1)`：面板进出场与悬停抬升。
  - `--ease-spring`：支持 CSS `linear(...)` 物理弹簧曲线，用于卡片悬浮、胶囊切换与 `.press` 按压回弹（释放时触发 560ms `.jelly` 弹性形变）。
  - `--ease-liquid: cubic-bezier(.3, .7, .2, 1)`：搜索框展开与抽屉过渡。
  - `--ease-shelf: cubic-bezier(.77, 0, .175, 1)`：架位重排与视图切换。
- **时长阶梯**：
  `--d-1: 120ms` · `--d-2: 200ms` · `--d-3: 320ms` · `--d-4: 480ms` · `--d-5: 640ms`。
- **启动海报幕布 (`HfpSplash`)**：
  沿用「藏书阁」2200×1396 比例尺的海报排版，展示品牌名 `hfp - all in one`、中英双语序言、1.5px 信号橙加载线，以及 4 项实时滚动计数的工坊指标（创作工坊、模型通道、画布工程、在库素材）。
- **键盘/遥控器焦点与无障碍支持**：
  - 全局 `:focus-visible` 采用醒目的 `2px solid var(--rl-accent)` 外框与 `2px` 偏移，适配桌面键盘与大屏遥控器焦点移动。
  - 完整支持 `@media (prefers-reduced-motion: reduce)`（瞬时完成动画，禁用悬停位移与呼吸灯）与 `@media (prefers-reduced-transparency: reduce)`（关闭模糊折射并提升表面不透明度至 `0.96`）。
