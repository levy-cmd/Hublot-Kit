#!/usr/bin/env bash
# Lance les tests Go sous Linux, en root, dans un conteneur golang (CLAUDE.md,
# spec §10). Mêmes droits que l'agent recommandé (spec §8) : le conteneur voit
# /proc/<pid>/root des autres conteneurs et parle au démon par son socket.
# Marche avec Docker Desktop (macOS) et dans les VM Lima hk-ubuntu / hk-fedora.
#
# Usage : scripts/test-linux.sh [arguments de go test] (-timeout 3m par défaut)
#   scripts/test-linux.sh ./internal/fsview/ -run Evasion -v
#   scripts/test-linux.sh -tags integration ./internal/...
set -euo pipefail
cd "$(dirname "$0")/.."
docker build -q -t hublot-test -f scripts/test.Dockerfile scripts >/dev/null
if [ "$#" -eq 0 ]; then set -- ./cmd/... ./internal/...; fi
exec docker run --rm \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD":/src -w /src \
  -v hublot-gomod:/go/pkg/mod -v hublot-gocache:/root/.cache/go-build \
  hublot-test go test -timeout 3m "$@"
