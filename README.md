# PicPress

> 拖进来，就变小。一个用 **Go + Wails v3 + React** 写的本地图片压缩桌面应用。

![PicPress 截图](docs/screenshot.png)

## 功能

- **拖拽即压缩**：把图片拖进窗口，或按 `⌘O` 选择，添加后立即自动压缩。
- **多种输出格式**：智能 / JPEG / PNG / WebP / AVIF。
  - 智能模式会同时尝试 WebP 与 JPEG（带透明通道时尝试 WebP 与 PNG），保留体积最小的那个。
- **参数可调**：质量（10–100）、原尺寸 / 限制长边 / 按比例缩放，改动后自动重新压缩。
- **批量处理**：一次添加多张，卡片展示原图尺寸、压缩后尺寸、体积变化和节省比例。
- **压缩前后对比**：内置滑块对比，直观看到画质差异。
- **保存灵活**：单张另存为，或一键保存全部到指定文件夹。
- **全程本地**：不上传、不联网，图片只在本机内存和临时目录中处理。
- **细节处理**：读取 EXIF 方向自动旋转、JPEG 透明背景填充、剥离元数据、防止误放大。

## 界面快捷键

| 快捷键 | 操作 |
| --- | --- |
| `⌘O` | 打开图片 |
| `⌘S` | 保存全部 |
| `⌘K` | 清空列表 |
| `Esc` | 关闭对比弹窗 |

## 技术栈

| 层 | 技术 |
| --- | --- |
| 桌面壳 / 图像处理 | Go 1.26、Wails v3 beta.32、纯 Go 编解码器（WebP / AVIF / HEIC） |
| UI | React 19、TypeScript 7 |
| 构建 | pnpm 12、Vite+ 1.3（`vp dev / check / test / build`） |
| 状态 | Effect 4.0.5 + `@effect/atom-react`（Atom） |
| 调用层 | Wails 生成的 TypeScript bindings，用 Effect 包装为类型化异步操作 |

## 构建与运行

要求：macOS 12+、Go 1.26+、Node 24+、pnpm 12+、Xcode Command Line Tools。

```bash
# 安装前端依赖
pnpm --dir frontend install

# 开发：Vite+ dev server + Wails（支持 HMR）
make dev

# 生产构建：先 vp build，再编译 Go 二进制
make build

# 打包 .app（ad-hoc 签名）
make app
open bin/PicPress.app

# 测试 / 静态检查
make test    # Go test + vp test run
make check   # vp check（格式化 + lint + 类型检查）+ go vet
```

`make dev` 会启动 `vp dev`（127.0.0.1:9245），然后设置 `FRONTEND_DEVSERVER_URL` 运行 Go 应用；Wails 会把前端请求代理到 Vite，修改 React 代码即可热更新。

> `wails3` CLI 只用于生成 bindings 和图标，不参与日常构建。
>
> Vite+ 1.3 把 `vite` alias 到 `@voidzero-dev/vite-plus-core`，`@vitejs/plugin-react` 会报 peer 版本警告；这是 Vite+ 的已知情况，不影响构建。项目在 `frontend/pnpm-workspace.yaml` 里用 `peerDependencyRules.allowAny` 抑制了这条噪音。

## 前端结构

```
frontend/
├── src/
│   ├── App.tsx                 # 应用壳、事件桥
│   ├── components/             # Dropzone / ItemCard / Settings / Summary / Compare / Toasts
│   ├── domain/                 # 类型、格式化、会话合并、设置 Schema
│   ├── hooks/                  # useSession、useRuntimeEvents、useToast
│   ├── services/wails.ts       # Effect 包装的 Wails 调用
│   ├── state/atoms.ts          # Atom 状态与派生 summary
│   ├── styles/style.css
│   └── bindings/               # Wails 生成的 TS 绑定（已提交）
├── vite.config.ts
├── tsconfig.json
└── package.json
```

设计上把“状态”和“副作用”分开：

- **Atom** 管理会话、设置、压缩状态、进度、对比弹窗和 toast；`summaryAtom` 是派生值。
- **Effect** 只出现在 Wails 调用边界（`services/wails.ts`），负责类型化错误；React 组件保持普通函数组件。
- **Go 仍是唯一真相**：压缩、文件写入、会话和临时文件都在 Go 侧；前端只做编排与渲染。

## 重新生成前端绑定

服务方法或模型变化后：

```bash
wails3 generate bindings -ts -i -d frontend/src/bindings -clean
```

- `-ts`：生成 TypeScript。
- `-i`：生成 interface，而不是 class，方便直接传普通对象。
- 生成文件 import `@wailsio/runtime`，因此前端需要在 `package.json` 中保留同版本的 `@wailsio/runtime`。

## 打包说明

- `make app` 会生成 `bin/PicPress.app`，拷贝 `build/darwin/Info.plist` 和 `icons.icns`，并使用 `codesign --sign -` 做 ad-hoc 签名。
- 面向其他 Mac 分发时，请使用自己的 Developer ID 重新签名并 notarize；否则 Gatekeeper 会提示未验证的开发者。
- 构建默认使用 `-tags production,nodynamic`：
  - `production` 使用 Wails 的生产运行时；
  - `nodynamic` 让 WebP / AVIF / HEIC 走纯 Go 的 WASM 编解码器，不依赖 Homebrew 安装的 `libwebp` 等动态库，便于打包分发。
- `frontend/dist/` 是构建产物，已被 `.gitignore` 忽略；只保留一个 `.gitkeep` 供全新 clone 时 `go:embed` 使用。

## 支持的格式

| | 格式 |
| --- | --- |
| 输入 | JPEG、PNG、WebP、AVIF、HEIC/HEIF、GIF（仅第一帧） |
| 输出 | JPEG、PNG、WebP、AVIF |

- **PNG** 是无损格式，质量参数对 PNG 不生效，界面会给出提示。
- **JPEG** 不支持透明通道，透明区域会用白色（`#ffffff`）填充。
- 重新编码会移除 EXIF / ICC 等元数据，这也是压缩的一部分；EXIF 方向会在编码前应用到像素上。
- AVIF 编码较慢（大图可能需要几秒），适合追求极限体积时使用。

## 实现要点

- **Wails v3** 提供原生窗口、`⌘` 菜单、系统文件对话框与文件拖放。拖放需要窗口开启 `EnableFileDrop`，并且前端注册 `data-file-drop-target` 区域；本项目把整个工作区作为放置目标。
- **压缩流水线**：解码 → EXIF 旋转 → 缩放（Lanczos）→ 多格式编码择优 → 生成预览。预览会重新解码压缩结果，确保对比看到的是真实画质。
- **自动重压**：设置变化后 320ms 防抖重压；压缩进行中再次变化会标记 dirty，当前批次结束后自动补跑，避免旧结果覆盖新设置。
- **编码器**：JPEG/PNG 使用标准库；WebP、AVIF、HEIC 使用 `gen2brain` 系列纯 Go（WASM）编解码器，无 CGO 依赖。
- **会话管理**：Go 侧维护图片会话、临时输出和去重；前端只负责状态渲染与交互。所有压缩结果先写入系统临时目录，退出时清理。

## 重新生成图标

```bash
go run scripts/gen-icon.go
wails3 generate icons -input build/appicon.png \
  -macfilename build/darwin/icons.icns \
  -windowsfilename build/windows/icon.ico
```

## License

仅用于学习与个人使用。
