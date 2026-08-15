#!/bin/bash

set -e

PROJECT_ROOT="/home/alfio/Projects/Dashboard"
BUILD_DIR="$PROJECT_ROOT/build/bin"

echo "=== Building Dashboard ==="
echo "Project root: $PROJECT_ROOT"

echo ""
echo "[1/3] Building frontend..."
cd "$PROJECT_ROOT/frontend"
npm run build

echo ""
echo "[2/3] Building Go application with Wails..."
cd "$PROJECT_ROOT"
# devtools tag enables WebKit developer extras, required for the built-in
# inspector (InspectorOpen/Close); webkit2_41 is mandatory on openSUSE Tumbleweed.
wails build -s -tags "webkit2_41 devtools"

echo ""
echo "[3/3] Build complete!"
echo "Binary: $BUILD_DIR/Dashboard"
ls -la "$BUILD_DIR/Dashboard"