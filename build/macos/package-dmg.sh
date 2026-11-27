#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "Usage: $0 <arm64|amd64> [version]" >&2
  exit 1
fi

ARCH="$1"
VERSION="${2:-0.1.0}"

case "$ARCH" in
  arm64|amd64) ;;
  *)
    echo "Unsupported macOS arch: $ARCH (expected arm64 or amd64)" >&2
    exit 1
    ;;
esac

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
DIST_DIR="$SCRIPT_DIR/dist-${ARCH}"
RULES_SRC_DIR="$DIST_DIR/rules"
if [ ! -d "$RULES_SRC_DIR" ] && [ -d "$SCRIPT_DIR/dist/rules" ]; then
  RULES_SRC_DIR="$SCRIPT_DIR/dist/rules"
fi

for required in \
  "$DIST_DIR/singtail" \
  "$DIST_DIR/singtail-service" \
  "$DIST_DIR/bin/sing-box" \
  "$SCRIPT_DIR/Info.plist" \
  "$SCRIPT_DIR/singtail.icns" \
  "$SCRIPT_DIR/install-service.command" \
  "$SCRIPT_DIR/uninstall-service.command"; do
  if [ ! -f "$required" ]; then
    echo "Missing required packaging file: $required" >&2
    exit 1
  fi
done

if [ ! -d "$RULES_SRC_DIR" ] || [ -z "$(find "$RULES_SRC_DIR" -maxdepth 1 -name '*.srs' -print -quit)" ]; then
  echo "Missing .srs rule files in $RULES_SRC_DIR" >&2
  exit 1
fi

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

STAGE_DIR="$WORK_DIR/dmg-root"
APP_DIR="$STAGE_DIR/SingTail.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

mkdir -p "$MACOS_DIR" "$RESOURCES_DIR/bin" "$RESOURCES_DIR/rules" "$ROOT_DIR/release-artifacts"

cp "$SCRIPT_DIR/Info.plist" "$CONTENTS_DIR/Info.plist"
cp "$SCRIPT_DIR/singtail.icns" "$RESOURCES_DIR/singtail.icns"
if [ -x /usr/libexec/PlistBuddy ]; then
  /usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $VERSION" "$CONTENTS_DIR/Info.plist"
  /usr/libexec/PlistBuddy -c "Set :CFBundleVersion $VERSION" "$CONTENTS_DIR/Info.plist"
fi

cp "$DIST_DIR/singtail" "$MACOS_DIR/singtail"
cp "$DIST_DIR/singtail-service" "$MACOS_DIR/singtail-service"
chmod +x "$MACOS_DIR/singtail" "$MACOS_DIR/singtail-service"

cp "$DIST_DIR/bin/sing-box" "$RESOURCES_DIR/bin/sing-box"
chmod +x "$RESOURCES_DIR/bin/sing-box"

cp "$RULES_SRC_DIR"/*.srs "$RESOURCES_DIR/rules/"

codesign --force --deep --sign - "$APP_DIR"

ln -s /Applications "$STAGE_DIR/Applications"
cp "$SCRIPT_DIR/install-service.command" "$STAGE_DIR/install-service.command"
cp "$SCRIPT_DIR/uninstall-service.command" "$STAGE_DIR/uninstall-service.command"
chmod +x "$STAGE_DIR/install-service.command" "$STAGE_DIR/uninstall-service.command"

OUT_DMG="$ROOT_DIR/release-artifacts/SingTail-macos-${ARCH}.dmg"
rm -f "$OUT_DMG"
hdiutil create \
  -volname "SingTail" \
  -srcfolder "$STAGE_DIR" \
  -ov \
  -format UDZO \
  "$OUT_DMG"

echo "Built $OUT_DMG"
