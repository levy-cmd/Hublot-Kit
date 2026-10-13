package main

// The agent speaks English (plan Lancement, Q49): every human text it
// prints, logs or returns in an API "detail" is English. This guard parses
// the production code of cmd/ and internal/ and refuses any string literal
// that looks French: an accented letter, a space before a colon (French
// typography: "%w : %s"), or a common French word. Code comments, tests and
// the interface dictionaries (web/) are not concerned.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var frenchText = regexp.MustCompile(`(?i)[àâçéèêëîïôûùüœ]|\s:(\s|$)|\b(le|la|les|des|du|de|un|une|est|pas|sans|sur|ou|et|au|aux|ce|dans|pour|par|avec|jeton|dossier|lien|chemin|fichier|conteneur|montage|refus|ensemble|journal|indisponible|lecture|seule|trop|volumineux|indexation|cours|plafond|atteint|certificat|octets|client Docker)\b`)

func TestAgentMessagesInEnglish(t *testing.T) {
	var found []string
	for _, dir := range []string{"../../cmd", "../../internal"} {
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.ImportSpec, *ast.Field: // import paths, struct tags
					return false
				case *ast.BasicLit:
					if s, err := strconv.Unquote(x.Value); x.Kind == token.STRING && err == nil && frenchText.MatchString(s) {
						found = append(found, fset.Position(x.Pos()).String()+" "+strconv.Quote(s))
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(found) > 0 {
		t.Errorf("%d French strings in the agent:\n%s", len(found), strings.Join(found, "\n"))
	}
}
