#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ -x "$SCRIPT_DIR/SingTail.app/Contents/MacOS/singtail-service" ]; then
  SERVICE_BIN="$SCRIPT_DIR/SingTail.app/Contents/MacOS/singtail-service"
elif [ -x "/Applications/SingTail.app/Contents/MacOS/singtail-service" ]; then
  SERVICE_BIN="/Applications/SingTail.app/Contents/MacOS/singtail-service"
elif [ -x "$HOME/Applications/SingTail.app/Contents/MacOS/singtail-service" ]; then
  SERVICE_BIN="$HOME/Applications/SingTail.app/Contents/MacOS/singtail-service"
else
  echo "[SingTail] SingTail.app/Contents/MacOS/singtail-service not found."
  exit 1
fi

echo "[SingTail] Uninstalling background service..."
"$SERVICE_BIN" -uninstall
echo "[SingTail] Background service uninstalled."
