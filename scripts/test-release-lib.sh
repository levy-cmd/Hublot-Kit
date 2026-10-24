#!/usr/bin/env bash
# Teste les gardes de scripts/release-lib.sh (plan Lancement, tâche 4) sans
# Docker : la commande de §8 lue dans des copies mutées de la spec doit faire
# échouer scripts/check-image.sh avant tout appel à docker ou à curl (de faux
# docker et curl notent chaque appel), et hk_redact doit masquer le jeton.
# Bash 3.2 (macOS) compris.
#
# Usage : scripts/test-release-lib.sh
set -euo pipefail
cd "$(dirname "$0")/.."
spec=docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fails=0
bad() { echo "ÉCHEC : $*" >&2; fails=$((fails + 1)); }

# Faux docker et curl : notent l'appel et échouent.
mkdir "$tmp/bin"
for t in docker curl; do
  printf '#!/bin/sh\necho "%s $*" >> "%s/calls"\nexit 1\n' "$t" "$tmp" > "$tmp/bin/$t"
  chmod +x "$tmp/bin/$t"
done

# mutate NOM DE VERS : copie de la spec où la première ligne égale à DE
# devient VERS (comparaison littérale, via ENVIRON : awk -v interpréterait les \).
mutate() {
  FROM=$2 TO=$3 awk '!done && $0 == ENVIRON["FROM"] {print ENVIRON["TO"]; done=1; next} {print} END {exit !done}' \
    "$spec" > "$tmp/$1.md" || { bad "$1 : ligne « $2 » absente de la spec"; return 1; }
}

# refused NOM MOTIF [IMAGE] : check-image.sh sur la copie NOM doit échouer
# avec MOTIF dans son message, sans aucun appel à docker ni à curl.
refused() {
  : > "$tmp/calls"
  if PATH="$tmp/bin:$PATH" HK_SPEC_FILE="$tmp/$1.md" scripts/check-image.sh "${3:-hk-local-test:arm64}" > "$tmp/out" 2>&1; then
    bad "$1 : check-image.sh a réussi"
  elif [ -s "$tmp/calls" ]; then
    bad "$1 : appel avant le refus : $(cat "$tmp/calls")"
  elif ! grep -q -- "$2" "$tmp/out"; then
    bad "$1 : refus pour une autre raison : $(tail -n 1 "$tmp/out")"
  else
    echo "ok refus $1 : $(tail -n 1 "$tmp/out")"
  fi
}

img='  ghcr.io/levy-cmd/hublot-kit:0.2'
mutate image-010 "$img" "$img.0" && refused image-010 "dernière ligne"
mutate espace-final "$img" "$img " && refused espace-final "dernière ligne"
mutate indentation 'docker run -d --name hublot-kit \' '  docker run -d --name hublot-kit \' && refused indentation introuvable
mutate ligne-en-plus "$img" '  --rm \
'"$img" && refused ligne-en-plus "9 lignes"
mutate ligne-en-moins '  -p 127.0.0.1:7878:7878 \' "$img" && refused ligne-en-moins "7 lignes"
mutate dollar '  -v hublot-data:/data \' '  -v hublot-data:/data$(id) \' && refused dollar "caractère inattendu"
mutate backtick '  -v hublot-data:/data \' '  -v `id`:/data \' && refused backtick "caractère inattendu"
mutate point-virgule '  --pid=host --cgroupns=host --cap-add SYS_PTRACE \' '  --pid=host; id; --cap-add SYS_PTRACE \' && refused point-virgule "caractère inattendu"
mutate espace-apres-continuation '  -p 127.0.0.1:7878:7878 \' '  -p 127.0.0.1:7878:7878 \ ' && refused espace-apres-continuation "7 lignes"
cp "$spec" "$tmp/image-refusee.md" && refused image-refusee "image refusée" 'x;id'
cp "$spec" "$tmp/image-option.md" && refused image-option "image refusée" '--privileged'

