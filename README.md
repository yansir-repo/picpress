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
- **默认全程本地**：不上传、不联网，图片只在本机内存和临时目录中处理。只有导入「推送配置」并在确认面板里点了确认，才会把你拖进来的文件压缩后推送到配置指定的云端地址（见下方「推送上传」）。
- **细节处理**：读取 EXIF 方向自动旋转、JPEG 透明背景填充、剥离元数据、防止误放大。

## 推送上传（开发中）

给「要向别人收文件」的场景：收到一份 `.picpress-config` 配置文件，双击导入，把文件夹拖进来，PicPress 在本机压缩后按你的文件夹路径推到云端；断网重开会接着传，传过的不会重复；视频不传原片，只传抽帧。PicPress 不认识任何项目或资料清单，只是通用的「压缩并推送」工具。

需求、配置格式、凭证接口、批次清单格式和验收清单见 [`docs/push-upload.md`](docs/push-upload.md)，实现现状见其中 §10。

- 代码：`internal/push`（扫描、键名规整、SigV4、只写 S3 客户端、凭证、journal、上传引擎、批次清单），`pushservice.go`（绑定给前端的服务）。
- 本地联调：`go run ./cmd/push-devserver -dir /tmp/picpress-inbox` 启动一个拒绝一切读操作的假 S3 和凭证接口，并生成 `dev.picpress-config`，拖到 PicPress 上即可导入；收到的对象写在 `-dir/objects/` 下。加 `-ttl 30s` 可以验证凭证自动续期。
- 视频抽帧用 `ffmpeg` / `ffprobe` 子进程：先找可执行文件旁边（`.app` 里是 `Contents/Resources/`，`make app` 会内置一份，离线可用，见[打包说明](#打包说明)），再找 `PATH`；都没有时视频会如实列为「跳过」。

## 界面快捷键

| 快捷键 | 操作 |
| --- | --- |
| `⌘O` | 打开图片 |
| `⌘S` | 保存全部 |
| `⌘I` | 导入推送配置 |
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

要求：macOS 12+、Go 1.26+、Node 24+、pnpm 12+、Xcode Command Line Tools；打包 x86_64（含 universal）还需要 `nasm`（`brew install nasm`，用于编译内置 ffmpeg 的汇编）。

```bash
# 安装前端依赖
pnpm --dir frontend install

# 开发：Vite+ dev server + Wails（支持 HMR）
make dev

# 生产构建：先 vp build，再编译 Go 二进制
make build

# 打包 .app（ad-hoc 签名；本机架构）
make app
open bin/PicPress.app

# 打包 universal .app（arm64 + x86_64，发布用）
make app-universal

# 测试 / 静态检查
make test    # Go test + vp test run
make check   # vp check（格式化 + lint + 类型检查）+ go vet
```

`make dev` 会启动 `vp dev`（127.0.0.1:9245），然后设置 `FRONTEND_DEVSERVER_URL` 运行 Go 应用；Wails 会把前端请求代理到 Vite，修改 React 代码即可热更新。

> `wails3` CLI 只用于生成 bindings 和图标，不参与日常构建。
>
> Vite+ 1.3 把 `vite` alias 到 `@voidzero-dev/vite-plus-core`，`@vitejs/plugin-react` 会报 peer 版本警告；这是 Vite+ 的已知情况，不影响构建。项目在 `frontend/pnpm-workspace.yaml` 里用 `peerDependencyRules.allowAny` 抑制了这条噪音。

## CI 与自动发布

- **CI**（`.github/workflows/ci.yml`）：PR / push 到 `main` 时运行前端 `vp check`、`vp test run`、`vp build`，以及 Go `test` / `vet`；macOS job 会实际构建 universal `.app` 并上传 artifact。
- **Release**（`.github/workflows/release.yml`）：当 `main` 上的 Go / 前端 / 打包相关代码发生变化时自动执行：
  1. 把 `VERSION` 的 patch 位 +1（`1.0.0` → `1.0.1`，即 +0.01）；
  2. 构建 arm64 + x86_64 的 universal app；
  3. 提交版本变更并打 `vX.Y.Z` tag；
  4. 创建 GitHub Release，附带 `PicPress-X.Y.Z-macOS-universal.zip`。

版本源是根目录的 `VERSION`；`scripts/bump-version.py` 同时同步 `build/darwin/Info.plist`。只改 README / docs，或 release 自己产生的版本提交，不会再次触发 release。

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
# 没装 wails3 时：
go run github.com/wailsapp/wails/v3/cmd/wails3 generate bindings -ts -i -d frontend/src/bindings -clean
```

- `-ts`：生成 TypeScript。
- `-i`：生成 interface，而不是 class，方便直接传普通对象。
- 生成文件 import `@wailsio/runtime`，因此前端需要在 `package.json` 中保留同版本的 `@wailsio/runtime`。

## 打包说明

- `make app` 会生成 `bin/PicPress.app`，拷贝 `build/darwin/Info.plist` 和 `icons.icns`，并使用 `codesign --sign -` 做 ad-hoc 签名。
- 面向其他 Mac 分发时，请使用自己的 Developer ID 重新签名并 notarize；否则 Gatekeeper 会提示未验证的开发者。`Contents/Resources/` 里的 `ffmpeg` / `ffprobe` 也要单独签名（`make bundle` 里已经先签它们，再签整个 app）。
- **内置 ffmpeg**：`make bundle` 用 `scripts/build-ffmpeg.sh` 从官方源码编译 `ffmpeg` / `ffprobe`，按 `bin/picpress` 实际包含的架构（`lipo -archs`）合成后放进 `Contents/Resources/`，连同 `FFmpeg-COPYING.LGPLv2.1`。
  - 版本和源码包 SHA-256 固定在 `Makefile`（`FFMPEG_VERSION` / `FFMPEG_SHA256`）；SHA-256 取自用 FFmpeg 发布签名（`FCF986EA15E6E293A5644F10B4322F04D67658D8`）验过的源码包。最低系统版本取自 `Info.plist` 的 `LSMinimumSystemVersion`。
  - 只开抽帧需要的组件：解封装 mov / avi / matroska / mpegts（覆盖 mp4、mov、m4v、avi、mkv、webm、mts），解码 H.264、HEVC、MPEG-1/2/4、H.263、MS-MPEG4、VC-1/WMV3、VP8、VP9、ProRes、MJPEG、DV、rawvideo，输出 PNG；静态链接，只依赖系统库（`libz` 和 CoreFoundation / CoreVideo / CoreMedia），不联网、不用 VideoToolbox。AV1 不支持（需要额外的 libdav1d），这类视频会抽帧失败、记为解码失败。
  - 编译结果缓存在 `.bin/ffmpeg-<版本>/<arch>/`，改了版本或脚本才会重新编译（每个架构约 1 分钟）。体积：每个架构 `ffmpeg` + `ffprobe` 约 13 MB，universal `.app` 约 65 MB（zip 约 29 MB）。
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

### 第三方：FFmpeg

`.app` 内置的 `ffmpeg` / `ffprobe`（`Contents/Resources/`）来自 [FFmpeg](https://ffmpeg.org/) 9.0.2，许可证为 **GNU LGPL 2.1 或更新版本**，全文随 app 附在 `Contents/Resources/FFmpeg-COPYING.LGPLv2.1`。编译时没有启用 `--enable-gpl`、`--enable-version3`、`--enable-nonfree`，也没有链接任何外部库（`--disable-autodetect`），脚本会在 `config.h` 里核对这一点，不满足就停止构建。PicPress 只把它们当独立程序以子进程调用，不链接 FFmpeg 的库。

- 源码：<https://ffmpeg.org/releases/ffmpeg-9.0.2.tar.xz>（SHA-256 `8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e`，签名 `ffmpeg-9.0.2.tar.xz.asc`）；每个 GitHub Release 也附带同一份源码包。
- 构建方式：未修改源码，配置参数见 `scripts/build-ffmpeg.sh`，也可以用 `Contents/Resources/ffmpeg -version` 查看。
- 替换：LGPL 允许你换成自己编译的版本，直接替换 `Contents/Resources/` 里的 `ffmpeg` / `ffprobe` 后重新签名即可。
