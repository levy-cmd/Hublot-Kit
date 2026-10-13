#!/usr/bin/env bash
# Vérifie une image de release avec la commande exacte de la spec §8, celle de
# la doc (plan Lancement). Seule la référence d'image change : IMAGE remplace
# ghcr.io/levy-cmd/hublot-kit:0.1. Puis : jeton lu dans `docker logs`, connexion,
# /api/meta, l'explorateur sur les cinq cibles de scripts/release-lib.sh, une
# édition et un envoi, et la recréation du conteneur sur le même volume.
# Nettoie à la fin ce que le script a créé, et seulement cela ; refuse de
# toucher un conteneur hublot-kit, un volume hublot-data, une cible ou un
# port 7878 déjà présents.
#
# Usage : scripts/check-image.sh IMAGE [VERSION_ATTENDUE]   (en root sur les VM)
#   scripts/check-image.sh ghcr.io/levy-cmd/hublot-kit:0.1.0-rc.1 0.1.0-rc.1
#   scripts/check-image.sh ghcr.io/levy-cmd/hublot-kit:0.1           (commande inchangée)
set -euo pipefail
cd "$(dirname "$0")/.."
. scripts/release-lib.sh
image=${1:?usage : scripts/check-image.sh IMAGE [VERSION_ATTENDUE]}
want=${2:-}

# Commande de §8 vérifiée et découpée en arguments (HK_RUN), avant tout
# appel à docker ; lancée sans shell.
hk_spec_args "$image"

# Garde : on sort avant le trap, sans rien toucher, si quelque chose de
# l'utilisateur porte déjà un des noms ou écoute sur le port.
if docker container inspect hublot-kit >/dev/null 2>&1; then hk_fail "un conteneur hublot-kit existe déjà, rien n'est touché"; fi
if docker volume inspect hublot-data >/dev/null 2>&1; then hk_fail "un volume hublot-data existe déjà, rien n'est touché"; fi
hk_check_no_targets
# curl sort en 7 quand la connexion est refusée : tout autre code veut dire
# que quelque chose écoute déjà (on s'y connecterait à la place de l'image).
rc=0
curl -s -o /dev/null --max-time 2 "$HK_BASE/" || rc=$?
[ "$rc" = 7 ] || hk_fail "le port 7878 est déjà pris, rien n'est touché (curl : $rc)"

# Le nettoyage ne supprime que ce que ce script a créé (drapeaux posés juste
# avant chaque création), et va jusqu'au bout même si une étape échoue.
made_ctr= made_vol= jar=
cleanup() {
  set +e
  [ -z "$made_ctr" ] || docker rm -f hublot-kit >/dev/null 2>&1
  [ -z "$made_vol" ] || docker volume rm -f hublot-data >/dev/null 2>&1
  hk_remove_targets
  [ -z "$jar" ] || rm -f "$jar"
}
trap cleanup EXIT
jar=$(mktemp)

hk_start_targets
printf '+ %s\n' "${HK_RUN[*]}"
# `docker run` crée le conteneur et, à la première fois, le volume nommé.
made_ctr=1 made_vol=1
"${HK_RUN[@]}" >/dev/null
hk_wait_login
logs=$(docker logs hublot-kit 2>&1)
tok=$(printf '%s\n' "$logs" | sed -n 's/^Access token (shown only once): //p')
# Les journaux ne s'affichent que masqués (hk_redact) : le jeton n'entre
# jamais dans la sortie, ni dans celle de la CI.
[ -n "$tok" ] || hk_fail "jeton absent de docker logs : $(printf '%s\n' "$logs" | hk_redact)"
# L'avertissement « without TLS » est attendu dans l'image (0.0.0.0 publié sur 127.0.0.1, spec §7) ;
# celui de l'espace de noms cgroup ne doit jamais paraître (spec §8).
case $logs in *"private cgroup namespace"*) hk_fail "avertissement cgroup au démarrage : $(printf '%s\n' "$logs" | hk_redact)" ;; esac
hk_login "$tok" "$jar"
hk_check_meta "$jar" "$want"
hk_check_targets "$jar"
hk_check_writes "$jar"

# Mise à jour (docs, « Upgrade ») : conteneur recréé sur le même volume, même
# jeton, et aucun nouveau jeton affiché.
docker rm -f hublot-kit >/dev/null
"${HK_RUN[@]}" >/dev/null
hk_wait_login
case $(docker logs hublot-kit 2>&1) in *"Access token"*) hk_fail "nouveau jeton après recréation : le volume n'a pas gardé l'empreinte" ;; esac
hk_login "$tok" "$jar"
echo "ok mise à jour (même volume, même jeton)"
echo "IMAGE OK : $image"
