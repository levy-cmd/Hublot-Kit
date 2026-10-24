#!/usr/bin/env bash
# Contrôles de la documentation publique (plan Lancement, spec §8 et §12) :
# - chaque page existe en anglais et en français, avec les mêmes niveaux de titres
#   et exactement les mêmes blocs de code (les commandes ne se traduisent pas) ;
# - la commande `docker run` de README.md, README.fr.md et des pages
#   d'installation est celle de la spec §8, à l'octet près ;
# - les deux README disent l'accès root dans leurs 12 premières lignes ;
# - tout lien relatif mène à un fichier qui existe, et son ancre (#…) à un
#   titre de ce fichier (ancres calculées comme GitHub, par perl).
# Usage : scripts/check-docs.sh   (bash 3.2 compris)
set -euo pipefail
cd "$(dirname "$0")/.."
. scripts/release-lib.sh
errors=0
err() { echo "ERREUR : $*" >&2; errors=$((errors + 1)); }
block() { awk '/^docker run -d --name hublot-kit/{on=1} on{print} on&&/ghcr\.io\/levy-cmd\/hublot-kit:0\.2$/{exit}' "$1"; }
code_blocks() { awk '/^ *```/{inside=!inside; next} inside' "$1"; }
# Ancres des titres d'un fichier Markdown, hors blocs de code : minuscules,
# ponctuation retirée (lettres accentuées gardées), espaces changés en tirets.
anchors() {
  perl -CSD -Mutf8 -ne 'if (/^ *```/) { $in = !$in; next } next if $in;
    next unless s/^#+\s+//; chomp; $_ = lc; s/[^\p{L}\p{N}\p{M} _-]//g; s/ /-/g; print "$_\n"' "$1"
}

want=$(hk_spec_command)
[ -n "$want" ] || err "commande docker run introuvable dans la spec §8"

pairs="README.md:README.fr.md docs/en/guide.md:docs/fr/guide.md docs/en/install.md:docs/fr/installation.md docs/en/security.md:docs/fr/securite.md docs/en/limitations.md:docs/fr/limites.md"
for p in $pairs; do
  en=${p%%:*} fr=${p##*:}
  ok=1
  for f in "$en" "$fr"; do [ -f "$f" ] || { err "$f absent"; ok=0; }; done
  [ $ok = 1 ] || continue
  # Suite des niveaux de titres (#, ##, ###…) : même nombre, même ordre, même niveau.
  [ "$(grep -o '^#*' "$en")" = "$(grep -o '^#*' "$fr")" ] || err "$en et $fr : titres différents (nombre ou niveau)"
  [ "$(code_blocks "$en")" = "$(code_blocks "$fr")" ] || err "$en et $fr : blocs de code différents"
done

for f in README.md README.fr.md docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  [ "$(block "$f")" = "$want" ] || err "$f : la commande docker run diffère de la spec §8"
done
for f in README.md README.fr.md; do
  [ -f "$f" ] || continue
  head -n 12 "$f" | grep -q '^> \*\*.*root' || err "$f : l'accès root n'est pas dit en tête (12 premières lignes)"
done

for f in README.md README.fr.md docs/en/*.md docs/fr/*.md; do
  [ -f "$f" ] || continue
  dir=$(dirname "$f")
  for link in $(grep -o '](\([^)]*\)' "$f" | sed 's/^](//' | grep -v '^[a-z][a-z]*:' | grep -v '^$' || true); do
    path=${link%%#*}
    target=$f
    if [ -n "$path" ]; then
      target=$dir/$path
      [ -e "$target" ] || { err "$f : lien mort $link"; continue; }
    fi
    case $link in
      *'#'*)
        case $target in *.md) ;; *) continue ;; esac
        anchors "$target" | grep -qxF -- "${link#*#}" || err "$f : ancre morte $link"
        ;;
    esac
  done
done

# Tranche A (plan Lancement) : l'image que cite la doc est celle que publie
# .goreleaser.yaml (dockers_v2), avec l'étiquette X.Y et sans latest (Q43).
gr=.goreleaser.yaml
img=$(awk '/^dockers_v2:/{d=1} d&&/images:/{gsub(/.*\[|\].*/,""); print; exit}' "$gr")
[ "$img" = ghcr.io/levy-cmd/hublot-kit ] || err "$gr : image « $img », la doc cite ghcr.io/levy-cmd/hublot-kit"
grep -qF -- '- "{{ if not .Prerelease }}{{ .Major }}.{{ .Minor }}{{ end }}"' "$gr" || err "$gr : pas d'étiquette X.Y, que cite la doc (:0.2)"
# Toutes les étiquettes des blocs `tags:` de dockers_v2, quelle que soit leur
# forme (liste, guillemets, gabarit, ou liste en ligne `tags: [...]`), hors
# commentaires : aucune ne doit contenir latest, même produite par un gabarit
# (le job release-snapshot contrôle aussi les étiquettes réellement produites).
tags=$(awk '
  /^[^ #]/ { d = /^dockers_v2:/; t = 0; next }
  !d || /^ *#/ || /^ *$/ { next }
  t { match($0, /^ */); if (RLENGTH >= ki && $0 ~ /^ *- /) { print; next } t = 0 }
  /^ *(- )?tags:/ { match($0, /^ *(- )?/); ki = RLENGTH; sub(/^ *(- )?tags:/, ""); if ($0 ~ /[^ ]/) print; else t = 1 }
' "$gr")
[ -n "$tags" ] || err "$gr : aucune étiquette trouvée dans dockers_v2"
! printf '%s\n' "$tags" | grep -qi latest || err "$gr : étiquette latest (Q43) : $(printf '%s\n' "$tags" | grep -i latest | tr -s ' ')"
# Références d'image de la doc, casse comprise : toute variante (Levy-cmd,
# GHCR.IO…) diffère de l'image de .goreleaser.yaml et est refusée.
for f in README.md README.fr.md docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  for ref in $(grep -oiE 'ghcr\.io/[a-z0-9._/-]+(:[a-z0-9._-]+|@sha256)?' "$f" | sort -u); do
    case $ref in "$img:0.2" | "$img@sha256") ;; *) err "$f : image « $ref », attendu $img:0.2 (ou $img@sha256:…)" ;; esac
  done
