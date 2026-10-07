#!/usr/bin/env bash
# Q2: same manipulations, then /changes, on the current storage backend.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=/tmp/hublot-spike; mkdir -p "$OUT"
DRIVER=$(docker info -f '{{.Driver}}')
docker build -q -t hublot-spike . >/dev/null
docker rm -f spike-changes >/dev/null 2>&1 || true
docker run -d --name spike-changes alpine:3 \
  sh -c 'echo added > /added.txt; echo changed >> /etc/profile; rm -f /etc/issue; sleep 3600' >/dev/null
sleep 2
docker restart -t 0 spike-changes >/dev/null            # changes must survive a restart
docker exec spike-changes sh -c 'echo second > /second-run.txt'
docker stop -t 0 spike-changes >/dev/null                # /changes must work on a stopped container
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock hublot-spike changes spike-changes \
  | tee "$OUT/changes-$DRIVER.txt"
echo "driver=$DRIVER"
