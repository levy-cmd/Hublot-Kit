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
block() { awk '/^docker run -d --name hublot-kit/{on=1} on{print} on&&/ghcr\.io\/levy-cmd\/hublot-kit:0\.1$/{exit}' "$1"; }
code_blocks() { awk '/^ *```/{inside=!inside; next} inside' "$1"; }
# Ancres des titres d'un fichier Markdown, hors blocs de code : minuscules,
# ponctuation retirée (lettres accentuées gardées), espaces changés en tirets.
anchors() {
  perl -CSD -Mutf8 -ne 'if (/^ *```/) { $in = !$in; next } next if $in;
    next unless s/^#+\s+//; chomp; $_ = lc; s/[^\p{L}\p{N}\p{M} _-]//g; s/ /-/g; print "$_\n"' "$1"
}

want=$(hk_spec_command)
[ -n "$want" ] || err "commande docker run introuvable dans la spec §8"

pairs="README.md:README.fr.md docs/en/install.md:docs/fr/installation.md docs/en/security.md:docs/fr/securite.md docs/en/limitations.md:docs/fr/limites.md"
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
grep -qF -- '- "{{ if not .Prerelease }}{{ .Major }}.{{ .Minor }}{{ end }}"' "$gr" || err "$gr : pas d'étiquette X.Y, que cite la doc (:0.1)"
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
    case $ref in "$img:0.1" | "$img@sha256") ;; *) err "$f : image « $ref », attendu $img:0.1 (ou $img@sha256:…)" ;; esac
  done
done

[ $errors = 0 ] || { echo "$errors erreur(s)" >&2; exit 1; }
echo "DOCS OK"