done

# Tranche B (plan Lancement) : l'archive et l'unité que cite la doc sont
# celles que produit .goreleaser.yaml (archives).
proj=$(sed -n 's/^project_name: *//p' "$gr")
nt=$(awk '/^archives:/{a=1} a&&/name_template:/{sub(/.*name_template: */,""); gsub(/"/,""); print; exit}' "$gr")
[ "$proj" = hublot-kit ] && [ "$nt" = '{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}' ] ||
  err "$gr : archives « $nt » (projet $proj), la doc attend hublot-kit_\${V}_linux_\${ARCH}.tar.gz"
grep -A1 -F 'src: packaging/systemd/hublot-kit.service' "$gr" | grep -q 'strip_parent: true' ||
  err "$gr : l'unité hublot-kit.service n'est pas à la racine de l'archive"
[ -f packaging/systemd/hublot-kit.service ] || err "packaging/systemd/hublot-kit.service absent"
for f in docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  grep -qF 'hublot-kit_${V}_linux_${ARCH}.tar.gz' "$f" || err "$f : nom d'archive de .goreleaser.yaml absent"
  grep -qxF 'sudo install -m 0644 hublot-kit.service /etc/systemd/system/hublot-kit.service' "$f" ||
    err "$f : installation de l'unité absente"
  grep -qxF 'sudo /usr/local/bin/hublot reset-token' "$f" || err "$f : reset-token par chemin complet absent (sudo sur Fedora)"
done
# Toute mention de l'archive, du binaire installé et de l'unité dans la doc
# égale ce que produisent .goreleaser.yaml et l'unité : une commande modifiée
# en anglais ET en français (blocs identiques) ne passe pas.
unit=packaging/systemd/hublot-kit.service
exe=$(sed -n 's/^ExecStart=\([^ ]*\).*/\1/p' "$unit")
svc=$(basename "$unit")
fmt=$(awk '/^archives:/{a=1} a&&/formats:/{gsub(/.*\[ *| *\].*/,""); print; exit}' "$gr")
want_ar=$(printf '%s' "$nt" | sed -e "s/{{ \.ProjectName }}/$proj/" -e 's/{{ \.Version }}/${V}/' \
  -e 's/{{ \.Os }}/linux/' -e 's/{{ \.Arch }}/${ARCH}/').$fmt
