#!/usr/bin/env bash
# Contrôles de la documentation publique (plan Lancement, spec §8 et §12) :
# - chaque page existe en anglais et en français, avec le même nombre de titres
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
  [ "$(grep -c '^#' "$en")" = "$(grep -c '^#' "$fr")" ] || err "$en et $fr : nombre de titres différent"
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

[ $errors = 0 ] || { echo "$errors erreur(s)" >&2; exit 1; }
echo "DOCS OK"
