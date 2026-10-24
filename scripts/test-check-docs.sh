#!/usr/bin/env bash
# Teste le contrôle des versions de scripts/check-docs.sh (Minor de la revue
# de la PR #12, registre v0.2) : sur une copie du dépôt, une doc d'installation
# dont une ligne `V=` diffère des autres, n'a pas la forme X.Y.Z, ou n'est pas
# de la série de l'image documentée (:0.2 va avec V=0.2.x), ou dont la regex
# cosign vise une autre série, doit faire échouer check-docs.sh. Les mutations
# touchent les deux docs à la fois : leurs blocs de code restent identiques.
# Bash 3.2 (macOS) compris.
#
# Usage : scripts/test-check-docs.sh
set -euo pipefail
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
fails=0
bad() { echo "ÉCHEC : $*" >&2; fails=$((fails + 1)); }
docs="docs/en/install.md docs/fr/installation.md"

# fresh NOM : copie neuve de ce que lit check-docs.sh, dans $tmp/NOM.
fresh() {
  mkdir "$tmp/$1"
  cp -R README.md README.fr.md LICENSE .goreleaser.yaml docs scripts packaging .github "$tmp/$1/"
}

# refused NOM MOTIF : check-docs.sh sur la copie NOM doit échouer, avec MOTIF
# dans son message.
refused() {
  if bash "$tmp/$1/scripts/check-docs.sh" > "$tmp/out" 2>&1; then
    bad "$1 : check-docs.sh a réussi"
  elif ! grep -q -- "$2" "$tmp/out"; then
    bad "$1 : refus pour une autre raison : $(grep ERREUR "$tmp/out" | head -n 3)"
  else
    echo "ok refus $1 : $(grep -- "$2" "$tmp/out" | head -n 1)"
  fi
}

# edit NOM EXPR : applique l'expression perl EXPR aux deux docs de la copie NOM.
edit() {
  for f in $docs; do perl -pi -e "$2" "$tmp/$1/$f"; done
}

# Témoin : la copie non modifiée passe (sinon les refus ne prouvent rien).
fresh temoin
bash "$tmp/temoin/scripts/check-docs.sh" > "$tmp/out" 2>&1 && echo "ok copie témoin : DOCS OK" || bad "copie témoin refusée : $(grep ERREUR "$tmp/out" | head -n 3)"

# Une seule ligne V= (la première de chaque doc) reste sur une autre version.
fresh une-differente
edit une-differente 'if (!$done && /^V=/) { s/^V=.*/V=0.2.1/; $done = 1 }'
refused une-differente "lignes V= différentes"

# Le cas du Minor : toutes les lignes V= restées sur la 0.1, image en :0.2.
fresh ancienne-serie
edit ancienne-serie 's/^V=.*/V=0.1.2/'
refused ancienne-serie "série 0.1"

# V= sans la forme X.Y.Z.
fresh forme
edit forme 's/^V=.*/V=0.2/'
refused forme "forme X.Y.Z"

# Regex cosign d'une autre série que l'image.
fresh regex
edit regex 's/v0\\\.2\\\.\[0-9\]\+/v0\\.1\\.[0-9]+/'
grep -qF 'v0\.1\.[0-9]+' "$tmp/regex/docs/en/install.md" || bad "regex : mutation non appliquée"
refused regex "identité de signature de l'image"

# Un seul fichier perd toutes ses lignes V= : l'autre ne suffit pas.
fresh sans-v-en
perl -ni -e 'print unless /^V=/' "$tmp/sans-v-en/docs/en/install.md"
refused sans-v-en "docs/en/install.md : aucune ligne V="

# Série 0.3 cohérente partout (spec §8, README, docs : image :0.3, V=0.3.0,
# regex v0\.3\.) : doit passer, la série n'est écrite en dur nulle part.
fresh serie-03
for f in docs/superpowers/specs/2026-10-07-hublot-kit-v0.1-design.md README.md README.fr.md docs/en/*.md docs/fr/*.md; do
  perl -pi -e 's/hublot-kit:0\.2(?![.\d])/hublot-kit:0.3/g; s/^V=0\.2\.0$/V=0.3.0/; s/v0\\\.2\\\./v0\\.3\\./g' "$tmp/serie-03/$f"
done
grep -q '^V=0\.3\.0$' "$tmp/serie-03/docs/fr/installation.md" && grep -qF 'v0\.3\.[0-9]+' "$tmp/serie-03/docs/fr/installation.md" &&
  ! grep -rq 'hublot-kit:0\.2' "$tmp/serie-03/README.md" "$tmp/serie-03/docs/en" "$tmp/serie-03/docs/fr" || bad "serie-03 : mutation incomplète"
bash "$tmp/serie-03/scripts/check-docs.sh" > "$tmp/out" 2>&1 && echo "ok série 0.3 cohérente : DOCS OK" ||
  bad "serie-03 : refusée : $(grep ERREUR "$tmp/out" | head -n 3)"

# La vraie doc passe.
bash scripts/check-docs.sh > "$tmp/out" 2>&1 && echo "ok doc actuelle : DOCS OK" || bad "doc actuelle refusée : $(cat "$tmp/out")"

[ "$fails" = 0 ] || { echo "$fails échec(s)" >&2; exit 1; }
echo "CHECK-DOCS OK"
