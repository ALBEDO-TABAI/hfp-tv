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

动效只用来交代发生了什么：东西从哪来、到哪去、和什么有关。全项目一套语言，唯一来源是 `web/src/styles/globals.css` 第 7 节的 CSS 变量；脚本侧镜像在 `web/src/lib/motion-tokens.ts`，AntD 动效 token 在 `web/src/lib/app-theme.ts` 里直接引用这些变量。出现别的时长或曲线就是 bug。

### 4.1 时长与曲线

| Token | 值 | 用在 |
| --- | --- | --- |
| `--motion-press` | 80ms | 按下的那一下 |
| `--motion-instant` | 120ms | 悬停、颜色与透明度反馈；小元素离开 |
| `--motion-state` | 180ms | 控件状态、图标互换、提示与下拉出现、松开回弹、页面内容淡入；浮层离开 |
| `--motion-panel` | 240ms | 弹窗、抽屉、浮层面板、消息出现；展开收起；列表增删；生成结果淡入 |
| `--motion-stage` | 320ms | 侧栏折叠、程序化镜头、主题切换 |

另有两个只给特定用途的时长：`--motion-emphasis`（600ms，只播一次的强调，例如生成完成时描边亮起、选中节点时连线跑一次光点）和 `--motion-sweep`（1.4s，不确定进度的扫光循环）。

离开比进入快一档。曲线只有四条：`--ease-out`（进入、移动、纯淡出）、`--ease-in`（带位移的离场）、`--ease-in-out`（非用户直接触发、起止都在画面里的移动，如程序化镜头、主题揭示）、`--ease-spring`（约 2.5% 回弹的 `linear()` 弹簧，只用于松开回弹、开关滑块和强调；悬停不用）。Tailwind 的 `transition` 默认时长和曲线也指向这里，写时长用 `duration-(--motion-state)`，不写 `duration-200` 或 `transition-all`。motion 库用 `motionDuration`、`motionEase`、`motionSpring`（snappy / smooth / settle）和 `motionPreset`。

「进行中」只用三种循环：转圈、不确定进度扫光（`--motion-sweep`）和呼吸点。呼吸点全项目共用 `--motion-breathe`（约 1 秒一呼吸，只变透明度），同一状态只留一处循环。纯装饰的无限循环不做。

### 4.2 各类变化

- **按下**：按下立即以指针为中心内收约 2px（小按钮约 0.97，大卡片只收一点），指针下的那一点不动，原地松手一定还落在按钮里；松开按弹簧回位，中途再按从当前大小接着走。全局统一由 `use-workspace-button-feedback.ts` 处理，用独立的 `scale` 属性，不覆盖组件自己的 `transform`；画布世界里的节点不参与。组件不再自己写 `:active` 缩放或 `whileTap`。
- **悬停**：颜色和底色 120ms；卡片最多抬起 2px，卡片里的图片最多放大 1.035（240ms）。
- **页面**：切换栏目时内容区 180ms 淡入，不位移；同一栏目内换参数不重播。列表进入页面不逐卡错峰。
- **弹窗与浮层**：`AppModal` 从点击处方向 0.96→1 放大淡入（240ms），离开 0.98 淡出（180ms）；直接使用 AntD `Modal` 的地方改用 `AppModal`。浮层按 `motionPreset.popover` 朝锚点移近 4px。需要离场动画的组件保持挂载（`useOpenedOnce`），或在 `AnimatePresence` 里条件渲染，不在外面提前 `return null`；「选中某项才渲染」的弹窗和抽屉用 `useClosingValue`，关闭后先保留内容播完离场再卸载。命令式确认框用 `useAppModal()`，和 `AppModal` 同一套进出场。抽屉滑入 240ms，滑出快一档（180ms，`--ease-in`）。离场期间内容一律不可操作：`AppModal`、`AppDrawer` 关闭时整体设为 `inert`（点击由遮罩接住，不会穿透到背后的页面），`AnimatePresence` 里的浮层用 `PresenceInert` 包一层，React Aria 浮层淡出时不接受点击，自绘菜单在焦点离开时关闭；`useAppModal` 打开的确认框取消后，焦点即使还停在确认按钮上，回车也不再执行；提交类回调再核对一次弹窗是否仍打开。键盘关闭菜单后焦点回到入口按钮。
- **画布**：拖动、平移、缩放、拉伸直接跟手，不加过渡。新建节点轻微放大淡入（只认刚创建的节点，平移时重新挂载的不播）；生成完成时结果淡入、描边亮起一次；选中节点时它的连线沿路径跑一次光点后保持静态强调。程序化镜头（适合屏幕、定位节点、缩放菜单里的档位和输入的百分比）在对数空间插值缩放，时长按距离取 240–400ms，过程中持续更新可见范围，任何输入立即接管；放大、缩小按钮和 ⌘+/⌘− 是直接命令，每次乘或除 1.2，180ms，连续点击时目标累加。
- **展开收起**：内容高度用 `grid-template-rows: 0fr → 1fr` 加淡入，240ms `--ease-out`；箭头同步旋转。收起时内容先设为 `inert`，播完再卸载（或在侧栏这类常驻分组里保持挂载并 `inert`）。
- **图标与文字互换**：新旧叠在同一格里交叉淡入（180ms），图标附带 0.8→1 缩放；按钮按较宽的那个定宽，不因文字变长跳动。复制、启停这类结果在按钮原地交代，不另弹提示。
- **列表换内容**：切换筛选、翻页时旧内容留在原处降到 60% 不透明度，新内容 120ms 淡入；首次等待超过 200ms 才出骨架屏。
- **进度**：用 `transform: scaleX()`，不过渡 `width`。

