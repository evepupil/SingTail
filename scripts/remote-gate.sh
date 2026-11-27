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

echo "==> [1/6] Checking gofmt..."
UNFORMATTED="$(gofmt -l cmd internal)"
if [ -n "$UNFORMATTED" ]; then
  echo "Unformatted Go files:"
  echo "$UNFORMATTED"
  exit 1
fi

echo "==> [2/6] Running frontend install, typecheck, test, and build..."
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend typecheck
pnpm --dir frontend test
pnpm --dir frontend build

echo "==> [3/6] Syncing frontend/dist into cmd/singtail-desktop/frontendDist..."
rm -rf cmd/singtail-desktop/frontendDist
mkdir -p cmd/singtail-desktop/frontendDist
cp -R frontend/dist/. cmd/singtail-desktop/frontendDist/

echo "==> [4/6] Running go vet..."
go vet ./...

echo "==> [5/6] Running go test -race..."
go test -race ./...

echo "==> [6/6] Cross-compiling Windows amd64 binaries (CGO_ENABLED=0)..."
TMP_BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_BIN_DIR"' EXIT
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o "$TMP_BIN_DIR/" ./cmd/...

echo "==> All SingTail remote gates passed!"
ls -lh "$TMP_BIN_DIR"
