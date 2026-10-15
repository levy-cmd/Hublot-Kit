#!/usr/bin/env bash
# Éprouve scripts/verify-release.sh sur de faux jeux d'artefacts, sans réseau :
# faux gh, cosign et docker dans bin/ (identité et émetteur comparés à
# l'identique, empreinte du fichier comparée à celle du bundle, toute autre
# option refusée). Chaque cas part d'une release juste, la dégrade, et exige
# le refus avec le motif attendu ; les cas justes exigent RELEASE OK.
# Ce que ce banc ne prouve pas : le comportement du vrai cosign, de GHCR et de
# gh sur un brouillon (pré-version, tâche 9 du plan Lancement).
# Usage : scripts/test/verify-release/run.sh [CHEMIN/verify-release.sh]
#   Linux seulement : bash, jq, coreutils et findutils GNU (job CI verify-release-bench).
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
target=${1:-$here/../../verify-release.sh}
vr=$(cd "$(dirname "$target")" && pwd)/$(basename "$target")
export PATH="$here/bin:$PATH"
ISS=https://token.actions.githubusercontent.com
SBOM_OK='{"spdxVersion":"SPDX-2.3","packages":[{"name":"github.com/levy-cmd/hublot-kit"},{"name":"stdlib"}]}'
idf() { echo "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/$1"; }
fails=0

# sign FICHIER [IDENTITÉ] [ÉMETTEUR] : faux bundle Sigstore du fichier.
sign() {
  jq -n --arg i "${2:-$(idf "$TAG")}" --arg o "${3:-$ISS}" --arg s "$(sha256sum "$1" | cut -d' ' -f1)" \
    '{identity: $i, issuer: $o, sha256: $s}' > "$1.sigstore.json"
}
resum() { (cd "$R" && sha256sum -- *.tar.gz *.sbom.json > checksums.txt) && sign "$R/checksums.txt"; }
mk_release() {
  R=$FAKE/releases/$TAG; mkdir -p "$R"; local v=${TAG#v} a
  for arch in amd64 arm64; do
    a=$R/hublot-kit_${v}_linux_${arch}.tar.gz
    head -c 256 /dev/urandom > "$a"
    echo "$SBOM_OK" > "$a.sbom.json"
    sign "$a"
  done
  resum
}
mk_image() { # mk_image ÉTIQUETTE [IDENTITÉ]
  local d=$FAKE/images/$1; mkdir -p "$d"
  jq -n --arg i "${2:-$(idf "$TAG")}" --arg o "$ISS" '{identity: $i, issuer: $o}' > "$d/sig.json"
  jq -n '{schemaVersion: 2, manifests: [
    {digest: "sha256:a", platform: {os: "linux", architecture: "amd64"}},
    {digest: "sha256:b", platform: {os: "linux", architecture: "arm64"}},
    {digest: "sha256:c", platform: {os: "unknown", architecture: "unknown"}},
    {digest: "sha256:d", platform: {os: "unknown", architecture: "unknown"}}]}' > "$d/raw.json"
  jq -n --argjson s "$SBOM_OK" '{"linux/amd64": {SPDX: $s}, "linux/arm64": {SPDX: $s}}' > "$d/sbom.json"
}
# check NOM ok|MOTIF -- ARGS… : lance verify-release.sh, compare au résultat attendu.
check() {
  local name=$1 want=$2 out st; shift 3
  out=$("$vr" "$@" 2>&1); st=$?
  if [ "$want" = ok ]; then
    if [ $st = 0 ] && grep -q "^RELEASE OK : $1\$" <<<"$out"; then echo "ok    $name"; return; fi
  elif [ $st != 0 ] && ! grep -q 'RELEASE OK' <<<"$out" && grep -qF -- "$want" <<<"$out"; then
    echo "ok    $name (refusé : $(tail -n 1 <<<"$out"))"; return
  fi
  echo "RATÉ  $name (code $st, attendu $want)"; sed 's/^/      | /' <<<"$out" | tail -n 6; fails=$((fails + 1))
}
fresh() { # fresh TAG : release et image justes
  FAKE=$(mktemp -d); export FAKE; TAG=$1; mk_release; mk_image "${TAG#v}"
}
A=hublot-kit_0.1.0-rc.1_linux_amd64.tar.gz B=hublot-kit_0.1.0-rc.1_linux_arm64.tar.gz

fresh v0.1.0-rc.1; check "pré-version juste" ok -- v0.1.0-rc.1
fresh v0.1.0; mk_image 0.1; check "release finale juste, avec :0.1" ok -- v0.1.0 0.1
fresh v0.1.0-rc.1; check "release absente" "release not found" -- v0.0.0-absent