[ "$exe" = /usr/local/bin/hublot ] || err "$unit : ExecStart « $exe », la doc installe /usr/local/bin/hublot"
# L'unité écoute en local seulement (spec §7, §8 ; la doc l'annonce sur 127.0.0.1:7878).
listen=$(sed -n 's/^Environment=HUBLOT_LISTEN=//p' "$unit")
[ "$listen" = 127.0.0.1:7878 ] || err "$unit : HUBLOT_LISTEN « $listen », attendu 127.0.0.1:7878"
for f in docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  for a in $(grep -oE '[A-Za-z0-9_${}.-]+\.tar\.gz' "$f" | sort -u); do
    [ "$a" = "$want_ar" ] || err "$f : archive « $a », .goreleaser.yaml produit « $want_ar »"
  done
  for b in $(grep -oE '/[a-z/]*bin/hublot' "$f" | sort -u); do
    [ "$b" = "$exe" ] || err "$f : binaire « $b », l'unité lance « $exe » (ExecStart)"
  done
  for s in $(grep -oE '[A-Za-z0-9_-]+\.service' "$f" | sort -u); do
    [ "$s" = "$svc" ] || err "$f : unité « $s », le fichier est « $svc »"
  done
  for s in $(grep -E 'systemctl ' "$f" | grep -v 'daemon-reload' | awk '{l=$NF; gsub(/[`.,:;)]+$/,"",l); print l}' | sort -u); do
    [ "$s" = "${svc%.service}" ] || err "$f : systemctl sur « $s », l'unité est « ${svc%.service} »"
  done
  # Dossier de données déplacé : sous ProtectSystem=strict, le drop-in qui
  # change HUBLOT_DATA_DIR doit aussi ouvrir ce dossier en écriture
  # (ReadWritePaths=, même chemin), et la doc le crée avant, root en 0700.
  # Une ligne par bloc ```ini qui déplace le dossier : « dossier|ReadWritePaths ».
  moved=$(awk '
    /^```ini/ { b = 1; d = ""; r = ""; next }
    b && /^```/ { b = 0; if (d != "") print d "|" r; next }
    b && /^Environment=HUBLOT_DATA_DIR=/ { sub(/^Environment=HUBLOT_DATA_DIR=/, ""); d = $0; next }
    b && /^ReadWritePaths=/ { sub(/^ReadWritePaths=/, ""); r = $0 }
  ' "$f")
  [ -n "$moved" ] || err "$f : aucun drop-in ne déplace le dossier de données (HUBLOT_DATA_DIR)"
  for m in $moved; do
    d=${m%%|*} r=${m#*|}
    [ "$r" = "$d" ] || err "$f : le drop-in HUBLOT_DATA_DIR=$d n'a pas ReadWritePaths=$d (ProtectSystem=strict)"
    grep -qxF "sudo install -d -m 0700 $d" "$f" || err "$f : $d n'est pas créé avant le drop-in (sudo install -d -m 0700 $d)"
  done
done
grep -qx 'ProtectSystem=strict' "$unit" || err "$unit : ProtectSystem=strict absent, la règle ReadWritePaths de la doc est à revoir"

# Tranche C (plan Lancement) : les commandes de vérification de la doc
# visent ce que signent .goreleaser.yaml et release.yml, avec l'identité et
# l'émetteur que vérifie scripts/verify-release.sh.
wf=.github/workflows/release.yml
vr=scripts/verify-release.sh
[ -f "$wf" ] || err "$wf absent"
grep -qx 'name_template: checksums.txt' <(sed 's/^ *//' "$gr") || err "$gr : checksums.txt renommé"
[ "$(grep -c 'signature: "${artifact}.sigstore.json"' "$gr")" = 2 ] || err "$gr : bundles .sigstore.json (checksums et archives) attendus"
grep -qF 'identity="https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag"' "$vr" ||
  err "$vr : identité différente de celle de la doc"
grep -qxF 'issuer=https://token.actions.githubusercontent.com' "$vr" || err "$vr : émetteur différent de celui de la doc"
# Versions de la doc d'installation (Minor de la revue de la PR #12) : toutes
# les lignes V= des deux docs ont une même valeur X.Y.Z, de la série X.Y de
# l'étiquette d'image des commandes docker run (égales à celle de la spec §8,
# contrôlé plus haut) ; la regex cosign de l'image vise cette même série.
serie=$(printf '%s\n' "$want" | sed -n 's/^ *ghcr\.io\/levy-cmd\/hublot-kit:\([0-9][0-9]*\.[0-9][0-9]*\)$/\1/p')
[ -n "$serie" ] || err "spec §8 : étiquette d'image X.Y introuvable dans la commande docker run"
vs=$(grep -h '^V=' docs/en/install.md docs/fr/installation.md 2>/dev/null | sort -u || true)
case $(printf '%s' "$vs" | awk 'END { print NR }') in
  0) err "docs d'installation : aucune ligne V=" ;;
  1)
    v=${vs#V=}
    if ! printf '%s\n' "$v" | grep -qxE '[0-9]+\.[0-9]+\.[0-9]+'; then
      err "docs d'installation : V=$v n'a pas la forme X.Y.Z"
    elif [ "${v%.*}" != "$serie" ]; then
      err "docs d'installation : V=$v est de la série ${v%.*}, l'image documentée est :$serie (attendu V=$serie.x)"
    fi
    ;;
  *) err "docs d'installation : lignes V= différentes ($(printf '%s\n' "$vs" | tr '\n' ' ' | sed 's/ $//'))" ;;
esac
serie_re=$(printf '%s' "$serie" | sed 's/\./\\./g')
for f in docs/en/install.md docs/fr/installation.md; do
  [ -f "$f" ] || continue
  grep -qF -- '--certificate-identity "https://github.com/levy-cmd/Hublot-Kit/.github/workflows/release.yml@refs/tags/v$V"' "$f" ||
    err "$f : identité de signature des archives"
  grep -qF -- "--certificate-identity-regexp '^https://github\.com/levy-cmd/Hublot-Kit/\.github/workflows/release\.yml@refs/tags/v$serie_re\.[0-9]+\$'" "$f" ||
    err "$f : identité de signature de l'image (regex cosign de la série $serie attendue)"
  grep -qF -- '--certificate-oidc-issuer https://token.actions.githubusercontent.com' "$f" || err "$f : émetteur OIDC"
  grep -qF 'checksums.txt.sigstore.json' "$f" || err "$f : bundle de checksums.txt"
done

[ $errors = 0 ] || { echo "$errors erreur(s)" >&2; exit 1; }
echo "DOCS OK"
