#!/usr/bin/env bash
#
# 生成 GitHub Pages 下载站首页（site/index.html）。
#
# 用法：
#   GITHUB_REF_NAME=main GITHUB_SHA_SHORT=<commit> .github/scripts/gen-pages.sh [site目录]
#
# 构建产物由 actions/download-artifact 事先放到 site/downloads/，
# 本脚本只负责读取四个二进制的文件大小并生成中文下载页。
set -euo pipefail

SITE_DIR="${1:-site}"
DL_DIR="$SITE_DIR/downloads"
mkdir -p "$DL_DIR"

REF_NAME="${GITHUB_REF_NAME:-本地构建}"
_raw_sha="${GITHUB_SHA_SHORT:-$(git rev-parse --short=8 HEAD 2>/dev/null || echo unknown)}"
SHA_SHORT="${_raw_sha:0:8}"
BUILD_TIME="${BUILD_TIME:-$(TZ=Asia/Singapore date '+%Y-%m-%d %H:%M')}"

card() { # $1=文件名 $2=平台名 $3=备注
  local file="$1" label="$2" note="$3"
  local path="$DL_DIR/$file" size="构建中"
  [[ -f "$path" ]] && size="$(du -h "$path" | cut -f1)"
  cat <<HTML
      <a class="card" href="downloads/${file}" download>
        <div class="plat">${label}</div>
        <div class="file">${file} · ${size}</div>
        <div class="note">${note}</div>
        <div class="btn">下载</div>
      </a>
HTML
}

{
cat <<HTML
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>GR-Home · 构建下载</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; background: #111; color: #eee;
         font-family: -apple-system, "PingFang SC", "Microsoft YaHei", sans-serif; }
  main { max-width: 720px; margin: 0 auto; padding: 48px 20px 64px; }
  h1 { font-size: 28px; margin: 0 0 4px; }
  .sub { color: #999; margin: 0 0 16px; }
  .meta { color: #777; font-size: 13px; margin: 0 0 28px; }
  .meta code { color: #aaa; }
  .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  @media (max-width: 560px) { .grid { grid-template-columns: 1fr; } }
  .card { display: block; background: #1c1c1c; border: 1px solid #2c2c2c;
          border-radius: 12px; padding: 18px; text-decoration: none; color: inherit; }
  .card:hover { border-color: #555; }
  .plat { font-size: 16px; font-weight: 600; margin-bottom: 6px; }
  .file { font-size: 13px; color: #999; margin-bottom: 6px; word-break: break-all; }
  .note { font-size: 12px; color: #777; margin-bottom: 14px; min-height: 32px; }
  .btn { display: inline-block; background: #2f81f7; color: #fff; font-size: 14px;
         padding: 8px 22px; border-radius: 8px; }
  .foot { margin-top: 32px; color: #666; font-size: 13px; line-height: 1.8; }
</style>
</head>
<body>
<main>
  <h1>GR-Home</h1>
  <p class="sub">Ricoh GR IV 改机工具 · macOS 移植版构建下载</p>
  <p class="meta">构建分支/标签：${REF_NAME} · 提交 <code>${SHA_SHORT}</code> · ${BUILD_TIME}（新加坡时间）</p>
  <div class="grid">
$(card "grmod-macos-arm64" "macOS · Apple Silicon" "未签名：首次运行被拦截时，去「系统设置 → 隐私与安全性」点允许")
$(card "grmod-macos-amd64" "macOS · Intel" "同上")
$(card "GRMod.exe" "Windows · 64 位" "SmartScreen 拦截时点「更多信息」→「仍要运行」")
$(card "grmod-dev" "Linux · 64 位" "下载后 chmod +x 再运行")
  </div>
  <p class="foot">每次 push 到 main（或打 v* 标签）都会自动重新构建，本页同步更新。<br>构建记录见仓库的 Actions 页面。</p>
</main>
</body>
</html>
HTML
} > "$SITE_DIR/index.html"

echo "wrote $SITE_DIR/index.html"
