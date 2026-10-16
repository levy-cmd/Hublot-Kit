#!/usr/bin/env bash
# Vérifie dist/ après `goreleaser release --snapshot --clean --skip=sign` (CI,
# job release-snapshot) ou une vraie release (plan Lancement) : une archive par
# architecture avec le bon contenu, checksums justes, un SBOM SPDX par archive,
# bien formé, qui voit le module de l'agent et figure dans checksums.txt,
# binaires statiques, version injectée, interface embarquée.
#
# Usage : [CHECK_DIST_EXEC="amd64 …"] scripts/check-dist.sh [DOSSIER_DIST]
# CHECK_DIST_EXEC : architectures dont le binaire DOIT être exécuté (`--version`),
# seule preuve que la version est injectée ; échec si l'hôte ne peut pas les
# lancer. Sans exécution, la recherche de la version dans le binaire ne
# discrimine que si la version est unique (snapshot : elle contient le commit) :
# une version courte comme 1.0.0 peut déjà y figurer, venue d'une dépendance.
# Prérequis : jq, go (lit les informations de build sans exécuter le binaire).
set -euo pipefail
cd "$(dirname "$0")/.."
d=${1:-dist}
fail() { echo "ÉCHEC : $*" >&2; exit 1; }
[ -f "$d/metadata.json" ] || fail "$d/metadata.json absent : goreleaser n'a pas tourné"
v=$(jq -r .version "$d/metadata.json")
[ -n "$v" ] && [ "$v" != null ] || fail "version absente de metadata.json"
echo "version : $v"

want_files="LICENSE README.fr.md README.md hublot hublot-kit.service"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
for arch in amd64 arm64; do
  a=hublot-kit_${v}_linux_${arch}.tar.gz
  [ -f "$d/$a" ] || fail "$a absent"
  # LC_ALL=C : ordre des octets ; en en_US ou fr_FR, sort ignore la casse.
  got=$(tar -tzf "$d/$a" | LC_ALL=C sort | tr '\n' ' ' | sed 's/ $//')
  [ "$got" = "$want_files" ] || fail "$a contient « $got », attendu « $want_files »"
  grep -q "  $a\$" "$d/checksums.txt" || fail "$a absent de checksums.txt"
  [ -f "$d/$a.sbom.json" ] || fail "$a.sbom.json absent"
  jq -e '(.spdxVersion | startswith("SPDX-")) and (.packages | length > 0)
         and any(.packages[]; .name == "github.com/levy-cmd/hublot-kit")' "$d/$a.sbom.json" >/dev/null ||
    fail "$a.sbom.json : pas un SPDX JSON qui contient le module github.com/levy-cmd/hublot-kit"
  grep -q "  $a.sbom.json\$" "$d/checksums.txt" || fail "$a.sbom.json absent de checksums.txt"
  mkdir -p "$tmp/$arch"
  tar -xzf "$d/$a" -C "$tmp/$arch"
  info=$(go version -m "$tmp/$arch/hublot")
  # -trimpath retire -ldflags des informations de build : la version se cherche dans le binaire.
  for w in "CGO_ENABLED=0" "GOARCH=$arch" "-trimpath=true" "vcs.modified=false"; do
    case $info in *"$w"*) ;; *) fail "$a : « $w » absent de go version -m" ;; esac
  done
  grep -qF "$v" "$tmp/$arch/hublot" || fail "$a : version $v absente du binaire"
  grep -q '_app/immutable' "$tmp/$arch/hublot" || fail "$a : interface non embarquée"
  how="version trouvée dans le binaire, non exécuté"
  if [ "$(uname -s)/$(uname -m)" = "Linux/$( [ $arch = amd64 ] && echo x86_64 || echo aarch64)" ]; then
    [ "$("$tmp/$arch/hublot" --version)" = "hublot $v" ] || fail "$a : --version"
    how="--version exécuté"
  else
    case " ${CHECK_DIST_EXEC:-} " in
      *" $arch "*) fail "$a : CHECK_DIST_EXEC demande d'exécuter $arch, impossible sur $(uname -s)/$(uname -m)" ;;
    esac
  fi
  echo "ok $a ($how)"
done
(cd "$d" && if command -v sha256sum >/dev/null; then sha256sum -c --quiet checksums.txt; else shasum -a 256 -c --quiet checksums.txt; fi) ||
  fail "checksums.txt"
echo "DIST OK : $v"
