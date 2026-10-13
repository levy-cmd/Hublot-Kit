# Fonctions communes de scripts/check-image.sh et scripts/check-systemd.sh
# (plan Lancement). À sourcer, pas à lancer. Bash 3.2 (macOS) compris.
#
# Cinq cibles alpine:3, comme le spike n°3 et 3b (spec §8, §11) : ordinaire,
# --privileged, apparmor=unconfined, uid 1000, et un conteneur arrêté. Plus
# une édition et un envoi dans la cible ordinaire (spec §5.3), relus par
# docker exec.

HK_TARGETS="hk-check-normal hk-check-priv hk-check-unconf hk-check-uid hk-check-stopped"
HK_BASE=http://127.0.0.1:7878

hk_fail() { echo "ÉCHEC : $*" >&2; exit 1; }

HK_SPEC_FILE=${HK_SPEC_FILE:-docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md}
HK_IMAGE_REF=ghcr.io/levy-cmd/hublot-kit:0.1
HK_SPEC_LINES=8

# La commande `docker run` de la spec §8, à l'octet près (celle de la doc :
# scripts/check-docs.sh vérifie que la doc la reprend telle quelle) : de la
# ligne `docker run -d --name hublot-kit \` à la première ligne qui ne finit
# pas par une barre oblique inverse. Aucune vérification ici : voir hk_spec_args.
hk_spec_command() {
  awk '/^docker run -d --name hublot-kit/{on=1} on{print; if (!/\\$/) exit}' "$HK_SPEC_FILE"
}

# hk_spec_args IMAGE : remplit HK_RUN (tableau d'arguments, lancé sans shell)
# avec la commande de §8 où IMAGE remplace HK_IMAGE_REF. Refuse, avant tout
# appel à docker, une commande qui n'a pas exactement HK_SPEC_LINES lignes,
# dont la dernière n'est pas HK_IMAGE_REF indentée de deux espaces, ou qui contient un caractère hors
# de [A-Za-z0-9 ./:=_-] (backtick, $, ;, |, &, guillemets, tabulation…).
hk_spec_args() {
  case $1 in ''|-*|*[!A-Za-z0-9./:@_-]*) hk_fail "référence d'image refusée : '$1'" ;; esac
  sc=$(hk_spec_command)
  [ -n "$sc" ] || hk_fail "commande docker run introuvable dans $HK_SPEC_FILE (§8)"
  n=$(printf '%s\n' "$sc" | wc -l | tr -d ' ')
  [ "$n" = "$HK_SPEC_LINES" ] || hk_fail "commande de §8 : $n lignes, $HK_SPEC_LINES attendues"
  [ "$(printf '%s\n' "$sc" | tail -n 1)" = "  $HK_IMAGE_REF" ] || hk_fail "commande de §8 : la dernière ligne n'est pas « $HK_IMAGE_REF » (indentée de deux espaces)"
  # Toutes les lignes sauf la dernière finissent par « \ » (vérifié par awk) ;
  # hors de ces continuations, seuls les caractères attendus passent.
  flat=$(printf '%s\n' "$sc" | sed 's/ \\$//' | tr '\n' ' ')
  case $flat in *[!A-Za-z0-9\ ./:=_-]*) hk_fail "commande de §8 : caractère inattendu dans « $flat »" ;; esac
  # Découpage sur les espaces, sans glob ni expansion (read -a).
  read -r -a HK_RUN <<< "$flat"
  last=$((${#HK_RUN[@]} - 1))
  [ "${HK_RUN[0]} ${HK_RUN[1]}" = "docker run" ] || hk_fail "commande de §8 : ne commence pas par docker run"
  [ "${HK_RUN[$last]}" = "$HK_IMAGE_REF" ] || hk_fail "commande de §8 : l'image n'est pas le dernier argument"
  HK_RUN[$last]=$1
  # Sauf si l'on teste l'image publique elle-même, elle ne doit plus paraître.
  if [ "$1" != "$HK_IMAGE_REF" ]; then
    for a in "${HK_RUN[@]}"; do
      [ "$a" != "$HK_IMAGE_REF" ] || hk_fail "commande de §8 : la référence publique reste après substitution"
    done
  fi
}

# hk_redact : masque, dans un journal à afficher, tout jeton (43 caractères
# base64url, auth.GenerateToken) et le reste de toute ligne « shown only once ».
hk_redact() {
  sed -E 's/(shown only once):.*/\1: [masqué]/; s/[A-Za-z0-9_-]{43,}/[masqué]/g'
}

# Conteneurs créés par ce script, les seuls que hk_remove_targets supprime :
# un conteneur hk-check-* qui existait avant le test n'est jamais touché.
HK_CREATED=

# hk_check_no_targets : refuse si une cible existe déjà (rien n'est touché).
hk_check_no_targets() {
  for c in $HK_TARGETS; do
    if docker container inspect "$c" >/dev/null 2>&1; then hk_fail "le conteneur $c existe déjà"; fi
  done
}

# hk_new_target NOM ARGUMENTS_DE_DOCKER_CREATE... : `docker create` réserve le
# nom (échoue s'il est pris), et le conteneur n'est noté comme nôtre qu'une
# fois créé.
hk_new_target() {
  n=$1
  shift
  docker create --name "$n" "$@" >/dev/null || hk_fail "création du conteneur $n"
  HK_CREATED="$HK_CREATED $n"
}

hk_start_targets() {
  hk_check_no_targets
  docker pull -q alpine:3 >/dev/null
  hk_new_target hk-check-normal alpine:3 sleep 600
  hk_new_target hk-check-priv --privileged alpine:3 sleep 600
  hk_new_target hk-check-unconf --security-opt apparmor=unconfined alpine:3 sleep 600
  hk_new_target hk-check-uid --user 1000:1000 alpine:3 sleep 600
  # Le conteneur arrêté : lancé une fois jusqu'au bout.
  hk_new_target hk-check-stopped alpine:3 true
  docker start hk-check-normal hk-check-priv hk-check-unconf hk-check-uid >/dev/null
  docker start -a hk-check-stopped >/dev/null
  docker exec hk-check-normal sh -c 'echo before > /root/hk-edit.txt'
}

# hk_remove_targets : ne supprime que les conteneurs notés dans HK_CREATED ;
# ne fait jamais échouer le nettoyage.
hk_remove_targets() {
  if [ -n "$HK_CREATED" ]; then
    # shellcheck disable=SC2086
    docker rm -f $HK_CREATED >/dev/null 2>&1 || true
  fi
  HK_CREATED=
}

hk_wait_login() {
  for _ in $(seq 1 60); do
    if curl -sf -o /dev/null "$HK_BASE/login"; then return 0; fi
    sleep 0.5
  done
  hk_fail "l'agent ne répond pas sur $HK_BASE/login"
}

# hk_login_code JETON [FICHIER_COOKIES] : code HTTP de POST /api/login. Le
# jeton passe par stdin (printf est interne à bash), jamais dans les arguments
# de curl, que `ps` montre à tous.
hk_login_code() {
  printf '{"token":"%s"}' "$1" | curl -s ${2:+-c "$2"} -H "Origin: $HK_BASE" -H 'Content-Type: application/json' \
    --data-binary @- -o /dev/null -w '%{http_code}' "$HK_BASE/api/login" || true
}

# hk_login JETON FICHIER_COOKIES
hk_login() {
  [ "$(hk_login_code "$1" "$2")" = 200 ] || hk_fail "connexion refusée"
}

# hk_check_meta FICHIER_COOKIES [VERSION]
hk_check_meta() {
  meta=$(curl -sf -b "$1" "$HK_BASE/api/meta") || hk_fail "/api/meta"
  echo "meta : $meta"
  if [ -n "${2:-}" ]; then
    case $meta in *"\"version\":\"$2\""*) ;; *) hk_fail "version attendue $2" ;; esac
  fi
}

