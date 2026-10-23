#!/usr/bin/env bash
# Parcours Playwright unique (spec §10) contre un vrai agent et un vrai Docker.
# Agent dans un conteneur (mêmes droits que scripts/dev-agent.sh, port 7879),
# conteneur arrêté hublot-e2e, puis Playwright ; tout est nettoyé à la fin.
# Prérequis : Docker, Node ≥ 22.17 (sur le Mac : export PATH=/opt/homebrew/bin:$PATH),
# `cd web && npx playwright install chromium` fait une fois.
set -euo pipefail
cd "$(dirname "$0")/.."
make web >/dev/null
arch=$(docker version -f '{{.Server.Arch}}')
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o bin/hublot-linux ./cmd/hublot
cleanup() {
  docker rm -f hublot-e2e hublot-e2e-agent >/dev/null 2>&1 || true
  docker rmi -f hublot-e2e-img >/dev/null 2>&1 || true
  docker volume rm -f hublot-e2e-data >/dev/null 2>&1 || true
}
cleanup
trap cleanup EXIT
docker run --name hublot-e2e -e DB_PASSWORD=s3cr3t-e2e -e API_TOKEN=s3cr3t-1 -e API_TOKEN=s3cr3t-2 -e GREETING=bonjour alpine:3 sh -c 'echo "bonjour hublot" > /root/note.txt'
# Image à secrets témoins (spec v0.2 §5.4) : un argument de construction dans
# l'historique (RUN |1 GH_PAT=…), une variable et un label tiers dans la
# configuration. Une couche de quelques octets sur alpine:3, supprimée à la fin.
printf 'FROM alpine:3\nARG GH_PAT\nRUN echo "$GH_PAT" > /dev/null\nENV API_KEY=s3cr3t-img-env GREETING=bonjour-img\nLABEL traefik.http.middlewares.a.basicauth.users=admin:s3cr3t-img-lbl org.opencontainers.image.title=hublot-e2e\n' |
  docker build -q -t hublot-e2e-img --build-arg GH_PAT=s3cr3t-img-arg - >/dev/null
tok=$(docker run --rm -v hublot-e2e-data:/data -v "$PWD/bin/hublot-linux:/hublot:ro" alpine:3 \
  /hublot reset-token --data-dir /data | sed -n 's/^New access token (shown only once): //p')
test -n "$tok"
docker run -d --name hublot-e2e-agent \
  --pid=host --cgroupns=host --cap-add SYS_PTRACE \
  --security-opt apparmor=unconfined --security-opt label=disable \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$PWD/bin/hublot-linux:/hublot:ro" -v hublot-e2e-data:/data \
  -p 127.0.0.1:7879:7878 \
  alpine:3 /hublot --listen 0.0.0.0:7878 --data-dir /data >/dev/null
for _ in $(seq 1 40); do curl -sf -o /dev/null http://127.0.0.1:7879/login && break; sleep 0.5; done
curl -sf -o /dev/null http://127.0.0.1:7879/login || { echo "l'agent ne répond pas" >&2; docker logs hublot-e2e-agent 2>&1 | tail -n 20; exit 1; }
cd web && HUBLOT_URL=http://127.0.0.1:7879 HUBLOT_TOKEN="$tok" npx playwright test
