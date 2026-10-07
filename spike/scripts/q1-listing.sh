#!/usr/bin/env bash
# Q1: list containers through /proc/<pid>/root, with symlink traps.
# AGENT_FLAGS: extra flags for the agent container (default: --cgroupns=host --cap-add SYS_PTRACE).
set -euo pipefail
cd "$(dirname "$0")/.."
AGENT_FLAGS="${AGENT_FLAGS-"--cgroupns=host --cap-add SYS_PTRACE"}"
MARK=/tmp/hublot-spike
mkdir -p "$MARK" && echo HOST-SECRET > "$MARK/marker"

docker build -q -t hublot-spike . >/dev/null
docker build -q -t hublot-spike-target testdata/target >/dev/null
docker build -q -t hublot-spike-target-nonroot --build-arg BASE=gcr.io/distroless/static-debian12:nonroot testdata/target >/dev/null
docker rm -f spike-root spike-nonroot >/dev/null 2>&1 || true
docker run -d --name spike-root hublot-spike-target >/dev/null
docker run -d --name spike-nonroot --read-only hublot-spike-target-nonroot >/dev/null

FAILED=0

run_agent() {
  # shellcheck disable=SC2086  # AGENT_FLAGS is split on purpose
  docker run --rm --pid=host $AGENT_FLAGS \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$MARK/marker:/tmp/HOST_MARKER:ro" \
    hublot-spike lsproc "$@"
}

# scenario <label> <args...>: runs the agent, reports failure without stopping.
scenario() {
  local label="$1"; shift
  echo "== $label"
  local rc=0
  run_agent "$@" || rc=$?
  if [ "$rc" -ne 0 ]; then
    echo "EXIT $rc (scenario: $label)"
    FAILED=1
  fi
}

echo "RESULT nonroot user: $(docker inspect -f '{{.Config.User}}' spike-nonroot)"
scenario "cible root" spike-root /usr/many evil evil-rel
scenario "noms bizarres" spike-root /odd
scenario "cible nonroot, --read-only" spike-nonroot /usr/many evil

PID_BEFORE="$(docker inspect -f '{{.State.Pid}}' spike-root)"
docker restart -t 0 spike-root >/dev/null
PID_AFTER="$(docker inspect -f '{{.State.Pid}}' spike-root)"
echo "RESULT restart pid $PID_BEFORE -> $PID_AFTER"
if [ "$PID_BEFORE" = "$PID_AFTER" ]; then
  echo "EXIT 1 (scenario: restart, pid did not change)"
  FAILED=1
fi
scenario "après redémarrage (nouveau PID)" spike-root /usr/many

exit "$FAILED"
