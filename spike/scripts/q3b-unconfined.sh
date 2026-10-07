#!/usr/bin/env bash
# Q3b: AppArmor and unconfined targets. All Q3 targets ran under docker-default;
# here the targets are --privileged or apparmor=unconfined (nonroot image, so
# CAP_SYS_PTRACE matters), the agent runs with the minimal flags, then with
# apparmor=unconfined on top.
set -euo pipefail
[ "$(id -u)" -eq 0 ] || { echo "run as root (sudo)" >&2; exit 1; }
cd "$(dirname "$0")/.."
MARK=/tmp/hublot-spike
mkdir -p "$MARK" && echo HOST-SECRET > "$MARK/marker"
OUT="$MARK/q3b-$(hostname)"; mkdir -p "$OUT"
MIN_FLAGS="--pid=host --cgroupns=host --cap-add SYS_PTRACE"

docker build -q -t hublot-spike . >/dev/null
docker build -q -t hublot-spike-target-nonroot --build-arg BASE=gcr.io/distroless/static-debian12:nonroot testdata/target >/dev/null
docker rm -f spike-priv spike-unconf >/dev/null 2>&1 || true
docker run -d --name spike-priv --privileged hublot-spike-target-nonroot >/dev/null
docker run -d --name spike-unconf --security-opt apparmor=unconfined hublot-spike-target-nonroot >/dev/null

for t in spike-priv spike-unconf; do
  echo "RESULT target $t: user=$(docker inspect -f '{{.Config.User}}' "$t") apparmor=$(docker inspect -f '{{.AppArmorProfile}}' "$t") privileged=$(docker inspect -f '{{.HostConfig.Privileged}}' "$t")"
done

FAILED=0

# run_case <label> <target> <agent flags>: one agent run, logged, verdict line.
run_case() {
  local label="$1" target="$2" flags="$3" log="$OUT/$1.log" rc=0
  echo "== $label (agent: $flags)"
  # shellcheck disable=SC2086  # flags are split on purpose
  docker run --rm $flags \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$MARK/marker:/tmp/HOST_MARKER:ro" \
    hublot-spike lsproc "$target" /usr/many evil >"$log" 2>&1 || rc=$?
  cat "$log"
  [ "$rc" -eq 0 ] || echo "EXIT $rc (case: $label)"
  if grep -q "RESULT list /usr/many: 1000" "$log" \
     && ! grep -q "securejoin read.*HOST-SECRET" "$log"; then
    echo "RESULT case $label: OK"
  else
    echo "RESULT case $label: ECHEC"
    FAILED=1
  fi
}

run_case a-priv    spike-priv   "$MIN_FLAGS"
run_case a-unconf  spike-unconf "$MIN_FLAGS"
run_case b-priv    spike-priv   "$MIN_FLAGS --security-opt apparmor=unconfined"
run_case b-unconf  spike-unconf "$MIN_FLAGS --security-opt apparmor=unconfined"

docker rm -f spike-priv spike-unconf >/dev/null 2>&1 || true
exit "$FAILED"
