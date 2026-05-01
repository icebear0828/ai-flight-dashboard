#!/bin/bash
set -e

# AI Flight Dashboard — macOS Installer
# Usage: curl -sL https://github.com/icebear0828/ai-flight-dashboard/releases/latest/download/install-mac.sh | bash

REPO="icebear0828/ai-flight-dashboard"
APP_NAME="AI Flight Dashboard"
INSTALL_DIR="/Applications"

echo "🛩️  Installing ${APP_NAME}..."

# Detect architecture
ARCH=$(uname -m)
if [ "$ARCH" = "arm64" ]; then
    ASSET="ai-flight-dashboard-macos-m-series.tar.gz"
    echo "   Detected: Apple Silicon (M-series)"
elif [ "$ARCH" = "x86_64" ]; then
    ASSET="ai-flight-dashboard-macos-intel.tar.gz"
    echo "   Detected: Intel Mac"
else
    echo "❌ Unsupported architecture: $ARCH"
    exit 1
fi

# Get latest release download URL
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

# Download
TMPDIR=$(mktemp -d)
echo "📥 Downloading ${ASSET}..."
curl -sL -o "${TMPDIR}/${ASSET}" "${DOWNLOAD_URL}"

# Extract
echo "📦 Extracting..."
tar -xzf "${TMPDIR}/${ASSET}" -C "${TMPDIR}"

# Remove quarantine attribute (bypass Gatekeeper for unsigned apps)
xattr -cr "${TMPDIR}/${APP_NAME}.app"

# Install
if [ -d "${INSTALL_DIR}/${APP_NAME}.app" ]; then
    echo "🔄 Removing previous version..."
    rm -rf "${INSTALL_DIR}/${APP_NAME}.app"
fi

echo "📂 Installing to ${INSTALL_DIR}..."
mv "${TMPDIR}/${APP_NAME}.app" "${INSTALL_DIR}/"

# Cleanup
rm -rf "${TMPDIR}"

echo ""
echo "✅ ${APP_NAME} installed successfully!"
echo "   Open from Applications or run:"
echo "   open '/Applications/${APP_NAME}.app'"
echo ""
