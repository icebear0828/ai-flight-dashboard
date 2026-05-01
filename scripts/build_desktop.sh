#!/bin/bash

set -e

echo "🖥️  Building AI Flight Dashboard Desktop App..."

# 1. Build frontend
echo "📦 Building frontend..."
cd frontend
npm install --silent
npm run build
cd ..

# 2. Build desktop binary with correct Wails build tags
echo "🔨 Building desktop binary..."

# macOS requires the UniformTypeIdentifiers framework for Wails v2
if [[ "$(uname)" == "Darwin" ]]; then
    export CGO_LDFLAGS="-framework UniformTypeIdentifiers"
fi

go build -tags desktop,production -ldflags "-w -s" -o build/bin/ai-flight-dashboard ./cmd/dashboard

echo ""
echo "✅ Desktop binary built at: build/bin/ai-flight-dashboard"
echo ""
echo "Usage:"
echo "  ./build/bin/ai-flight-dashboard --gui    # Native desktop window"
echo "  ./build/bin/ai-flight-dashboard --web    # Web dashboard mode"
echo "  ./build/bin/ai-flight-dashboard          # TUI terminal mode"

# Platform-specific packaging hints
if [[ "$(uname)" == "Darwin" ]]; then
    echo ""
    echo "📦 To create a macOS .app bundle, use: wails build"
fi
