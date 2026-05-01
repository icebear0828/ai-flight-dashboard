#!/bin/bash

set -e

echo "🔨 Building AI Flight Dashboard Fat Server..."

# 1. Empty the embed directory to prevent recursive bloat during build
mkdir -p dist-bin
rm -f dist-bin/*

# 2. Build slim binaries to a temporary directory
TMP_DIR=$(mktemp -d)

echo "Compiling for macOS (ARM64)..."
GOOS=darwin GOARCH=arm64 go build -o $TMP_DIR/dashboard-darwin-arm64 ./cmd/dashboard

echo "Compiling for macOS (AMD64)..."
GOOS=darwin GOARCH=amd64 go build -o $TMP_DIR/dashboard-darwin-amd64 ./cmd/dashboard

echo "Compiling for Linux (AMD64)..."
GOOS=linux GOARCH=amd64 go build -o $TMP_DIR/dashboard-linux-amd64 ./cmd/dashboard

echo "Compiling for Windows (AMD64)..."
GOOS=windows GOARCH=amd64 go build -o $TMP_DIR/dashboard-windows-amd64.exe ./cmd/dashboard

# 3. Move the slim binaries into the embed directory
mv $TMP_DIR/* dist-bin/
rm -rf $TMP_DIR

echo "✅ Multi-platform binaries staged in dist-bin/"
echo "🚀 Now building the local Fat Server..."
go build -o dashboard ./cmd/dashboard

echo "🎉 Done! The local './dashboard' executable now contains all other platforms."
