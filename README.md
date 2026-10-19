# PicPress

> 拖进来，就变小。一个用 **Go + Wails v3** 写的本地图片压缩桌面应用。

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

## 构建与运行

要求：macOS 12+、Go 1.26+、Xcode Command Line Tools。

```bash
# 开发运行（直接编译并启动）
make run

# 构建 .app（ad-hoc 签名）
make app
open bin/PicPress.app

# 只编译二进制
make build

# 单元测试
make test
```

> 项目没有 Node 构建步骤：前端是原生 ES Module，直接通过 `go:embed` 打进二进制；Wails 的 JS 运行时由内置资源服务 `/wails/runtime.js` 提供。

## 打包说明

- `make app` 会生成 `bin/PicPress.app`，拷贝 `build/darwin/Info.plist` 和 `icons.icns`，并使用 `codesign --sign -` 做 ad-hoc 签名。
- 面向其他 Mac 分发时，请使用自己的 Developer ID 重新签名并 notarize；否则 Gatekeeper 会提示未验证的开发者。
- 构建默认使用 `-tags production,nodynamic`：
  - `production` 使用 Wails 的生产运行时；
  - `nodynamic` 让 WebP / AVIF / HEIC 走纯 Go 的 WASM 编解码器，不依赖 Homebrew 安装的 `libwebp` 等动态库，便于打包分发。

## 支持的格式

| | 格式 |
| --- | --- |
| 输入 | JPEG、PNG、WebP、AVIF、HEIC/HEIF、GIF（仅第一帧） |
| 输出 | JPEG、PNG、WebP、AVIF |

- **PNG** 是无损格式，质量参数对 PNG 不生效，界面会给出提示。
- **JPEG** 不支持透明通道，透明区域会用白色（`#ffffff`）填充。
- 重新编码会移除 EXIF / ICC 等元数据，这也是压缩的一部分；EXIF 方向会在编码前应用到像素上。
- AVIF 编码较慢（大图可能需要几秒），适合追求极限体积时使用。

## 项目结构

```
.
├── main.go                     # Wails 应用、窗口、原生拖拽、菜单
├── appservice.go               # 绑定给前端的会话/压缩/保存服务
├── internal/imagepress/        # 解码、EXIF 方向、缩放、编码、预览
├── frontend/                   # 原生 ES Module 前端（无构建步骤）
│   ├── index.html
│   ├── style.css
│   ├── app.js
│   └── bindings/               # Wails 生成的 JS 绑定（已提交）
├── build/                      # macOS 图标与 Info.plist
├── scripts/gen-icon.go         # 生成 build/appicon.png
├── Makefile
└── Taskfile.yml
```

### 重新生成前端绑定

服务方法或模型发生变化后：

```bash
wails3 generate bindings -b -d frontend/bindings -clean
```

`-b` 让绑定直接导入 `/wails/runtime.js`，因此前端不需要 npm 依赖。

### 重新生成图标

```bash
go run scripts/gen-icon.go
wails3 generate icons -input build/appicon.png \
  -macfilename build/darwin/icons.icns \
  -windowsfilename build/windows/icon.ico
```

## 实现要点

- **Wails v3** 提供原生窗口、`⌘` 菜单、系统文件对话框与文件拖放。拖放需要窗口开启 `EnableFileDrop`，并且前端注册 `data-file-drop-target` 区域；本项目把整个工作区作为放置目标。
- **压缩流水线**：解码 → EXIF 旋转 → 缩放（Lanczos）→ 多格式编码择优 → 生成预览。预览会重新解码压缩结果，确保对比看到的是真实画质。
- **编码器**：JPEG/PNG 使用标准库；WebP、AVIF、HEIC 使用 `gen2brain` 系列纯 Go（WASM）编解码器，无 CGO 依赖。
- **会话管理**：Go 侧维护图片会话、临时输出和去重；前端只负责状态渲染与交互。所有压缩结果先写入系统临时目录，退出时清理。

## License

仅用于学习与个人使用。
