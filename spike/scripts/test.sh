#!/usr/bin/env bash
# Runs the spike's Go tests inside a Linux container (the code is Linux-only).
set -euo pipefail
cd "$(dirname "$0")/.."
docker run --rm -v "$PWD":/src -v hublot-gomod:/go/pkg/mod -w /src golang:1 go test "${@:-./...}"
