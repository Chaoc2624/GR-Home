# GR Mod shell — macOS 移植说明

本分支在原版 Windows shell 之上增加了 macOS 支持。核心改动都在 `shell/` 里：

| 文件 | 说明 |
| --- | --- |
| `internal/platform/darwin.go` | macOS 版 `platform.Host` 实现：`/Volumes` 卷枚举（`diskutil info -plist` 判断可移动设备与总线类型）、`osascript` 文件夹选择框、Finder 显示（`open -R`）、`diskutil eject` 安全弹出、系统代理（`scutil --proxy`） |
| `ui_darwin.go` | macOS 启动行为：打印 `LISTEN` 地址后自动用默认浏览器打开 UI（沿用 Windows `--browser` 模式的心跳机制，关掉页面服务即退出） |
| `internal/platform/dev.go`、`ui_other.go` | build tag 改为 `!windows && !darwin`，避免与 darwin 文件冲突 |
| `build.sh` | 新增 `darwin` / `darwin-arm64` / `darwin-amd64` 构建目标 |
| `../ui/src/store.ts` | 弹射按钮的平台判断从 `kind === 'windows'` 改为同时允许 `'macos'` |

`core/`（固件 patch 引擎，TypeScript）与 `ui/`（React）在 Mac 上零改动——UI 只通过本地 HTTP API 与 shell 通信。

## 在 Mac 上构建

需要：Go 1.24+、Node.js（只有改了 UI 才需要；下面第 1 步可选）。

```sh
cd shell

# 1.（可选）如果你改了 ui/，先构建前端；不改可跳过，
#    直接用下面 zip 包里自带的 shell/web（已构建好的 UI）
cd ../ui && npm install && npm run build && cd ../shell
#    —— 或者：npm install && npm -w ui run build（与原版 README 一致）

# 2. 构建 macOS 版（本机架构；Apple Silicon/Intel 会自动对应）
./build.sh darwin
#    产物：dist/grmod-macos
#    也可显式指定：./build.sh darwin-arm64 / ./build.sh darwin-amd64

# 3. 运行：双击 dist/grmod-macos（会打开终端），或在终端里执行
./dist/grmod-macos
#    首次运行 macOS 会弹 Gatekeeper 提示（未签名）：
#    系统设置 → 隐私与安全性 → 仍要打开
```

运行后默认浏览器会自动打开 UI；关闭页面约 15 秒后服务自动退出（也可用 Ctrl-C）。

## 功能对照（macOS vs Windows）

| 功能 | macOS 实现 | 说明 |
| --- | --- | --- |
| SD 卡/卷列表 | `/Volumes` + `diskutil` | 可移动设备优先列出；`?all=1` 显示全部已挂载卷（系统盘永不列出） |
| 文件夹选择 | `osascript` 原生对话框 | 取消返回空 |
| 在 Finder 中显示 | `open -R` | |
| 安全弹出 | `diskutil eject` | 有文件被占用时失败，卡保持挂载（与 Windows 行为一致：不强制） |
| 固件下载代理 | 环境变量 → `scutil --proxy` 系统代理 | 与 Windows 版（读注册表）对等 |
| 原生窗口 | 无（用默认浏览器） | Windows 用 WebView2；macOS 下纯 Go 写不了 WKWebView，如需原生窗口可再包一层 Swift |

## 已知限制

- 未做代码签名/公证：分发给他人时对方会遇到 Gatekeeper 拦截，需手动放行；正式分发建议做 `codesign` + `notarytool` 公证。
- `diskutil` 按卷逐个查询，卷很多时会稍慢（每次查询有 10 秒超时保护）。
- 交叉编译验证通过（`GOOS=darwin GOARCH=arm64/amd64`），真机只待你在 Mac 上跑一遍冒烟测试：插 SD 卡 → 卷列表出现 → 写文件 → 弹射。

## 协议

原项目为 GPL-2.0-only，本移植同样遵循 GPL-2.0-only：分发二进制必须附带完整源码。
