#!/usr/bin/env bash
# Q4: stopped container with a named volume: HEAD/GET archive and full index.
set -euo pipefail
cd "$(dirname "$0")/.."
docker build -q -t hublot-spike . >/dev/null
docker rm -f spike-q4 >/dev/null 2>&1 || true
docker volume rm spike-vol >/dev/null 2>&1 || true
docker volume create spike-vol >/dev/null
docker run --name spike-q4 -v spike-vol:/data alpine:3 \
  sh -c 'echo vol > /data/in-volume.txt; echo root > /in-root.txt'
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock hublot-spike archive spike-q4
