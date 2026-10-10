package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/levy-cmd/hublot-kit/internal/dockerx"
)

func TestChanges(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	f.docker.Diffs = map[string][]dockerx.Change{"web": {
		{Path: "/z", Kind: "added"}, {Path: "/a/b", Kind: "deleted"}, {Path: "/a", Kind: "modified"},
	}}
	if rec := f.do("GET", "/api/containers/web/changes", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", rec.Code)
	}
	rec := f.do("GET", "/api/containers/web/changes", "", c)
	var got []dockerx.Change
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	want := []dockerx.Change{{Path: "/a", Kind: "modified"}, {Path: "/a/b", Kind: "deleted"}, {Path: "/z", Kind: "added"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("trié par chemin : %+v", got)
	}
	f.docker.Diffs["empty"] = nil
	if rec := f.do("GET", "/api/containers/empty/changes", "", c); rec.Body.String() != "[]\n" {
		t.Errorf("aucun changement : %q, attendu []", rec.Body)
	}
}

func TestChangesErrors(t *testing.T) {
	f := newFixture(t, false)
	c := f.session(t)
	if rec := f.do("GET", "/api/containers/nope/changes", "", c); rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Errorf("inconnu : %d %s", rec.Code, rec.Body)
	}
	if rec := f.do("GET", "/api/containers/.hidden/changes", "", c); rec.Code != http.StatusNotFound {
		t.Errorf("identifiant invalide : %d", rec.Code)
	}
	f.docker.Err = fmt.Errorf("%w : dial", dockerx.ErrUnavailable)
	if rec := f.do("GET", "/api/containers/web/changes", "", c); rec.Code != http.StatusServiceUnavailable || errCode(t, rec) != "docker_unavailable" {
		t.Errorf("Docker arrêté : %d %s", rec.Code, rec.Body)
	}
}
