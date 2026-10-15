#!/usr/bin/env bash
# Vérifie l'unité systemd de la release (spec §8, alternative : binaire lancé
# en root hors conteneur) avec le binaire d'une archive, en suivant les
# commandes de docs/en/install.md : install, unité, reset-token avant le
# premier démarrage (le jeton n'entre jamais dans le journal), enable --now.
# Puis l'explorateur sur les cinq cibles de scripts/release-lib.sh, une
# édition et un envoi (sous le durcissement de l'unité, et SELinux sur Fedora),
# la rotation du jeton, et la désinstallation complète. Refuse si Hublot-Kit est
# déjà installé sur la machine.
#
# Usage (root, sur hk-ubuntu ou hk-fedora) :
#   sudo scripts/check-systemd.sh /tmp/hublot-kit_0.1.0_linux_arm64.tar.gz 0.1.0
# La version attendue est obligatoire : elle est comparée à `hublot --version`
# et à /api/meta.
set -euo pipefail
cd "$(dirname "$0")/.."
. scripts/release-lib.sh
usage="usage : scripts/check-systemd.sh ARCHIVE.tar.gz VERSION_ATTENDUE"
archive=${1:?$usage}
want=${2:?$usage}
[ "$(id -u)" = 0 ] || hk_fail "à lancer en root"

# Garde : si Hublot-Kit (fichiers, unité connue de systemd, surcharge), une
# cible ou le port 7878 sont déjà là, on sort avant le trap, sans rien toucher.
for p in /usr/local/bin/hublot /var/lib/hublot-kit \
  /etc/systemd/system/hublot-kit.service /etc/systemd/system/hublot-kit.service.d \
  /run/systemd/system/hublot-kit.service /run/systemd/system/hublot-kit.service.d \
  /usr/lib/systemd/system/hublot-kit.service /usr/lib/systemd/system/hublot-kit.service.d \
  /lib/systemd/system/hublot-kit.service /lib/systemd/system/hublot-kit.service.d \
  /etc/systemd/system/multi-user.target.wants/hublot-kit.service; do
  if [ -e "$p" ] || [ -L "$p" ]; then hk_fail "$p existe déjà : Hublot-Kit est installé, rien n'est touché"; fi
done
if systemctl cat hublot-kit.service >/dev/null 2>&1; then
  hk_fail "systemd connaît déjà hublot-kit.service, rien n'est touché"
fi
busy=$(ss -ltnH 'sport = :7878')
[ -z "$busy" ] || hk_fail "le port 7878 est déjà pris, rien n'est touché : $busy"
hk_check_no_targets

# Le nettoyage ne supprime que ce que ce script a créé (drapeaux posés juste
# avant chaque création), et va jusqu'au bout même si une étape échoue.
made_bin= made_unit= made_data= work= jar=
cleanup() {
  set +e
  if [ -n "$made_unit" ]; then
    systemctl disable --now hublot-kit >/dev/null 2>&1
    rm -f /etc/systemd/system/hublot-kit.service
  fi
  [ -z "$made_bin" ] || rm -f /usr/local/bin/hublot
  [ -z "$made_data" ] || rm -rf /var/lib/hublot-kit
  [ -z "$work" ] || rm -rf "$work"
  [ -z "$jar" ] || rm -f "$jar"
  [ -z "$made_unit" ] || systemctl daemon-reload
  hk_remove_targets
}
trap cleanup EXIT
work=$(mktemp -d)
jar=$(mktemp)

# Journal de l'unité depuis le début du test, capturé d'abord : sous pipefail,
# `journalctl | grep -q` peut finir en SIGPIPE (141) et masquer une fuite.
# Aucun jeton ne doit y paraître : ni celui du test, ni un autre (l'agent
# affiche « … (shown only once): <jeton> » quand il crée ou renouvelle un
# jeton, par exemple à un premier démarrage sans reset-token préalable).
check_journal() {
  journalctl --sync
  journal=$(journalctl -u hublot-kit --since "$since" --no-pager)
  case $journal in *"$tok"*) hk_fail "le jeton est dans le journal" ;; esac
  case $journal in *"shown only once"*) hk_fail "un jeton est affiché dans le journal" ;; esac
  case $journal in *WARNING*) hk_fail "avertissement au démarrage (127.0.0.1 attendu, cgroup de l'hôte)" ;; esac
}

tar -xzf "$archive" -C "$work"
# Les commandes de docs/en/install.md, section « Binary and systemd ».
made_bin=1
install -m 0755 "$work/hublot" /usr/local/bin/hublot
# Le binaire de l'archive s'exécute sur cette architecture et porte sa version
# (la CI ne peut exécuter que l'amd64).
ver=$(/usr/local/bin/hublot --version) || hk_fail "hublot --version : $ver"
echo "version : $ver"
[ "$ver" = "hublot $want" ] || hk_fail "hublot --version : « $ver », attendu « hublot $want »"
made_unit=1
install -m 0644 "$work/hublot-kit.service" /etc/systemd/system/hublot-kit.service
# Seuls les avertissements sur notre unité comptent (verify parle aussi de ses dépendances).
out=$(systemd-analyze verify /etc/systemd/system/hublot-kit.service 2>&1) || hk_fail "systemd-analyze : $out"
case $out in *hublot-kit*) hk_fail "systemd-analyze : $out" ;; esac
made_data=1
tok=$(/usr/local/bin/hublot reset-token | sed -n 's/^New access token (shown only once): //p')
[ -n "$tok" ] || hk_fail "reset-token n'a pas donné de jeton"
since=$(date '+%Y-%m-%d %H:%M:%S')
systemctl daemon-reload
systemctl enable --now hublot-kit

hk_start_targets
hk_wait_login
check_journal
[ "$(stat -c %a /var/lib/hublot-kit)" = 700 ] || hk_fail "/var/lib/hublot-kit n'est pas en 0700"
hk_login "$tok" "$jar"
hk_check_meta "$jar" "$want"
hk_check_targets "$jar"
hk_check_writes "$jar"
for prop in NoNewPrivileges=yes ProtectSystem=strict PrivateTmp=yes User= ; do
  systemctl show hublot-kit -p "${prop%%=*}" | grep -qx "$prop" || hk_fail "propriété $prop : $(systemctl show hublot-kit -p "${prop%%=*}")"
done
if command -v getenforce >/dev/null && [ "$(getenforce)" = Enforcing ]; then
  ctx=$(ps -o label= -p "$(systemctl show hublot-kit -p MainPID --value)")
  echo "SELinux : $ctx"
  case $ctx in *unconfined_service_t*) ;; *) hk_fail "contexte SELinux inattendu : $ctx" ;; esac
fi

# Rotation : nouveau jeton, redémarrage, l'ancien est refusé.
new=$(/usr/local/bin/hublot reset-token | sed -n 's/^New access token (shown only once): //p')
systemctl restart hublot-kit
hk_wait_login
code=$(hk_login_code "$tok")
[ "$code" = 401 ] || hk_fail "ancien jeton après rotation : $code"
hk_login "$new" "$jar"
check_journal
echo "SYSTEMD OK : $archive"