# Signatures absentes
fresh v0.1.0-rc.1; rm "$R/checksums.txt.sigstore.json"; check "bundle de checksums.txt absent" "ÉCHEC" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; rm "$R/$B.sigstore.json"; check "bundle d'une archive absent" "ÉCHEC" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; rm "$FAKE/images/0.1.0-rc.1/sig.json"; check "image non signée" "ÉCHEC : signature de ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1" -- v0.1.0-rc.1
fresh v0.1.0; check "étiquette :0.1 absente" "ÉCHEC : signature de ghcr.io/levy-cmd/hublot-kit:0.1" -- v0.1.0 0.1

# Mauvaise identité, mauvais émetteur, bundle d'un autre fichier
fresh v0.1.0-rc.1; sign "$R/checksums.txt" "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/heads/main"; check "checksums.txt signé depuis une branche" "ÉCHEC" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; sign "$R/$A" "$(idf v0.1.0-rc.2)"; check "archive signée sur un autre tag" "ÉCHEC" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; sign "$R/$A" "https://github.com/autre/Hublot-Kit/.github/workflows/release.yml@refs/tags/v0.1.0-rc.1"; check "archive signée par un autre dépôt" "ÉCHEC" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; sign "$R/checksums.txt" "" "https://accounts.google.com"; check "autre émetteur OIDC" "ÉCHEC" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; cp "$R/$A.sigstore.json" "$R/$B.sigstore.json"; check "bundle d'une autre archive" "ÉCHEC" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; mk_image 0.1.0-rc.1 "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/ci.yml@refs/tags/v0.1.0-rc.1"; check "image signée par un autre workflow" "ÉCHEC : signature de" -- v0.1.0-rc.1

# Sommes de contrôle
fresh v0.1.0-rc.1; head -c 256 /dev/urandom > "$R/$B"; sign "$R/$B"; check "archive remplacée (checksum faux)" "ÉCHEC : checksums.txt" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; echo '{}' > "$R/$A.sbom.json"; check "SBOM remplacé (checksum faux)" "ÉCHEC : checksums.txt" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; (cd "$R" && sha256sum -- *.tar.gz > checksums.txt) && sign "$R/checksums.txt"; check "SBOM absent de checksums.txt" "absent de checksums.txt" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; echo "pirate" > "$R/notes.sh"; check "fichier en plus dans la release" "ÉCHEC : fichiers de la release" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; rm "$R/$B.sbom.json"; resum; check "SBOM d'une archive manquant" "ÉCHEC" -- v0.1.0-rc.1

# SBOM invalides (signés et listés : seul le contrôle SPDX peut les refuser)
fresh v0.1.0-rc.1; echo '{"spdxVersion":"SPDX-2.3","packages":[{"name":"stdlib"}]}' > "$R/$A.sbom.json"; resum; check "SBOM d'archive sans le module" "ÉCHEC : $A.sbom.json" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; echo 'pas du JSON' > "$R/$B.sbom.json"; resum; check "SBOM d'archive illisible" "ÉCHEC : $B.sbom.json" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; echo '{"bomFormat":"CycloneDX","packages":[{"name":"github.com/levy-cmd/hublot-kit"}]}' > "$R/$A.sbom.json"; resum; check "SBOM d'archive non SPDX" "ÉCHEC : $A.sbom.json" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; jq '.["linux/arm64"].SPDX.packages = []' "$FAKE/images/0.1.0-rc.1/sbom.json" > "$FAKE/s" && mv "$FAKE/s" "$FAKE/images/0.1.0-rc.1/sbom.json"; check "attestation SBOM arm64 vide" "attestation SBOM linux/arm64" -- v0.1.0-rc.1
fresh v0.1.0-rc.1; jq 'del(.["linux/amd64"])' "$FAKE/images/0.1.0-rc.1/sbom.json" > "$FAKE/s" && mv "$FAKE/s" "$FAKE/images/0.1.0-rc.1/sbom.json"; check "attestation SBOM amd64 absente" "attestation SBOM linux/amd64" -- v0.1.0-rc.1

# Plateformes
fresh v0.1.0-rc.1; jq '.manifests |= map(select(.platform.architecture != "arm64"))' "$FAKE/images/0.1.0-rc.1/raw.json" > "$FAKE/s" && mv "$FAKE/s" "$FAKE/images/0.1.0-rc.1/raw.json"; check "image sans linux/arm64" "sans linux/arm64" -- v0.1.0-rc.1

echo "---"
if [ $fails = 0 ]; then echo "TOUS LES CAS OK"; else echo "$fails cas RATÉ(S)"; exit 1; fi