# La vraie spec : huit lignes, image locale en dernier argument, plus de
# référence publique ; ou la référence publique gardée telle quelle.
got=$(. scripts/release-lib.sh && hk_spec_args hk-local-test:arm64 && printf '%s|' "${HK_RUN[@]}")
want='docker|run|-d|--name|hublot-kit|--pid=host|--cgroupns=host|--cap-add|SYS_PTRACE|--security-opt|apparmor=unconfined|--security-opt|label=disable|-v|/var/run/docker.sock:/var/run/docker.sock|-v|hublot-data:/data|-p|127.0.0.1:7878:7878|hk-local-test:arm64|'
[ "$got" = "$want" ] && echo "ok spec : image locale" || bad "spec : $got"
pub=ghcr.io/levy-cmd/hublot-kit:0.2
got=$(. scripts/release-lib.sh && hk_spec_args "$pub" && printf '%s|' "${HK_RUN[@]}")
[ "$got" = "${want%hk-local-test:arm64|}$pub|" ] && echo "ok spec : image publique" || bad "spec publique : $got"

# hk_redact : ni le jeton, ni la ligne qui l'affiche.
tok=Ab3_-xY9Ab3_-xY9Ab3_-xY9Ab3_-xY9Ab3_-xY9Ab3
out=$(printf 'WARNING: listening on 0.0.0.0:7878 without TLS.\nAccess token (shown only once): %s\nlater %s here\n' "$tok" "$tok" | (. scripts/release-lib.sh && hk_redact))
case $out in
  *"$tok"* | *Ab3_-xY9*) bad "hk_redact laisse passer le jeton : $out" ;;
  *"without TLS"*"shown only once): [masqué]"*"later [masqué] here"*) echo "ok hk_redact" ;;
  *) bad "hk_redact : $out" ;;
esac

# Garde du démarrage (hk_check_startup_logs, appelée par check-image.sh) : un
# faux journal d'agent avec l'avertissement cgroup, ou le détail procfs « start
# the agent with --cgroupns=host », échoue sans afficher le jeton ;
# l'avertissement « without TLS » seul passe. Remplace la mutation « retirer
# --cgroupns=host » de la commande de §8, que le contrôle de permissions a bloquée.
(. scripts/release-lib.sh && type hk_check_startup_logs > /dev/null 2>&1) || bad "hk_check_startup_logs absente de scripts/release-lib.sh"
grep -qx 'hk_check_startup_logs "$logs"' scripts/check-image.sh || bad "check-image.sh n'applique pas hk_check_startup_logs à docker logs"
cg=$(printf 'WARNING: listening on 0.0.0.0:7878 without TLS.\nWARNING: private cgroup namespace (/proc/self/cgroup = 0::/): the explorer will answer container_changed. Start the agent with --cgroupns=host.\nAccess token (shown only once): %s\n' "$tok")
if (. scripts/release-lib.sh && hk_check_startup_logs "$cg") > "$tmp/out" 2>&1; then
  bad "garde cgroup : journal avec l'avertissement accepté"
elif ! grep -q 'avertissement cgroup' "$tmp/out"; then
  bad "garde cgroup : refus pour une autre raison : $(tail -n 1 "$tmp/out")"
elif grep -qF "$tok" "$tmp/out"; then
  bad "garde cgroup : le jeton est affiché"
else
  echo "ok garde cgroup : $(tail -n 1 "$tmp/out" | cut -c1-60)"
fi
detail='file route error: the container changed: pid 42 is not in the cgroup of container 0123456789ab; start the agent with --cgroupns=host'
if (. scripts/release-lib.sh && hk_check_startup_logs "$detail") > "$tmp/out" 2>&1; then
  bad "garde cgroup : détail procfs accepté"
elif ! grep -q 'avertissement cgroup' "$tmp/out"; then
  bad "garde cgroup (détail procfs) : refus pour une autre raison : $(tail -n 1 "$tmp/out")"
else
  echo "ok garde cgroup (détail procfs)"
fi
(. scripts/release-lib.sh && hk_check_startup_logs 'WARNING: listening on 0.0.0.0:7878 without TLS.') > /dev/null 2>&1 &&
  echo "ok garde cgroup : avertissement TLS seul accepté" || bad "garde cgroup : avertissement TLS seul refusé"

[ "$fails" = 0 ] || { echo "$fails échec(s)" >&2; exit 1; }
echo "RELEASE-LIB OK"