# hk_check_targets FICHIER_COOKIES : /etc de chaque cible actif listé, un
# fichier lu ; le conteneur arrêté répond 202 (indexation) puis 200.
hk_check_targets() {
  for c in hk-check-normal hk-check-priv hk-check-unconf hk-check-uid; do
    out=$(curl -s -b "$1" -w '\n%{http_code}' "$HK_BASE/api/containers/$c/fs?path=/etc")
    code=${out##*$'\n'}
    case "$code $out" in 200*'"name":"hostname"'*) ;; *) hk_fail "$c : $out" ;; esac
    code=$(curl -s -o /dev/null -w '%{http_code}' -b "$1" "$HK_BASE/api/containers/$c/fs/file?path=/etc/os-release")
    [ "$code" = 200 ] || hk_fail "$c : lecture de /etc/os-release → $code"
    echo "ok $c"
  done
  code=
  for _ in $(seq 1 60); do
    code=$(curl -s -o /dev/null -w '%{http_code}' -b "$1" "$HK_BASE/api/containers/hk-check-stopped/fs?path=/etc")
    [ "$code" = 200 ] && break
    sleep 0.5
  done
  [ "$code" = 200 ] || hk_fail "hk-check-stopped : $code"
  echo "ok hk-check-stopped"
}

# hk_check_writes FICHIER_COOKIES : édition (PUT fs/file, If-Match = ETag de
# l'aperçu) puis envoi (POST fs/upload, multipart, If-None-Match: *) dans le
# conteneur actif hk-check-normal ; chaque écriture est relue par docker exec.
hk_check_writes() {
  url="$HK_BASE/api/containers/hk-check-normal"
  etag=$(curl -s -D - -o /dev/null -b "$1" "$url/fs/file?path=/root/hk-edit.txt" | tr -d '\r' | sed -n 's/^[Ee][Tt][Aa][Gg]: //p')
  [ -n "$etag" ] || hk_fail "aperçu de /root/hk-edit.txt sans ETag"
  code=$(printf 'after\n' | curl -s -o /dev/null -w '%{http_code}' -b "$1" -X PUT -H "Origin: $HK_BASE" \
    -H "If-Match: $etag" -H 'Content-Type: application/octet-stream' --data-binary @- "$url/fs/file?path=/root/hk-edit.txt")
  [ "$code" = 200 ] || hk_fail "édition (PUT fs/file) : $code"
  [ "$(docker exec hk-check-normal cat /root/hk-edit.txt)" = after ] || hk_fail "édition absente du conteneur"
  echo "ok édition (PUT fs/file)"
  up=$(mktemp)
  printf 'uploaded\n' > "$up"
  code=$(curl -s -o /dev/null -w '%{http_code}' -b "$1" -H "Origin: $HK_BASE" -H 'If-None-Match: *' \
    -F "file=@$up;filename=hk-upload.txt" "$url/fs/upload?dir=/root")
  rm -f "$up"
  [ "$code" = 201 ] || hk_fail "envoi (POST fs/upload) : $code"
  [ "$(docker exec hk-check-normal cat /root/hk-upload.txt)" = uploaded ] || hk_fail "envoi absent du conteneur"
  echo "ok envoi (POST fs/upload)"
}
