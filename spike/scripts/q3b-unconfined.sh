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

cleanup() { docker rm -f $TARGETS $(docker ps -aq --filter name=spike-agent- 2>/dev/null) >/dev/null 2>&1 || true; }
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

HAVE_AUSEARCH=no; command -v ausearch >/dev/null 2>&1 && HAVE_AUSEARCH=yes
AUDITD=absent
if command -v systemctl >/dev/null 2>&1; then
  AUDITD=$(systemctl is-active auditd 2>/dev/null || true)
  case "$AUDITD" in active) ;; "") AUDITD=absent ;; *) AUDITD=inactive ;; esac
fi
echo "RESULT audit: ausearch=$HAVE_AUSEARCH auditd=$AUDITD"

FAILED=0
SUMMARY=""

# run_case <label> <target> <agent flags>: one agent run, logged, verdict line.
run_case() {
  local label="$1" target="$2" flags="$3" log="$OUT/$1.log" rc=0 ok=1
  local t0; t0=$(date +%H:%M:%S)
  echo "== $label (agent: $flags)"
  # shellcheck disable=SC2086  # flags are split on purpose
  docker create --name "spike-agent-$label" $flags \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$MARK/marker:/tmp/HOST_MARKER:ro,z" \
    hublot-spike lsproc "$target" /usr/many evil >/dev/null
  docker start -a "spike-agent-$label" >"$log" 2>&1 || rc=$?
  # the profile and label are only populated once the container has started
  echo "RESULT agent $label: apparmor=$(docker inspect -f '{{.AppArmorProfile}}' "spike-agent-$label") label=$(docker inspect -f '{{.ProcessLabel}}' "spike-agent-$label")"
  docker rm -f "spike-agent-$label" >/dev/null 2>&1 || true
  cat "$log"
  [ "$rc" -eq 0 ] || { echo "EXIT $rc (case: $label)"; ok=0; }
  grep -q "RESULT list /usr/many: 1000" "$log" || ok=0
  grep -qE 'RESULT securejoin read evil:.*no such file' "$log" || ok=0
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
      echo "=== case $label (start $t0, flags: $flags)"
      echo "--- dmesg (non attribuable)"
      dmesg 2>&1 | grep -iE 'apparmor|avc' | tail -20 || true
      echo "--- ausearch -m avc -ts $t0"
      if [ "$HAVE_AUSEARCH" = yes ] && [ "$AUDITD" = active ]; then
        ausearch -m avc -ts "$t0" 2>/dev/null | tail -20 || true
      else
        echo "AVC capture indisponible (ausearch=$HAVE_AUSEARCH auditd=$AUDITD)"
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
