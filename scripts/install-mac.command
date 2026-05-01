#!/bin/bash
set -e
DIR="$(cd "$(dirname "$0")" && pwd)"
APP="${DIR}/AI Flight Dashboard.app"
echo "🛩️  正在安装 AI Flight Dashboard..."
xattr -cr "$APP"
if [ -d "/Applications/AI Flight Dashboard.app" ]; then
    rm -rf "/Applications/AI Flight Dashboard.app"
fi
mv "$APP" /Applications/
echo "✅ 安装完成！正在启动..."
open "/Applications/AI Flight Dashboard.app"
