#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [ -d "$HOME/sdk/go/bin" ]; then
  export PATH="$HOME/sdk/go/bin:$PATH"
fi

if [ -s "$HOME/.nvm/nvm.sh" ]; then
  # shellcheck source=/dev/null
  source "$HOME/.nvm/nvm.sh"
fi

echo "==> [1/7] Checking gofmt..."
UNFORMATTED="$(gofmt -l cmd internal)"
if [ -n "$UNFORMATTED" ]; then
  echo "Unformatted Go files:"
  echo "$UNFORMATTED"
  exit 1
fi

echo "==> [2/7] Running frontend install, typecheck, test, and build..."
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend typecheck
pnpm --dir frontend test
pnpm --dir frontend build

echo "==> [3/7] Syncing frontend/dist into cmd/singtail-desktop/frontendDist..."
rm -rf cmd/singtail-desktop/frontendDist
mkdir -p cmd/singtail-desktop/frontendDist
cp -R frontend/dist/. cmd/singtail-desktop/frontendDist/

echo "==> [4/7] Running go vet..."
go vet ./...

echo "==> [5/7] Running go test -race..."
go test -race ./...

TMP_BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_BIN_DIR"' EXIT

echo "==> [6/7] Cross-compiling Windows amd64 Wails frameless & service binaries (CGO_ENABLED=0)..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$TMP_BIN_DIR/singtail-service.exe" ./cmd/singtail-service
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop,production -trimpath -ldflags="-s -w -H windowsgui" -o "$TMP_BIN_DIR/singtail.exe" ./cmd/singtail-desktop

if [ "$(uname -s)" = "Darwin" ]; then
  echo "==> [7/7] Compiling macOS arm64 & amd64 Wails frameless & service binaries..."
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$TMP_BIN_DIR/singtail-service-darwin-arm64" ./cmd/singtail-service
  GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags desktop,production -trimpath -ldflags="-s -w" -o "$TMP_BIN_DIR/singtail-darwin-arm64" ./cmd/singtail-desktop
  GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags desktop,production -trimpath -ldflags="-s -w" -o "$TMP_BIN_DIR/singtail-darwin-amd64" ./cmd/singtail-desktop
fi

echo "==> All SingTail remote gates passed!"
ls -lh "$TMP_BIN_DIR"
