#!/usr/bin/env bash
# Q3b: AppArmor / SELinux and unconfined targets. All Q3 targets ran under the
# default confinement; here the targets are --privileged, apparmor=unconfined,
# label=disable (nonroot image, so CAP_SYS_PTRACE matters) plus one ROOT
# --privileged target. The agent runs with the minimal flags, then with
# apparmor=unconfined, label=disable, or both on top.
# OK = agent rc 0 AND "RESULT list /usr/many: 1000" AND a "securejoin read evil:"
# line containing "no such file" AND no securejoin HOST-SECRET.
set -euo pipefail
[ "$(id -u)" -eq 0 ] || { echo "run as root (sudo)" >&2; exit 1; }
cd "$(dirname "$0")/.."
MARK=/tmp/hublot-spike
mkdir -p "$MARK" && echo HOST-SECRET > "$MARK/marker"
OUT="$MARK/q3b-$(hostname)"; mkdir -p "$OUT"
: > "$OUT/q3b-denials.txt"
MIN_FLAGS="--pid=host --cgroupns=host --cap-add SYS_PTRACE"
TARGETS="spike-priv spike-unconf spike-nolabel spike-rootpriv"

cleanup() { docker rm -f $TARGETS >/dev/null 2>&1 || true; }
trap cleanup EXIT

docker build -q -t hublot-spike . >/dev/null
docker build -q -t hublot-spike-target testdata/target >/dev/null
docker build -q -t hublot-spike-target-nonroot --build-arg BASE=gcr.io/distroless/static-debian12:nonroot testdata/target >/dev/null
cleanup
NR=hublot-spike-target-nonroot
docker run -d --name spike-priv --privileged $NR >/dev/null
docker run -d --name spike-unconf --security-opt apparmor=unconfined $NR >/dev/null
docker run -d --name spike-nolabel --security-opt label=disable $NR >/dev/null
docker run -d --name spike-rootpriv --privileged hublot-spike-target >/dev/null

for t in $TARGETS; do
  echo "RESULT target $t: user=$(docker inspect -f '{{.Config.User}}' "$t") apparmor=$(docker inspect -f '{{.AppArmorProfile}}' "$t") label=$(docker inspect -f '{{.ProcessLabel}}' "$t") privileged=$(docker inspect -f '{{.HostConfig.Privileged}}' "$t")"
done

FAILED=0
SUMMARY=""

# run_case <label> <target> <agent flags>: one agent run, logged, verdict line.
run_case() {
  local label="$1" target="$2" flags="$3" log="$OUT/$1.log" rc=0 ok=1
  echo "== $label (agent: $flags)"
  # shellcheck disable=SC2086  # flags are split on purpose
  docker run --rm $flags \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$MARK/marker:/tmp/HOST_MARKER:ro" \
    hublot-spike lsproc "$target" /usr/many evil >"$log" 2>&1 || rc=$?
  cat "$log"
  [ "$rc" -eq 0 ] || { echo "EXIT $rc (case: $label)"; ok=0; }
  grep -q "RESULT list /usr/many: 1000" "$log" || ok=0
  grep "RESULT securejoin read evil:" "$log" | grep -q "no such file" || ok=0
  ! grep -q "securejoin read.*HOST-SECRET" "$log" || ok=0
  if [ "$ok" -eq 1 ]; then
    echo "RESULT case $label: OK"
    SUMMARY="$SUMMARY$(printf '%-22s rc=%-3s OK' "$label" "$rc")
"
  else
    echo "RESULT case $label: ECHEC"
    SUMMARY="$SUMMARY$(printf '%-22s rc=%-3s ECHEC' "$label" "$rc")
"
    FAILED=1
    {
      echo "=== denials after $label ($(date -u +%FT%TZ))"
      dmesg 2>&1 | grep -iE 'apparmor|avc' | tail -20 || true
      if command -v ausearch >/dev/null 2>&1; then
        ausearch -m avc -ts recent 2>/dev/null | tail -20 || true
      fi
    } >> "$OUT/q3b-denials.txt"
  fi
}

AA="--security-opt apparmor=unconfined"
LD="--security-opt label=disable"
for t in $TARGETS; do
  short="${t#spike-}"
  run_case "a-$short" "$t" "$MIN_FLAGS"
  run_case "b-$short" "$t" "$MIN_FLAGS $AA"
  run_case "c-$short" "$t" "$MIN_FLAGS $LD"
  run_case "d-$short" "$t" "$MIN_FLAGS $AA $LD"
done

echo "== SUMMARY (a=min, b=+apparmor=unconfined, c=+label=disable, d=both)"
printf '%s' "$SUMMARY"
echo "denials: $OUT/q3b-denials.txt"
exit "$FAILED"
