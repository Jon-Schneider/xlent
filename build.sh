#!/usr/bin/env bash
# Builds the xlent binary at the repo root after vetting and testing.
# Usage: ./build.sh [--fast]   (--fast skips vet and tests)
set -euo pipefail
cd "$(dirname "$0")"

if [[ "${1:-}" != "--fast" ]]; then
	go vet ./...
	go test ./...
fi

go build -trimpath -o xlent ./cmd/xlent
echo "built ./xlent"
