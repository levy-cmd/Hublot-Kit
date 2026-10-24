#!/usr/bin/env bash
# Vérifie une release faite par .github/workflows/release.yml, brouillon
# compris (plan Lancement) : bundles cosign keyless de checksums.txt et des
# archives (identité : ce workflow, sur ce tag), sommes de contrôle, SBOM des
# archives (SPDX JSON bien formé, module de l'agent présent, empreinte dans
# checksums.txt signé), liste exacte des fichiers de la release, signature de
# l'image, plateformes, attestation SBOM de chaque plateforme (validée de même).
#
# Ce que les signatures couvrent : les archives et leurs SBOM (par
# checksums.txt signé, et chaque archive par son propre bundle), et l'index de
# l'image, qui désigne par empreinte les images de chaque plateforme et leurs
# attestations SBOM. Elles prouvent que ces fichiers sortent du workflow de
# release de ce dépôt, lancé sur ce tag, et n'ont pas changé depuis.
# Ce qu'elles ne couvrent pas : la relecture du code du tag ; l'exactitude et
# l'exhaustivité des SBOM (ce que syft et BuildKit ont trouvé dans le binaire) ;
# le contenu de l'interface, construite par `npm ci` à partir de
# web/package-lock.json et embarquée dans le binaire signé (le job web à part
# protège les jetons de signature, pas ce que npm produit) ;
# le texte de la page de release ; les bundles .sigstore.json eux-mêmes (absents
# de checksums.txt : chacun porte son certificat, vérifié par cosign) ; aucune
# provenance SLSA n'est publiée en v0.1 et v0.2.
#
# Usage : scripts/verify-release.sh vX.Y.Z[-rc.N] [IMAGE_TAG_EN_PLUS ...]
#   scripts/verify-release.sh v0.1.0-rc.1
#   scripts/verify-release.sh v0.1.0 0.1       (vérifie aussi ghcr.io/…:0.1)
# Prérequis (Linux) : gh authentifié (lecture d'un brouillon : droit d'écriture
# sur le dépôt), cosign 3.x, jq, coreutils GNU, docker avec buildx, accès en
# lecture au paquet GHCR.
set -euo pipefail
tag=${1:?usage : scripts/verify-release.sh vX.Y.Z[-rc.N] [TAG_IMAGE ...]}
shift
v=${tag#v}
repo=levy-cmd/Hublot-Kit
image=ghcr.io/levy-cmd/hublot-kit
module=github.com/levy-cmd/hublot-kit
identity="https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag"
issuer=https://token.actions.githubusercontent.com
fail() { echo "ÉCHEC : $*" >&2; exit 1; }
blob() {
  cosign verify-blob --bundle "$1.sigstore.json" --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$1" ||
    fail "signature de $1"
}
# spdx_ok : lit un document SPDX JSON sur l'entrée standard.
spdx_ok() {
  jq -e --arg m "$module" '(.spdxVersion | startswith("SPDX-")) and (.packages | length > 0)
    and any(.packages[]; .name == $m)' >/dev/null
}

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
gh release download "$tag" -R "$repo" -D "$dir"
cd "$dir"
blob checksums.txt
sha256sum -c --quiet checksums.txt || fail "checksums.txt"
want=$(printf '%s\n' checksums.txt checksums.txt.sigstore.json)
for arch in amd64 arm64; do
  a=hublot-kit_${v}_linux_${arch}.tar.gz
  blob "$a"
  grep -q "  $a.sbom.json\$" checksums.txt || fail "$a.sbom.json absent de checksums.txt signé"
  spdx_ok < "$a.sbom.json" || fail "$a.sbom.json : SPDX JSON invalide ou sans $module"
  want=$(printf '%s\n' "$want" "$a" "$a.sbom.json" "$a.sigstore.json")
  echo "ok $a"
done
# Rien d'autre dans la release : un fichier ajouté ne serait couvert par aucune signature.
have=$(find . -mindepth 1 -maxdepth 1 -printf '%P\n' | LC_ALL=C sort)
[ "$have" = "$(LC_ALL=C sort <<<"$want")" ] ||
  fail "fichiers de la release : $(tr '\n' ' ' <<<"$have")(attendu : $(tr '\n' ' ' <<<"$want"))"

for t in "$v" "$@"; do
  cosign verify --certificate-identity "$identity" --certificate-oidc-issuer "$issuer" "$image:$t" >/dev/null ||
    fail "signature de $image:$t"
  raw=$(docker buildx imagetools inspect --raw "$image:$t") || fail "index de $image:$t illisible"
  sbom=$(docker buildx imagetools inspect "$image:$t" --format '{{ json .SBOM }}') || fail "SBOM de $image:$t illisible"
  for p in amd64 arm64; do
    jq -e --arg a "$p" 'any(.manifests[]; .platform.os == "linux" and .platform.architecture == $a)' <<<"$raw" >/dev/null ||
      fail "$image:$t sans linux/$p"
    jq --arg p "linux/$p" '.[$p].SPDX' <<<"$sbom" | spdx_ok || fail "$image:$t : attestation SBOM linux/$p invalide ou sans $module"
  done
  echo "ok $image:$t"
done
echo "RELEASE OK : $tag"
