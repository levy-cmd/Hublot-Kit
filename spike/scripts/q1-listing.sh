#!/usr/bin/env bash
# Q1: list containers through /proc/<pid>/root, with symlink traps.
# AGENT_FLAGS: extra flags for the agent container (default: --cap-add SYS_PTRACE).
set -euo pipefail
cd "$(dirname "$0")/.."
AGENT_FLAGS="${AGENT_FLAGS-"--cap-add SYS_PTRACE"}"
MARK=/tmp/hublot-spike
mkdir -p "$MARK" && echo HOST-SECRET > "$MARK/marker"

docker build -q -t hublot-spike . >/dev/null
docker build -q -t hublot-spike-target testdata/target >/dev/null
docker build -q -t hublot-spike-target-nonroot --build-arg BASE=gcr.io/distroless/static-debian12:nonroot testdata/target >/dev/null
docker rm -f spike-root spike-nonroot >/dev/null 2>&1 || true
docker run -d --name spike-root hublot-spike-target >/dev/null
docker run -d --name spike-nonroot --read-only hublot-spike-target-nonroot >/dev/null

run_agent() {
  # shellcheck disable=SC2086  # AGENT_FLAGS is split on purpose
  docker run --rm --pid=host $AGENT_FLAGS \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$MARK/marker:/tmp/HOST_MARKER:ro" \
    hublot-spike lsproc "$@"
}

echo "== cible root";                       run_agent spike-root /usr/many evil evil-rel
echo "== noms bizarres";                    run_agent spike-root /odd
echo "== cible nonroot, --read-only";       run_agent spike-nonroot /usr/many evil
echo "== après redémarrage (nouveau PID)";  docker restart -t 0 spike-root >/dev/null; run_agent spike-root /usr/many
