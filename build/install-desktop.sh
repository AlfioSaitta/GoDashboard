#!/usr/bin/env bash
# Installs the Dashboard launcher and icons for the current user (KDE Plasma / freedesktop).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BIN_PATH="${PROJECT_DIR}/build/bin/Dashboard"

APP_ID="dashboard"
APP_NAME="Dashboard"

if [[ ! -x "${BIN_PATH}" ]]; then
  echo "ERROR: binary not found at ${BIN_PATH}" >&2
  echo "Run 'wails build -s -tags webkit2_41' first." >&2
  exit 1
fi

APP_DIR="${HOME}/.local/share/applications"
ICON_BASE="${HOME}/.local/share/icons/hicolor"

mkdir -p \
  "${APP_DIR}" \
  "${ICON_BASE}/512x512/apps" \
  "${ICON_BASE}/256x256/apps" \
  "${ICON_BASE}/128x128/apps" \
  "${ICON_BASE}/64x64/apps"

install -m 0644 "${SCRIPT_DIR}/icon.png"     "${ICON_BASE}/512x512/apps/${APP_ID}.png"
install -m 0644 "${SCRIPT_DIR}/icon-256.png" "${ICON_BASE}/256x256/apps/${APP_ID}.png"
install -m 0644 "${SCRIPT_DIR}/icon-128.png" "${ICON_BASE}/128x128/apps/${APP_ID}.png"
install -m 0644 "${SCRIPT_DIR}/icon-64.png"  "${ICON_BASE}/64x64/apps/${APP_ID}.png"

DESKTOP_FILE="${APP_DIR}/${APP_ID}.desktop"

cat > "${DESKTOP_FILE}" <<EOF
[Desktop Entry]
Type=Application
Version=1.0
Name=${APP_NAME}
GenericName=Service Dashboard
Comment=Multi-service dashboard for NeuroNet, Minecraft and SlotBuilder
Exec=${BIN_PATH}
Icon=${APP_ID}
Terminal=false
Categories=Utility;System;Monitor;
Keywords=dashboard;neuronet;minecraft;slotbuilder;
StartupNotify=true
StartupWMClass=${APP_NAME}
SingleMainWindow=true
EOF

chmod 0644 "${DESKTOP_FILE}"

# Refresh desktop/icon databases (ignore failures on systems without them).
update-desktop-database "${APP_DIR}" 2>/dev/null || true
gtk-update-icon-cache -f -t "${ICON_BASE}" 2>/dev/null || true

echo "Launcher installed:  ${DESKTOP_FILE}"
echo "Icons installed to:  ${ICON_BASE}/<size>/apps/${APP_ID}.png"