### 4.3 性能纪律

只动 `opacity` 和 `transform`（含 `translate`、`scale`）；不过渡 `width/height/left/top/backdrop-filter`，也不过渡模糊和投影类滤镜（`blur()`、`drop-shadow()` 逐帧重算很贵）和卡片、面板这类大面积阴影，阴影要变就用伪元素的透明度；按钮这类小控件的阴影可以随颜色一起过渡。图片悬停时饱和度、对比度的微调可以随缩放一起过渡（颜色矩阵在合成层上完成）。独立的 `scale` 会把元素自身 `transform` 里的平移一起缩放，所以靠 `transform` 定位的元素（例如画布节点）只缩放它的内层。空闲页面不应有常驻动画在跑，隐藏的转圈也不留动画。例外有三处：侧栏折叠过渡宽度和 `flex-basis`，代价已实测（每帧约 0.4ms 布局）；展开收起过渡 `grid-template-rows`，只用在侧栏分组、参数区这类小块内容上；没有子元素的单个绝对定位指示块（标签的选中底板、新手引导的聚光灯）可以过渡宽高，只重排它自己。

### 4.4 减少动态效果、流畅模式与无障碍

- **系统「减少动态效果」**：所有过渡和关键帧瞬时完成（全局规则的优先级抬到 ID 级，组件里带 `!important` 的过渡也会被覆盖），平滑滚动改为直接跳到位置，转圈照常转；motion 动画由根部 `MotionConfig reducedMotion="user"` 去掉位移和缩放。组件读减弱状态用 `useReducedMotionConfig`；脚本里临时起的动画（程序化镜头、按下反馈）用 `prefersReducedMotionNow()` 实时判断，运行中切换偏好也立即生效。
- **流畅模式**（侧栏底部和画布「画布外观」里的开关，`<html data-effects="lite">`）：关掉毛玻璃、玻璃追光和装饰循环，大阴影收小，主题切换不做整页揭示，画布「自动性能」按性能优先处理，脚本动画只保留淡入淡出，也不做按下缩放。和系统「减少动态效果」互相独立。
- **启动海报幕布 (`HfpSplash`)**：只在首次安装或升级后出现一次，总长约 1.1s，点按或任意键跳过；流畅模式和减少动态效果下不出现。
- **焦点**：全局 `:focus-visible` 采用 `2px solid var(--rl-accent)` 外框与 `2px` 偏移，适配桌面键盘与大屏遥控器焦点移动。
- **减少透明度**：`@media (prefers-reduced-transparency: reduce)` 关闭模糊折射并提升表面不透明度至 `0.96`。
