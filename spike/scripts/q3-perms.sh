#!/usr/bin/env bash
# Q3: try permission variants from weakest to strongest, then the host binary.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=/tmp/hublot-spike; mkdir -p "$OUT"
REPORT="$OUT/q3-$(hostname).txt"; : > "$REPORT"

try() {
  local label="$1" flags="$2" log="$OUT/q3-$(hostname)-$1.log"
  if AGENT_FLAGS="$flags" ./scripts/q1-listing.sh >"$log" 2>&1 \
     && [ "$(grep -c 'RESULT list /usr/many: 1000' "$log")" -eq 3 ] \
     && ! grep -q "securejoin read.*HOST-SECRET" "$log"; then
    echo "OK     $label ($flags)" | tee -a "$REPORT"
  else
    echo "ECHEC  $label ($flags) -> voir $log" | tee -a "$REPORT"
  fi
}

# Every variant carries --cgroupns=host (private cgroupns makes procroot refuse the cgroup path);
# 0-sans-cgroupns documents that failure.
try 0-sans-cgroupns  "--cap-add SYS_PTRACE"
try 1-aucun          "--cgroupns=host"
try 2-ptrace         "--cgroupns=host --cap-add SYS_PTRACE"
try 3-ptrace-aa      "--cgroupns=host --cap-add SYS_PTRACE --security-opt apparmor=unconfined"
try 4-ptrace-label   "--cgroupns=host --cap-add SYS_PTRACE --security-opt label=disable"
try 5-privileged     "--cgroupns=host --privileged"

# Host binary, run as root, no container around the agent (host cgroup namespace already).
docker rm -f q3-bin >/dev/null 2>&1 || true
docker create --name q3-bin hublot-spike lsproc >/dev/null
docker cp q3-bin:/usr/local/bin/lsproc "$OUT/lsproc" >/dev/null
docker rm q3-bin >/dev/null
echo HOST-SECRET | tee /tmp/HOST_MARKER >/dev/null
if "$OUT/lsproc" spike-nonroot /usr/many evil >"$OUT/q3-$(hostname)-binaire.log" 2>&1 \
   && grep -q "RESULT list /usr/many: 1000" "$OUT/q3-$(hostname)-binaire.log"; then
  echo "OK     6-binaire-hote (root, systemd)" | tee -a "$REPORT"
else
  echo "ECHEC  6-binaire-hote -> voir $OUT/q3-$(hostname)-binaire.log" | tee -a "$REPORT"
fi
