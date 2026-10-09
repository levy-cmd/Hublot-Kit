#!/usr/bin/env bash
# Lance l'agent dans un conteneur Linux pour le développement sur macOS
# (spec §8 : avec Docker Desktop, /proc/<pid>/root n'existe que dans la VM).
# Mêmes droits que la commande recommandée. L'interface embarquée est celle du
# dernier `make web` ; pour travailler l'interface, lancez plutôt `npm run dev`
# dans web/ (proxy /api vers 127.0.0.1:7878).
# Premier lancement : le jeton s'affiche. Ensuite :
#   docker run --rm -v hublot-dev-data:/data -v "$PWD/bin/hublot-linux:/hublot:ro" alpine:3 /hublot reset-token --data-dir /data
set -euo pipefail
cd "$(dirname "$0")/.."
arch=$(docker version -f '{{.Server.Arch}}')
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o bin/hublot-linux ./cmd/hublot
docker rm -f hublot-dev >/dev/null 2>&1 || true
exec docker run --rm --name hublot-dev \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD/bin/hublot-linux:/hublot:ro" -v hublot-dev-data:/data \
  -p 127.0.0.1:7878:7878 \
  alpine:3 /hublot --listen 0.0.0.0:7878 --data-dir /data "$@"
